# api-gateway

A reverse proxy serving one or more named routes, each with its own destination and an
optional chain of Python request/response handlers. The core gateway is Go; handlers are
Python, run as supervised local worker processes the gateway talks to over gRPC.

## Running the gateway

```bash
make build
make venv   # once, to create .venv with a matching handlers-sdk + protobuf install
```

```yaml
# config.yaml
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
- `--config`: path to a YAML file listing every route this process serves. Each route has a
  `name` (also its first path segment — `/app1/...` routes to `app1`, with that segment
  stripped before forwarding), a `dest`, an optional `description` (for operators; the gateway
  never reads it), and optional `handlers` (one path, or a list to chain several in order —
  omit for plain pass-through on that route).
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
