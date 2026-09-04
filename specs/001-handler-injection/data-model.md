# Data Model: Custom Request/Response Handler Injection

**Input**: [spec.md](./spec.md) Key Entities, resolved against [research.md](./research.md)
(see research.md §5r/§5s/§5t for the multi-route revision this version reflects)

## GatewayConfig

The whole process's configuration, resolved once at startup (FR-011): `--host`/`--port` from
CLI flags, plus the top-level options and named routes loaded from the YAML file at
`--config`. One instance per running gateway process; a process now serves as many routes as
its config file lists (previously exactly one — see research.md §5/§5r).

The `--config` file's top level is a map, not a bare list, so keys besides `endpoints` can be
added later without another shape change: `{verbose?: bool, single_endpoint?: string,
endpoints: list[RouteConfig]}`. `endpoints` mirrors the name used by the `/_` discovery
response (contracts/discovery.md) for the same data.

| Field | Type | Notes |
|---|---|---|
| `listen_host` | string | From `--host` |
| `listen_port` | int | From `--port` |
| `routes` | list[RouteConfig] | From the `--config` YAML file's `endpoints` key, in file order |
| `verbose` | bool | From the `--config` YAML file's top-level `verbose` key; defaults to `false` if omitted. When `true`, the gateway logs each request's method, path, and response status code to its standard log output |
| `single_endpoint` | string | From the `--config` YAML file's top-level `single_endpoint` key; empty/omitted means disabled (normal per-route subpath matching applies). When set, must equal the name of the sole configured route (see "Routing" below) |

**Validation rules**:
- `--config` MUST point to a parseable YAML file, or the gateway fails to start.
- `endpoints` MUST be non-empty — a `--config` file listing zero endpoints is treated as a
  startup error, not an empty-but-valid gateway (there would be nothing for it to do).
- Every `RouteConfig.name` MUST be unique across the file (case-sensitive) — the gateway fails
  to start on a duplicate rather than silently using whichever loaded last.
- No `RouteConfig.name` MAY be exactly `_` — that path is reserved for the discovery endpoint
  (research.md §5u).
- `single_endpoint`, if set, MUST be used only when `endpoints` has exactly one entry, and MUST
  equal that entry's `name` — the gateway fails to start otherwise. Root-mounting one route
  chosen out of several configured routes is not supported.

## RouteConfig

One entry in the YAML config file — a named destination with its own optional handler chain.

| Field | Type | Notes |
|---|---|---|
| `name` | string | Required, non-empty, unique, MUST NOT contain `/`, MUST NOT be exactly `_` (reserved for the discovery endpoint, research.md §5u) — also the first path segment requests are matched against (FR-001/FR-002 now apply per-route; see "Routing" below) |
| `description` | string | Optional free text for operators; the gateway itself never reads it |
| `dest` | URL | Required; the backend this route proxies to |
| `handlers` | list[HandlerRef] | Optional; from the YAML `handlers` key, which accepts either one scalar path or a list, normalized to an ordered list here (FR-005) |

**Validation rules**:
- `dest` MUST be a well-formed address; the gateway fails to start otherwise.
- Each `HandlerRef.file_path` MUST resolve to a loadable handler at startup, or the gateway
  fails to start (research.md §5r) — a route is never silently left without a handler it was
  configured to have.
- A route with no `handlers` behaves as baseline pass-through proxying for that route (FR-009).

## Routing

Not a persisted entity, but the request-time behavior tying `RouteConfig.name` to inbound
requests (research.md §5s):

- The first path segment of an inbound request (`/app1/foo` → `app1`) is looked up against the
  configured routes by exact `name` match (O(1) map lookup — route count doesn't affect
  per-request latency).
- On a match, that segment is stripped before forwarding: `/app1/foo` → backend request `/foo`;
  `/app1` alone → backend request `/`.
- On no match (including a request to `/` itself), the gateway responds 404 with the
  `ROUTE_NOT_FOUND` error code (`contracts/error-format.md`) — no handler chain runs, since
  there is no route to run one for.
- The exact path `/_` is checked first, ahead of route matching, and always returns the
  discovery listing (research.md §5u, `contracts/discovery.md`) regardless of HTTP method —
  it is not itself a route and never proxies anywhere.
- If `GatewayConfig.single_endpoint` is set, first-segment matching and stripping are bypassed
  entirely (after the `/_` check above): every request goes straight to that one route's
  handler chain with its path unchanged, so `/orders/123` reaches the backend as `/orders/123`,
  not stripped to anything. The route's own `name` plays no part in matching in this mode —
  it exists only to confirm `single_endpoint` names the right (and only) route.

## HandlerRef

One entry in a route's ordered handler list, derived from that route's `handlers` YAML value.

| Field | Type | Notes |
|---|---|---|
| `name` | string | Human-readable identifier, used in logs and error messages |
| `file_path` | string | Path to the Python module implementing this handler (FR-010, FR-011) |
| `timeout_ms` | int | Per-call timeout; defaults to 100 if unspecified (research.md §3) |
| `position` | int | Index within this route's `handlers` list; execution order (FR-005) |

## HandlerWorkerPool (runtime, not persisted)

One per distinct `(route name, file_path)` pair (research.md §5t) — even if two routes
reference the same handler file, each gets its own pool, so a crash-prone handler on one route
can't degrade another route's supply of workers. Not written to disk; exists only in the
running gateway process.

| Field | Type | Notes |
|---|---|---|
| `route_name` | string | Which route this pool belongs to |
| `file_path` | string | The handler file this pool serves |
| `size` | int | Fixed at startup; default = CPU core count (research.md §2) |
| `workers` | list[WorkerState] | One entry per pooled process |

**State transitions** (per worker): `Starting → Ready → Busy → Ready` (normal call cycle);
`Busy → Crashed → Restarting → Ready` (failure cycle, FR-006); after 5 consecutive crashes,
`Restarting → Degraded` (logged, not auto-restarted further).

## HandlerExecutionRecord

One log entry per handler invocation, for observability (FR-008). Append-only; not queried by
the gateway itself at request time.

| Field | Type | Notes |
|---|---|---|
| `request_id` | string | Correlates all records for one inbound request |
| `route` | string | `RouteConfig.name` this request matched — reintroduced now that a process serves multiple routes; previously omitted as constant when a process served exactly one destination |
| `handler_name` | string | From `HandlerRef.name` |
| `direction` | enum(`request`,`response`) | Which chain this invocation belongs to |
| `outcome` | enum(`passed`,`modified`,`rejected`,`errored`) | Per FR-008 |
| `duration_ms` | int | Wall-clock time of the call, including IPC overhead |
| `timestamp` | datetime | When the invocation completed |

**Relationships**: `GatewayConfig` 1—* `RouteConfig` (via `routes`); `RouteConfig` 1—*
`HandlerRef` (via `handlers`); `HandlerRef` *—1 `HandlerWorkerPool` (keyed by
`(route_name, file_path)` — two `HandlerRef`s on the *same* route pointing at the same file
share a pool; the same file used by a *different* route does not); every handler invocation
produces exactly one `HandlerExecutionRecord`.
