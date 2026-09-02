# Contract: Standard Error Envelope

Applies to every error the gateway returns to a caller as a result of this feature: a rejected
request (FR-003), a rejected response, or a handler failure/timeout (FR-012). Satisfies
Constitution Principle III (User Experience Consistency) — no route may invent its own shape.

## Shape

```json
{
  "error": {
    "code": "HANDLER_REJECTED_REQUEST",
    "message": "Missing required field 'customer_id' in request body.",
    "handler": "validate-order-request"
  }
}
```

| Field | Always present | Notes |
|---|---|---|
| `error.code` | yes | Stable, documented, machine-readable (see catalog below) |
| `error.message` | yes | Human-readable, tells the caller what to do next |
| `error.handler` | yes | `HandlerRef.name` of the handler that rejected/failed |

No `error.route` field: a single gateway process proxies to exactly one `--dest`, so which
destination is failing is already implied by which process/port answered the request.

## Status codes

| `error.code` | HTTP status | When |
|---|---|---|
| `HANDLER_REJECTED_REQUEST` | 400 | A request handler returned a `Rejection` (FR-003) |
| `HANDLER_REJECTED_RESPONSE` | 502 | A response handler returned a `Rejection` |
| `HANDLER_TIMEOUT` | 504 | A handler call exceeded its configured timeout (FR-012) |
| `HANDLER_ERROR` | 502 | A handler call errored (crash, transport error) (FR-012) |

`error.message` for `HANDLER_REJECTED_REQUEST` / `HANDLER_REJECTED_RESPONSE` comes directly from
the handler's `Rejection.message` (handler.proto). For `HANDLER_TIMEOUT` / `HANDLER_ERROR`, the
gateway supplies a generic message — the underlying cause is recorded in the
`HandlerExecutionRecord` (data-model.md) for operator troubleshooting, not exposed to the caller.
