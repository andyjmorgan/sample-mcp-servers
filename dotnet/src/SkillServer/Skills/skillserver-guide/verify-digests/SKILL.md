---
name: verify-digests
description: Check that a skill file read over MCP matches the sha256 digest and size in its skills/list or skills/get entry.
license: Apache-2.0
---

# Verify digests

1. Fetch the skill's entry with `skills/get`.
2. Read each file with `resources/read`. For a `text` result, take its UTF-8
   bytes. For a `blob` result, base64-decode it.
3. Compare the byte length to the entry's `size`, then compare
   `sha256:` plus the lowercase hex SHA-256 of the bytes to the entry's `digest`.
4. Treat any mismatch as a stale or tampered file. Call `skills/get` again and
   start over from the new manifest.
