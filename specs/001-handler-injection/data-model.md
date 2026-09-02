# Data Model: Custom Request/Response Handler Injection

**Input**: [spec.md](./spec.md) Key Entities, resolved against [research.md](./research.md)

## GatewayConfig

The whole process's configuration, resolved once from CLI flags at startup (FR-011). One
instance per running gateway process — there is no route table; a process proxies to exactly
one destination (confirmed by the CLI example: `--dest`/`--host`/`--port`/`--handlers`).
Multiple destinations are served by running multiple gateway processes, each with its own
`GatewayConfig`.

| Field | Type | Notes |
|---|---|---|
| `listen_host` | string | From `--host` |
| `listen_port` | int | From `--port` |
| `dest` | URL | From `--dest`; the single backend this process proxies to |
| `request_handlers` | list[HandlerRef] | Ordered by `--handlers` flag order; run before proxying (FR-001, FR-005) |
| `response_handlers` | list[HandlerRef] | Ordered by `--handlers` flag order; run after the backend responds (FR-002, FR-005) |

**Validation rules**:
- `dest` MUST be a well-formed address; the gateway fails to start otherwise.
- Each `HandlerRef.file_path` (from `--handlers`) MUST resolve to a loadable handler at startup,
  or the gateway fails to start (research.md §5) — a process is never silently left without a
  handler it was configured to have.
- A given `--handlers` file MAY implement a request handler, a response handler, or both; each
  is added to the corresponding ordered list in the flag's position.
- A process launched with no `--handlers` flags behaves as baseline pass-through proxying
  (FR-009).

## HandlerRef

One entry in the process's ordered handler list, derived from one `--handlers` flag occurrence.

| Field | Type | Notes |
|---|---|---|
| `name` | string | Human-readable identifier, used in logs and error messages |
| `file_path` | string | Path to the Python module implementing this handler (FR-010, FR-011) |
| `timeout_ms` | int | Per-call timeout; defaults to 100 if unspecified (research.md §3) |
| `position` | int | Index given by `--handlers` flag order; execution order (FR-005) |

## HandlerWorkerPool (runtime, not persisted)

One per distinct `file_path` passed via `--handlers`. Not written to disk; exists only in the
running gateway process.

| Field | Type | Notes |
|---|---|---|
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
| `handler_name` | string | From `HandlerRef.name` |
| `direction` | enum(`request`,`response`) | Which chain this invocation belongs to |
| `outcome` | enum(`passed`,`modified`,`rejected`,`errored`) | Per FR-008 |
| `duration_ms` | int | Wall-clock time of the call, including IPC overhead |
| `timestamp` | datetime | When the invocation completed |

No `route` field: a single process serves exactly one `dest`, so it would be constant within a
process's logs — the process's own identity (host:port it listens on) already disambiguates
logs across instances.

**Relationships**: `GatewayConfig` 1—* `HandlerRef` (via `request_handlers` / `response_handlers`);
`HandlerRef` *—1 `HandlerWorkerPool` (multiple refs, e.g. one request- and one response-handler
entry from the same file, share one pool since they point at the same `file_path`); every
handler invocation produces exactly one `HandlerExecutionRecord`.
