# api-gateway

A reverse proxy serving one or more named routes, each with its own destination and an
optional chain of Python request/response handlers. The core gateway is Go; handlers are
Python, run as supervised local worker processes the gateway talks to over gRPC.

## Running the gateway

```bash
make build
make venv   # once, to create .venv with a matching handlers-sdk + protobuf install
```

`--config` points at a YAML file listing every route this process serves, under an `endpoints`
key (matching the `/_` discovery response's own `endpoints` key below):

```yaml
# config.yaml
verbose: true
endpoints:
  - name: app1
    description: The public orders API.
    dest: http://localhost:3000
  - name: app2
    description: Internal admin service.
    dest: http://localhost:3001
    handlers: ./handlers/app1.py
```

```bash
GATEWAY_PYTHON="$(pwd)/.venv/bin/python3" \
  ./gateway --host 0.0.0.0 --port 1235 --config ./config.yaml
```

- `--host` / `--port`: where this gateway process itself listens.
- `--config`: top-level `verbose` (optional, defaults to `false`; see below) plus `endpoints`,
  a list where each route has a `name` (also its first path segment — `/app1/...` routes to
  `app1`, with that segment stripped before forwarding), a `dest`, an optional `description`
  (for operators; the gateway never reads it), and optional `handlers` (one path, or a list to
  chain several in order — omit for plain pass-through on that route), and optional `headers`
  (a map of header name to value injected into every request forwarded to that route's backend,
  overriding same-named headers; `${VAR}` in a value is read from the gateway's environment at
  startup, and startup fails if `VAR` is unset — e.g. `headers: {API_TOKEN: ${API_TOKEN}, X_HEADER: foo}`).
- `verbose`: when `true`, the gateway logs each request's method, path, and response status
  code to its standard log output.
- `single_endpoint`: optional; only valid when `endpoints` has exactly one entry, and its value
  must match that entry's `name`. Mounts that one endpoint at the gateway's root instead of
  under `/<name>/...` — every path, including `/` itself, forwards to its `dest` unchanged:

  ```yaml
  # config.yaml
  single_endpoint: app1
  endpoints:
    - name: app1
      description: The public orders API.
      dest: http://localhost:3000
  ```

  A request to `http://<gateway>/orders/123` forwards to `http://localhost:3000/orders/123`
  (no path stripped, no `/app1` prefix needed). `GET /_` still returns the discovery listing.
- `GATEWAY_PYTHON`: the Python interpreter used to launch handler workers. Defaults to
  whatever `python3` resolves to on `$PATH` — set this explicitly rather than relying on that
  default, especially if `python3` happens to be a shared/pyenv/conda environment: the
  generated gRPC stubs require `protobuf>=7.35`, and an unrelated environment satisfying some
  other package's older pin will fail at startup with a `VersionError` (gencode vs. runtime
  mismatch) rather than silently working.

All configuration is resolved once at startup from these flags — there is no runtime
route/handler-management API. Changing routes means editing `config.yaml` and restarting.
A built-in `GET /_` endpoint lists every configured route's name and description, for
discovery.

### Try it: a runnable example

[examples/config.yaml](examples/config.yaml) defines three routes exercising every option —
`app1` with no handlers (plain pass-through), `app2` with one handler
([examples/handlers/add_request_id.py](examples/handlers/add_request_id.py)), and `app3`
chaining two ([examples/handlers/null_handler.py](examples/handlers/null_handler.py) then
`add_request_id.py`). [examples/echo_backend.py](examples/echo_backend.py) is a minimal
stdlib-only backend that reflects the request it received as JSON, so `curl` alone shows
exactly what each route's handlers did — a plain `python3 -m http.server` wouldn't let you see
an injected header in the response. From the repo root, in one terminal:

```bash
python3 examples/echo_backend.py 18081 &
python3 examples/echo_backend.py 18082 &
python3 examples/echo_backend.py 18083 &

make build && make venv
GATEWAY_PYTHON="$(pwd)/.venv/bin/python3" \
  ./gateway --host 0.0.0.0 --port 1235 --config examples/config.yaml
```

In another:

```bash
curl -s http://localhost:1235/app1/       # plain pass-through
curl -s http://localhost:1235/app2/       # {"...,"headers":{...,"X-Request-Id":"<uuid>",...}}
curl -s http://localhost:1235/app3/       # same X-Request-Id, added via a two-handler chain
curl -s http://localhost:1235/_           # discovery: lists app1, app2, app3
```

## Writing a handler

Install the SDK into a local dev virtualenv (from a checkout of this repo):

```bash
make venv
```

```python
# handlers/v1.py
from gateway_handler_sdk import RequestHandler, HttpMessage, Rejection

class AddRequestId(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage | Rejection:
        if "x-request-id" not in request.headers:
            request.headers["X-Request-Id"] = generate_request_id()
        return request
```

A single file may define a `RequestHandler` subclass, a `ResponseHandler` subclass (with
`process_response(self, response: HttpMessage) -> HttpMessage | Rejection`), or both — the
gateway calls whichever it finds and passes the message through unchanged for the direction
a file doesn't implement.

Returning a `Rejection(error_code, message)` stops the request (or response) instead of
forwarding it, and produces the gateway's standard error envelope:

```json
{"error": {"code": "MISSING_FIELD", "message": "...", "route": "app1", "handler": "add-request-id"}}
```

A handler that errors or exceeds its timeout (default 100ms) fails the same way — the gateway
always fails closed, and one broken handler can't affect other requests on that route, other
routes, or other gateway processes.

See [specs/001-handler-injection/quickstart.md](specs/001-handler-injection/quickstart.md) for
a full walkthrough, and [specs/001-handler-injection/](specs/001-handler-injection/) for the
feature's spec, plan, and design decisions.

## Release binaries

The [Release workflow](.github/workflows/release.yml) cross-compiles the gateway for
Linux, macOS, and Windows (amd64 and arm64 each) and publishes them as a GitHub Release.

To cut a release, push a tag starting with `v`:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The release then contains `gateway-<os>-<arch>.tar.gz` (Linux/macOS) or
`gateway-<os>-<arch>.zip` (Windows, containing `gateway.exe`), plus a `checksums.txt`.

To build without releasing, run the workflow manually from the repo's **Actions** tab
(**Release** → **Run workflow**); the binaries are attached to the run as artifacts.

The binaries are the Go gateway only. Handlers still need Python and `handlers-sdk` installed
on the machine running the gateway.

## Development

```bash
make build          # go build -o gateway ./cmd/gateway
make test           # Go tests (spawns real Python workers) + the Python SDK's pytest suite
make lint           # go vet + golangci-lint + ruff
make fmt            # gofmt + ruff format (skips generated protobuf/grpc code)
make venv           # create/refresh .venv with handlers-sdk installed in editable mode
```

`golangci-lint` isn't installed by `make venv`/`make build` — install it separately
(https://golangci-lint.run/welcome/install/) to run `make lint` locally; CI installs it for you.

Regenerate the Go/Python gRPC stubs after editing `api/handler/v1/handler.proto`:

```bash
make proto
```

This also fixes up `handler_pb2_grpc.py`'s import: `grpc_tools.protoc` generates it as an
absolute `import handler_pb2 as handler__pb2`, which breaks once the file lives inside the
`gateway_handler_sdk.v1` package, so `make proto` rewrites that one line to
`from . import handler_pb2 as handler__pb2`. Review the diff before committing.
