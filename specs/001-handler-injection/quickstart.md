# Quickstart: Writing a Request/Response Handler

## 1. Implement a handler

```python
# handlers/v1.py
from gateway_handler_sdk import RequestHandler, HttpMessage, Rejection

class AddRequestIdHandler(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage | Rejection:
        if "X-Request-Id" not in request.headers:
            request.headers["X-Request-Id"] = generate_request_id()
        return request
```

A single file may implement a request handler, a response handler, or both — subclass
`ResponseHandler` and implement `process_response(self, response: HttpMessage) -> HttpMessage |
Rejection` alongside (or instead of) `RequestHandler`. The SDK registers whichever base classes
the file defines.

Returning a `Rejection(error_code, message)` stops the request (or response) and produces the
standard error envelope (`contracts/error-format.md`) instead of forwarding it (FR-003).

## 2. Launch the gateway

```bash
api-gateway \
  --dest http://127.0.0.1:1234 \
  --host 0.0.0.0 \
  --port 1235 \
  --handlers ./handlers/v1.py
```

- `--dest`: the single backend this process proxies to.
- `--host`/`--port`: where this gateway process itself listens.
- `--handlers`: path to a handler file; repeat the flag to chain multiple files in the order
  given (`--handlers a.py --handlers b.py`). Omit it entirely for plain pass-through proxying
  (FR-009).

The gateway resolves all of this once at startup and refuses to start if `--dest` isn't a
well-formed address or any `--handlers` file fails to load (FR-011). To front more than one
backend, run more than one `api-gateway` process, each with its own `--dest`/`--host`/`--port`.

## 3. Verify it worked

```bash
curl -i http://0.0.0.0:1235/orders/123
# The backend at 127.0.0.1:1234 should show it received X-Request-Id (check its logs),
# and the gateway's own logs should show one HandlerExecutionRecord for
# AddRequestIdHandler with outcome=modified.
```

If `handlers/v1.py` raises an exception or exceeds its timeout, the gateway fails closed: the
caller gets a `HANDLER_ERROR` or `HANDLER_TIMEOUT` response (contracts/error-format.md), and the
process keeps serving other requests normally (FR-006).
