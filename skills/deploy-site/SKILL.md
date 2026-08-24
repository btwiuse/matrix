---
name: deploy-site
description: |
  Publish a local static website to a public URL via the matrix.k0s.io
  deploy API. Load when the user asks to deploy/publish a website, HTML
  page, or built dist folder and get a public link, without using MCP
  tools.
---

# Deploy a local site to a public URL

One curl command uploads a `.tar.gz` or `.zip` archive and returns the
deployed site URL. No MCP tools, no server-side files, no base64.

## Command

```sh
# From a built dist folder (index.html at the archive root)
tar -czf site.tar.gz -C dist .
# or: zip -r site.zip dist

curl -X POST --data-binary @site.tar.gz -H 'Content-Type: application/gzip' \
  https://matrix.k0s.io/api/deploy
```

## Response

```json
{"website_id": 431913861627312, "website_url": "https://<random>.matrix.k0s.io", "screenshot_url": ""}
```

The `website_url` is the public site (HTTPS, works immediately).

## Notes

- Archive must contain `index.html` at its root (a single top-level
  directory like `dist/` is stripped automatically).
- Max archive size: 64 MiB. Format: .tar.gz or .zip only.
- Every deployment gets a fresh random URL; previous sites stay live.
- Errors return HTTP 400 with a JSON error body, e.g.
  `{"error": "extracting archive: unsupported archive format (want .tar.gz or .zip)"}`.
- If the user instead wants to go through MCP tools, use the
  `remote_deploy` / `upload_file` tools on the mini MCP server.
