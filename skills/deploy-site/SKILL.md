---
name: deploy-site
description: |
  Publish a local static website to a public URL via the matrix deploy
  command or the deploy API. Load when the user asks to deploy/publish
  a website, HTML page, or built dist folder and get a public link.
---

# Deploy a local site to a public URL

## Via the deploy command (recommended)

```sh
matrix deploy ./dist
matrix deploy site.tar.gz
matrix deploy --json ./dist
```

The standalone form works the same:

```sh
deploy ./dist
```

Flags: `--server URL` (default `https://matrix.k0s.io`, or `$MATRIX_SERVER`),
`--json` (print the full response instead of just the URL). A directory is
packed into `.tar.gz` automatically; an existing `.tar.gz`/`.zip` is sent
as-is.

## Via the raw API

```sh
tar -czf site.tar.gz -C dist .
curl -X POST --data-binary @site.tar.gz -H 'Content-Type: application/gzip'   https://matrix.k0s.io/api/deploy
```

## Response

```json
{"website_id": 431913861627312, "website_url": "https://<random>.matrix.k0s.io", "screenshot_url": ""}
```

`website_url` is the public site (HTTPS, live immediately).

## Notes

- The archive must contain `index.html` at its root; a single top-level
  directory like `dist/` is stripped automatically.
- Max archive size: 64 MiB. Format: .tar.gz or .zip only.
- Every deployment gets a fresh random URL; previous sites stay live.
- Errors return HTTP 400 with a JSON error body, e.g.
  `{"error": "extracting archive: unsupported archive format (want .tar.gz or .zip)"}`.
