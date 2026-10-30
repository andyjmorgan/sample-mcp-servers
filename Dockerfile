# One image per server: docker build --build-arg SERVER=timeserver -t timeserver .
FROM golang:1.26-alpine AS build
ARG SERVER
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN test -n "$SERVER" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/$SERVER

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/server"]
