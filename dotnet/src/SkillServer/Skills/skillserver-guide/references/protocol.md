# Cases skillserver exercises

1. Prefixed skill paths. The anthropic skills live at
   `skill://anthropic/<name>/SKILL.md`. The prefix is organisational; the
   skill's name is the final segment.
2. Nested skills. `verify-digests` sits inside `skillserver-guide`. Its files
   appear in both skills' manifests, and it has its own `skills/list` entry.
3. Binary content. The theme-factory PDF is served as a `blob`. Its digest
   covers the decoded bytes, not the base64 text.
4. Dynamic content. `skill://clock/SKILL.md` is generated on each read, so its
   entry carries `"resources": "dynamic"`.
5. Errors. `skills/get` and `resources/read` answer an unknown URI with
   `-32602` (Invalid params).
