# Contract: Standard Error Envelope

Applies to every error the gateway returns to a caller as a result of this feature: no
matching route (404), a rejected request (FR-003), a rejected response, or a handler
failure/timeout (FR-012). Satisfies Constitution Principle III (User Experience Consistency) —
no route may invent its own shape.

## Shape

```json
{
  "error": {
    "code": "HANDLER_REJECTED_REQUEST",
    "message": "Missing required field 'customer_id' in request body.",
    "route": "app2",
    "handler": "validate-order-request"
  }
}
```

| Field | Always present | Notes |
|---|---|---|
| `error.code` | yes | Stable, documented, machine-readable (see catalog below) |
| `error.message` | yes | Human-readable, tells the caller what to do next |
| `error.route` | no | `RouteConfig.name` the request matched. Omitted for `ROUTE_NOT_FOUND`, since by definition no route matched. Reintroduced now that a process serves multiple routes (research.md §5r) — previously omitted as constant when a process served exactly one destination. |
| `error.handler` | no | `HandlerRef.name` of the handler that rejected/failed. Omitted for `ROUTE_NOT_FOUND`. |

## Status codes

| `error.code` | HTTP status | When |
|---|---|---|
| `ROUTE_NOT_FOUND` | 404 | No configured route's `name` matches the request's first path segment (research.md §5s) |
| `HANDLER_REJECTED_REQUEST` | 400 | A request handler returned a `Rejection` (FR-003) |
| `HANDLER_REJECTED_RESPONSE` | 502 | A response handler returned a `Rejection` |
| `HANDLER_TIMEOUT` | 504 | A handler call exceeded its configured timeout (FR-012) |
| `HANDLER_ERROR` | 502 | A handler call errored (crash, transport error) (FR-012) |

`error.message` for `HANDLER_REJECTED_REQUEST` / `HANDLER_REJECTED_RESPONSE` comes directly from
the handler's `Rejection.message` (handler.proto). For `HANDLER_TIMEOUT` / `HANDLER_ERROR`, the
gateway supplies a generic message — the underlying cause is recorded in the
`HandlerExecutionRecord` (data-model.md) for operator troubleshooting, not exposed to the caller.
For `ROUTE_NOT_FOUND`, the message names the unmatched path segment so an operator can quickly
tell a typo from a genuinely unconfigured route.
