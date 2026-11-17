# Contract: Path Rewriting (`rewrite_paths: true`)

Opt-in, per route (research.md §5v). Makes a backend that emits root-relative URLs
(`/assets.css`) work when mounted under `/<route name>/` on the gateway. With `rewrite_paths`
unset/false the gateway forwards bytes and headers exactly as before.

Let `P` = `/<route name>` (e.g. `/foo`).

## Response body

| Content-Type | What is rewritten |
|---|---|
| `text/html` | URL-valued attributes `href`, `src`, `action`, `formaction`, `poster`, `data`, `srcset` (every candidate); `url(...)` in `<style>` and `style=""` |
| `text/css` | `url(...)`, `@import "..."` |
| anything else | untouched (JSON, JS, images, ...) |

A URL value `U` becomes `P + U` iff `U` starts with exactly one `/`, and does not already
start with `P/` (or equal `P`). Untouched: `//cdn.example/x`, `https://...`, `data:`, `#frag`,
`?q`, and relative `img/a.png`. Compressed upstream bodies and bodies > 10 MiB are passed
through unmodified (warn log).

## Response headers

| Header | Rewrite |
|---|---|
| `Location` | `/x` → `P/x`; `<dest origin>/x` → `P/x`; other origins untouched |
| `Set-Cookie` | `Path=/x` → `Path=P/x` (incl. bare `Path=/` → `Path=P/`) |
| `Content-Length` | recomputed after body rewrite |

## Request-time fallback (route miss)

If no route matches the first path segment, and `Referer`'s path begins with `/<name>` of a
`rewrite_paths` route:

```
GET /api/items?x=1          Referer: http://gateway/foo/dashboard
→ 307 Temporary Redirect    Location: /foo/api/items?x=1
                            Vary: Referer
```

Otherwise the existing `404 ROUTE_NOT_FOUND` (contracts/error-format.md) is returned.

## Example

Backend response for `GET /foo/` (backend saw `GET /`):

```html
<link rel="stylesheet" href="/assets.css"><img src="//cdn.x/y.png"><a href="page2">
```
becomes
```html
<link rel="stylesheet" href="/foo/assets.css"><img src="//cdn.x/y.png"><a href="page2">
```
