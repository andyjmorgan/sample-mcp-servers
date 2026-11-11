// Package mcpserve holds the plumbing shared by every sample server: env
// config, the stateless Streamable HTTP handler pinned to one protocol
// version, and the auth layers (Keycloak/CIMD bearer tokens, X-Api-Key).
package mcpserve

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // IANA zones without relying on the base image

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// Spec describes one sample server.
type Spec struct {
	Name            string
	Version         string
	ProtocolVersion string // the only version the server advertises
	JSONResponse    bool   // reply with application/json instead of SSE
	Register        func(*mcp.Server)
}

// Config is read from the environment.
//
//	PORT            listen port (default 8080)
//	PUBLIC_URL      external base URL, e.g. https://mcp-time.donkeywork.dev
//	OAUTH_ISSUER    Keycloak realm issuer; enables bearer-token auth when set
//	OAUTH_JWKS_URL  JWKS endpoint (default: <issuer>/protocol/openid-connect/certs)
//	OAUTH_SCOPE     scope the token must carry (also advertised in PRM)
//	API_KEY         when set, requests must also send a matching X-Api-Key
//	BASIC_USER      with BASIC_PASSWORD, requests must send HTTP Basic credentials
//	BASIC_PASSWORD  password for BASIC_USER
//	BEARER_TOKEN    when set, requests must send Authorization: Bearer <this token>
//	                (a static token; mutually exclusive with OAUTH_ISSUER)
type Config struct {
	Port          string
	PublicURL     string
	Issuer        string
	JWKSURL       string
	Scope         string
	APIKey        string
	BasicUser     string
	BasicPassword string
	BearerToken   string
}

func configFromEnv() Config {
	c := Config{
		Port:          getenv("PORT", "8080"),
		PublicURL:     strings.TrimRight(os.Getenv("PUBLIC_URL"), "/"),
		Issuer:        os.Getenv("OAUTH_ISSUER"),
		JWKSURL:       os.Getenv("OAUTH_JWKS_URL"),
		Scope:         os.Getenv("OAUTH_SCOPE"),
		APIKey:        os.Getenv("API_KEY"),
		BasicUser:     os.Getenv("BASIC_USER"),
		BasicPassword: os.Getenv("BASIC_PASSWORD"),
		BearerToken:   os.Getenv("BEARER_TOKEN"),
	}
	if c.Issuer != "" && c.JWKSURL == "" {
		c.JWKSURL = strings.TrimRight(c.Issuer, "/") + "/protocol/openid-connect/certs"
	}
	if c.PublicURL == "" {
		c.PublicURL = "http://localhost:" + c.Port
	}
	return c
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Run serves the MCP endpoint at /mcp until SIGTERM or SIGINT, then drains
// in-flight requests for up to shutdownTimeout and returns (exit 0).
func Run(spec Spec) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("server", spec.Name)
	cfg := configFromEnv()
	if cfg.Issuer != "" && cfg.BearerToken != "" {
		log.Error("OAUTH_ISSUER and BEARER_TOKEN are mutually exclusive")
		os.Exit(2)
	}
	if (cfg.BasicUser == "") != (cfg.BasicPassword == "") {
		log.Error("BASIC_USER and BASIC_PASSWORD must be set together")
		os.Exit(2)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: spec.Name, Version: spec.Version}, &mcp.ServerOptions{
		SupportedProtocolVersions: []string{spec.ProtocolVersion},
	})
	spec.Register(server)

	var handler http.Handler = mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: spec.JSONResponse},
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	resource := cfg.PublicURL + "/mcp"
	if cfg.APIKey != "" {
		handler = requireAPIKey(cfg.APIKey, handler)
	}
	if cfg.BasicUser != "" {
		handler = requireBasicAuth(cfg.BasicUser, cfg.BasicPassword, spec.Name, handler)
	}
	if cfg.BearerToken != "" {
		handler = requireStaticBearer(cfg.BearerToken, handler)
	}
	if cfg.Issuer != "" {
		prmURL := cfg.PublicURL + "/.well-known/oauth-protected-resource/mcp"
		var scopes []string
		if cfg.Scope != "" {
			scopes = []string{cfg.Scope}
		}
		prm := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
			Resource:               resource,
			AuthorizationServers:   []string{cfg.Issuer},
			ScopesSupported:        scopes,
			BearerMethodsSupported: []string{"header"},
			ResourceName:           spec.Name,
		})
		// RFC 9728 path-aware location first, root location for older clients.
		mux.Handle("/.well-known/oauth-protected-resource/mcp", prm)
		mux.Handle("/.well-known/oauth-protected-resource", prm)

		verifier := newVerifier(cfg, resource)
		handler = auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
			ResourceMetadataURL: prmURL,
			Scopes:              scopes,
		})(handler)
	}
	mux.Handle("/mcp", handler)

	log.Info("listening", "port", cfg.Port, "protocol", spec.ProtocolVersion, "resource", resource,
		"oauth", cfg.Issuer != "", "scope", cfg.Scope, "apiKey", cfg.APIKey != "",
		"basicAuth", cfg.BasicUser != "", "staticBearer", cfg.BearerToken != "", "jsonResponse", spec.JSONResponse)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: logRequests(log, mux), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		log.Error("server stopped", "err", err)
		os.Exit(1)
	case <-ctx.Done():
	}
	stop() // a second signal kills the process immediately
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		log.Error("shutdown", "err", err)
		os.Exit(1)
	} else if err != nil {
		log.Warn("shutdown timed out; closing open connections", "err", err)
		srv.Close()
	}
	log.Info("stopped")
}

// shutdownTimeout bounds the drain on SIGTERM; well inside the default 30s
// terminationGracePeriodSeconds. Long-lived SSE streams are cut when it expires.
const shutdownTimeout = 5 * time.Second

// newVerifier validates Keycloak access tokens: signature via JWKS, issuer,
// expiry, and an audience that names this server's resource URL.
func newVerifier(cfg Config, resource string) auth.TokenVerifier {
	keys := oidc.NewRemoteKeySet(context.Background(), cfg.JWKSURL)
	v := oidc.NewVerifier(cfg.Issuer, keys, &oidc.Config{SkipClientIDCheck: true})
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		tok, err := v.Verify(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", auth.ErrInvalidToken, err)
		}
		if !contains(tok.Audience, resource) {
			return nil, fmt.Errorf("%w: audience %v does not include %s", auth.ErrInvalidToken, tok.Audience, resource)
		}
		var claims struct {
			Scope string `json:"scope"`
			AZP   string `json:"azp"`
		}
		if err := tok.Claims(&claims); err != nil {
			return nil, fmt.Errorf("%w: %v", auth.ErrInvalidToken, err)
		}
		return &auth.TokenInfo{
			Scopes:     strings.Fields(claims.Scope),
			Expiration: tok.Expiry,
			UserID:     tok.Subject,
			Extra:      map[string]any{"azp": claims.AZP},
		}, nil
	}
}

func requireAPIKey(key string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Api-Key")), []byte(key)) != 1 {
			http.Error(w, "missing or invalid X-Api-Key", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireBasicAuth enforces RFC 7617 HTTP Basic credentials.
func requireBasicAuth(user, pass, realm string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(user)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(pass)) != 1 {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q, charset="UTF-8"`, realm))
			http.Error(w, "missing or invalid Basic credentials", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireStaticBearer enforces a fixed Authorization: Bearer token (no OAuth,
// no JWT): the shape of an API that hands out long-lived tokens.
func requireStaticBearer(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		attrs := []any{"method", r.Method, "path", r.URL.Path, "status", sw.status,
			"ms", time.Since(start).Milliseconds(), "mcpProtocol", r.Header.Get("Mcp-Protocol-Version")}
		if ti := auth.TokenInfoFromContext(r.Context()); ti != nil {
			attrs = append(attrs, "sub", ti.UserID, "azp", ti.Extra["azp"])
		}
		log.Info("request", attrs...)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }

// Flush keeps SSE streaming working through the logging wrapper.
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// ErrResult turns a validation problem into a tool error the model can read.
func ErrResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}}}
}

// Text wraps a plain string result.
func Text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// Tool builds a tool definition with a human title and all four behaviour
// hints set explicitly. Every sample tool is a stateless, local computation,
// so it is read-only, non-destructive and closed-world; idempotent is true for
// deterministic tools and false for randomised ones. DestructiveHint and
// OpenWorldHint are pointers because the spec defaults both to true. The title
// is also copied into Annotations.Title for 2025-03-26 clients, which predate
// Tool.title.
func Tool(name, title, description string, idempotent bool) *mcp.Tool {
	f := false
	return &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			Title:           title,
			ReadOnlyHint:    true,
			DestructiveHint: &f,
			IdempotentHint:  idempotent,
			OpenWorldHint:   &f,
		},
	}
}
