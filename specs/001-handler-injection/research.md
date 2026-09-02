# Research: Custom Request/Response Handler Injection

**Input**: [spec.md](./spec.md) | **Feature**: 001-handler-injection

## 1. Go ↔ Python execution boundary

**Decision**: The gateway supervises a bounded pool of long-lived Python worker processes —
one pool per distinct handler file referenced by the startup configuration — and calls into
them over gRPC on a local Unix domain socket (HashiCorp `go-plugin`-style: handshake on start,
liveness by process/health check, no network exposure). Each call carries a gRPC deadline
derived from the handler's configured timeout.

**Rationale**: Satisfies FR-006 (isolate a failing/slow handler) and FR-012 (fail closed on
error/timeout) far more reliably than in-process execution: a worker crash or a GIL-stuck call
is contained to that worker and cancelled/killed on deadline, without risking the Go process
itself. gRPC's built-in deadline/cancellation propagation maps directly onto the fail-closed
requirement. The gateway owns the worker's entire lifecycle from its own startup configuration
(spawns it, health-checks it, restarts it, kills it) — no independently-deployed or
remotely-hosted service is ever in the path — which is the substantive intent behind FR-010's
"code-level plugin loaded into the gateway process" (excluding an externally-operated webhook),
even though the mechanism is a supervised local subprocess rather than same-process execution.

**Alternatives considered**:
- *Embedded CPython via CGo* (e.g., go-python3 bindings): satisfies FR-010 literally (same OS
  process/thread), but a crashing or GIL-stuck handler can stall or crash the entire gateway,
  directly undermining FR-006/FR-012. Also complicates cross-compilation and requires `libpython`
  at build and runtime. Rejected after discussing the tradeoff with the user, who chose the
  supervised subprocess pool.
- *Custom line-delimited JSON-RPC over stdio*: simpler tooling (no protobuf codegen), but weaker
  typing/versioning and no built-in deadline propagation — would require hand-rolling timeout
  and cancellation semantics that gRPC provides natively. Rejected.

## 2. Worker pool sizing and lifecycle

**Decision**: A fixed-size, configurable pool per handler file (default size = number of CPU
cores available to the gateway process). Workers are started once at gateway startup and reused
across requests — never spawned per-request. On worker crash, the supervisor restarts it with
backoff; any in-flight call to a crashed worker is treated as a handler failure (fails closed
per FR-012). After 5 consecutive crashes (reasonable default), the supervisor marks that worker
slot degraded and logs an alert-worthy error rather than restart-looping indefinitely.

**Rationale**: A fixed, bounded pool directly satisfies the constitution's Performance
Requirements principle ("resource usage ... MUST be bounded ... unbounded ... process pools are
prohibited") and keeps latency low by avoiding per-request process startup cost.

**Alternatives considered**: Spawn-per-request (rejected: unbounded process creation under load,
high latency from Python interpreter startup); unbounded/auto-scaling pool (rejected: violates
the bounded-resource requirement without a load-shedding story that's in scope for this feature).

## 3. Timeout and fail-closed mechanics

**Decision**: Every handler invocation carries a per-handler configurable timeout (default
100ms, reasonable default pending real measurement). The Go supervisor issues the gRPC call with
a deadline; on deadline exceeded or any RPC error, the gateway treats it as FR-012's "handler
errors or exceeds its allotted time" case and fails closed — the request is rejected (for a
request handler) or the response is rejected (for a response handler) with the standard error
envelope (see `contracts/error-format.md`).

**Rationale**: Directly implements FR-012 and gives Success Criterion SC-004's 20ms p95 overhead
budget a concrete enforcement point — the timeout is the backstop that keeps a single handler
call from blowing the budget.

**Alternatives considered**: No per-handler timeout, relying only on the caller's overall request
timeout (rejected: too coarse — a slow handler could consume the entire request budget with no
isolation from other handlers in the same chain).

## 4. Handler contract shape

**Decision**: A single gRPC service (defined in `contracts/handler.proto`) with two RPCs —
`ProcessRequest` and `ProcessResponse` — each taking a language-agnostic representation of the
HTTP message (method, path, headers, body bytes, ...) and returning either a possibly-modified
message or a structured rejection (error code + human-readable message).

**Rationale**: A generic transport-level message (rather than a domain-specific schema) keeps
the contract stable regardless of what any individual backend's API looks like, and the
structured rejection shape maps directly onto the standard error envelope required by
Principle III (User Experience Consistency) and FR-007.

**Alternatives considered**: A separate RPC per HTTP verb or per route (rejected: unnecessary
surface area — the handler only needs to see/modify the message, not dispatch on verb); passing
the raw HTTP bytes directly (rejected: forces every handler author to write their own HTTP
parser instead of working with structured fields).

## 5. Startup configuration format

**Decision** (confirmed by the user's CLI example: `api-gateway --dest http://127.0.0.1:1234
--host 0.0.0.0 --port 1235 --handlers ./handlers/v1.py`): configuration is entirely command-line
flags, resolved once at process startup — no config file. A single gateway process proxies to
exactly **one** upstream destination (`--dest`), listening on `--host`/`--port`; there is no
route table. To front multiple destinations, an operator runs multiple gateway processes (one
per destination), each with its own listen address and handler chain. `--handlers` points to a
Python module; the SDK loads it and registers whichever of `RequestHandler`/`ResponseHandler`
it defines (a single file MAY implement both directions). `--handlers` is repeatable
(`--handlers a.py --handlers b.py`) to chain multiple handler files in the given order (FR-005) —
a reasonable default consistent with idiomatic Go flag parsing (`flag.Func` collecting into a
slice), since the user's example showed only one file but FR-005 requires chaining support.
The gateway fails to start if any `--handlers` file cannot be loaded, or if `--dest` is not a
well-formed address — refusing to start beats silently falling back to bare pass-through.

**Rationale**: Directly implements FR-011 (handler wiring resolved from startup configuration,
no runtime registration API), FR-005 (execution order = flag order), and matches the simpler
single-destination shape revealed by the CLI — this is a per-destination sidecar/proxy, not a
multi-tenant router.

**Alternatives considered**: A YAML/JSON config file (this document's original decision, since
superseded — rejected once the user specified plain CLI flags as the actual launch mechanism); a
route table supporting multiple destinations per process (rejected — the CLI signature has
exactly one `--dest`, no path-to-backend mapping, so multi-destination routing is out of scope
for a single process).

## 6. Testing approach

**Decision**: Three layers, per the constitution's Testing Standards principle: (a) contract
tests validating the Go gRPC client and a reference Python worker against `handler.proto`; (b)
integration tests running the real gateway binary against a stub backend and real (test) Python
handlers, one per acceptance scenario in each user story; (c) unit tests for the Go-side CLI
config parsing, proxy, chain-execution, and worker-pool supervisor logic in isolation.

**Rationale**: Matches FR-by-FR: contract tests cover FR-010/FR-011 (the handler boundary
itself); integration tests cover FR-001–FR-005 and FR-009 (end-to-end proxy behavior); unit tests
cover FR-006/FR-008/FR-012 (isolation, logging, fail-closed) at the component level where
failures are easiest to inject deterministically.
