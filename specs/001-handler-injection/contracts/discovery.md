# Contract: Discovery Endpoint (`/_`)

A built-in, always-on endpoint (research.md §5u) that lists every configured route so a caller
can discover what's available without reading `config.yaml` directly. Not itself a route —
`_` is a reserved `RouteConfig.name` (data-model.md) — and never proxies anywhere.

## Request

`GET /_` (any HTTP method returns the same listing; there's no body/side-effect distinction
for a read-only endpoint).

## Response

Always `200 OK`, `Content-Type: application/json`:

```json
{
  "endpoints": [
    {"name": "app1", "description": "this app is doing foo...."},
    {"name": "app2", "description": "...."}
  ]
}
```

| Field | Notes |
|---|---|
| `endpoints` | One entry per configured route, in `config.yaml` order |
| `endpoints[].name` | `RouteConfig.name` |
| `endpoints[].description` | `RouteConfig.description`, or `""` if the route didn't set one |

Deliberately excludes `dest` (the backend address) and `handlers` (file paths) — this endpoint
is for callers discovering *what's available*, not an operational/debug view of *how* each
route is wired. It also never runs a route's handler chain: it answers directly from the
loaded `GatewayConfig`, before any route matching happens.
