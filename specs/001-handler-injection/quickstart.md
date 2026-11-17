# Quickstart: Multiple Named Routes with Request/Response Handlers

## 1. Implement a handler

```python
# handlers/app1.py
from gateway_handler_sdk import RequestHandler, HttpMessage, Rejection

class AddRequestIdHandler(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage | Rejection:
        if "x-request-id" not in request.headers:
            request.headers["X-Request-Id"] = generate_request_id()
        return request
```

A single file may implement a request handler, a response handler, or both — subclass
`ResponseHandler` and implement `process_response(self, response: HttpMessage) -> HttpMessage |
Rejection` alongside (or instead of) `RequestHandler`. The SDK registers whichever base classes
the file defines.

Returning a `Rejection(error_code, message)` stops the request (or response) and produces the
standard error envelope (`contracts/error-format.md`) instead of forwarding it (FR-003).

## 2. Write the routes config

```yaml
# config.yaml
verbose: true
endpoints:
  - name: app1
    description: The public orders API.
    dest: http://localhost:3000
  - name: app2
    description: Internal admin service — validates every request.
    dest: http://localhost:3001
    handlers: ./handlers/app1.py
```

- `verbose`: optional, defaults to `false`. When `true`, the gateway logs each request's
  method, path, and response status code to its standard log output.
- `endpoints`: the list of routes, each with:
  - `name`: required, unique, no `/` — also the first path segment requests are matched against.
  - `description`: optional, for operators only; the gateway never reads it.
  - `dest`: required, the backend this route proxies to.
  - `handlers`: optional; one file path, or a YAML list to chain multiple in order
    (`handlers: [./handlers/a.py, ./handlers/b.py]`). Omit for plain pass-through on that route
    (FR-009).
  - `headers`: optional map of header name to value, injected into every request forwarded to
    this route's backend (overriding same-named headers from the caller or a handler). `${VAR}`
    in a value is expanded from the gateway's environment at startup; an unset variable is a
    startup error.
  - `rewrite_paths`: optional, default `false`. Set `true` when the backend's pages reference
    root-relative URLs like `/assets.css` and would otherwise render blank under
    `http://gateway/<name>/`. See "Pages with root-relative paths" below.
- `single_endpoint`: optional; only valid when `endpoints` has exactly one entry, and its
  value must match that entry's `name`. Mounts that endpoint at the gateway's root — every
  path (including `/`) forwards to its `dest` unchanged, with no `/<name>/...` prefix needed:

  ```yaml
  # config.yaml
  single_endpoint: app1
  endpoints:
    - name: app1
      description: The public orders API.
      dest: http://localhost:3000
  ```

  The gateway fails to start if `single_endpoint` doesn't match the sole endpoint's `name`, or
  if it's set alongside more than one `endpoints` entry — root-mounting one route out of
  several isn't supported.

## 3. Launch the gateway

```bash
api-gateway --host 0.0.0.0 --port 1235 --config ./config.yaml
```

The gateway resolves all routes once at startup and refuses to start if `--config` is
missing/unparseable, any route's `dest` isn't well-formed, any route's handler file fails to
load, or any route `name` is empty, duplicated, or contains `/` (FR-011). Changing routes means
editing `config.yaml` and restarting.

## 4. Verify it worked

```bash
curl -i http://0.0.0.0:1235/app1/orders/123
# Forwarded to http://localhost:3000/orders/123 (app1's "app1" prefix is stripped) — app1 has
# no handlers configured, so this is plain pass-through.

curl -i http://0.0.0.0:1235/app2/anything
# Forwarded to http://localhost:3001/anything, after AddRequestIdHandler runs. The backend
# should show it received X-Request-Id, and the gateway's logs should show one
# HandlerExecutionRecord with route="app2", handler_name="app1" (from the file's basename),
# outcome=modified.

curl -i http://0.0.0.0:1235/does-not-exist
# 404 ROUTE_NOT_FOUND — no configured route's name matches "does-not-exist".

curl -i http://0.0.0.0:1235/_
# 200, lists every configured route for discovery (contracts/discovery.md):
# {"endpoints":[{"name":"app1","description":"The public orders API."},
#                {"name":"app2","description":"Internal admin service..."}]}
```

If a route's handler raises an exception or exceeds its timeout, the gateway fails closed: the
caller gets a `HANDLER_ERROR` or `HANDLER_TIMEOUT` response (contracts/error-format.md,
including which `route` and `handler` failed), and every other route — including ones sharing
the same handler *file* — keeps serving normally (FR-006): each route gets its own worker pool,
even for a handler file two routes happen to share.

## Pages with root-relative paths

Symptom: `http://gateway/foo/` returns a blank page because the HTML says
`<link href="/assets.css">`, which the browser requests as `http://gateway/assets.css` (no
`/foo`) and gets `ROUTE_NOT_FOUND`. Fixes, in order of preference:

1. **One app only?** Use `single_endpoint` — the app is mounted at `/` and nothing needs
   rewriting.
2. **You control the app?** Make it prefix-agnostic (relative URLs like `assets.css`).
3. **Otherwise** set `rewrite_paths: true` on the route. The gateway then rewrites URLs in
   HTML/CSS, redirects and cookie paths, and redirects stray JS-initiated requests using the
   `Referer` header (`contracts/path-rewriting.md`).

Limits of `rewrite_paths`: URLs constructed inside JavaScript are only recovered through the
`Referer` fallback (one extra 307 per request, and not at all if the app sends
`Referrer-Policy: no-referrer`); `Link:` headers and `<meta refresh>` are not rewritten.
