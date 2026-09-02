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

## 5. Startup configuration format (superseded — see §5r below)

~~**Decision**: configuration is entirely command-line flags, resolved once at process
startup — no config file. A single gateway process proxies to exactly **one** upstream
destination (`--dest`); there is no route table. To front multiple destinations, an operator
runs multiple gateway processes.~~

Superseded by the user's follow-up request for multiple named routes in one process,
configured from a file. Kept here (struck through) rather than deleted, since the rejection of
a config file in favor of CLI-only flags was itself a real decision worth a paper trail — the
tradeoff reverses once the shape genuinely needs a route *table*, not just route-shaped flags.

## 5r. Startup configuration format (revised)

**Decision** (confirmed by the user's follow-up CLI/config example): `--host` and `--port`
remain CLI flags (the gateway's own listen address), but `--dest` and `--handlers` are
**removed** as top-level flags. In their place, `--config <path>` points to a YAML file
listing one or more named routes, resolved once at gateway startup — still no runtime
registration API (FR-011 unchanged, just now covering N routes instead of 1):

```yaml
- name: app1
  description: this app is doing foo....
  dest: http://localhost:3000
- name: app2
  description: ....
  dest: http://localhost:3001
  handlers: ./handlers/app1.py
```

- `name` (required, unique, no `/`): the route's identifier — also the first path segment
  requests are matched against (§5s below) and the name used in logs/errors.
- `description` (optional): free text for operators; the gateway itself never reads it.
- `dest` (required): the backend this route proxies to.
- `handlers` (optional): a single file path, or a YAML list of file paths, in the given order
  (FR-005 chaining) — same semantics as the old repeatable `--handlers` flag, just scoped per
  route instead of to the whole process. Omitted = pass-through for that route (FR-009).

The gateway fails to start if `--config` is missing/unparseable, any route's `dest` isn't a
well-formed address, any route's `handlers` file can't be loaded, or any route `name` is empty,
duplicated, or contains `/` — same "fail fast rather than silently misconfigured" precedent as
before.

**Rationale**: A YAML *file* is the natural fit once the shape is "a list of routes," each with
its own optional handler chain — expressing that as repeated flags (e.g. a
`--route name=app1,dest=...,handlers=a.py,b.py` mini-DSL) would be far more awkward to write
and parse correctly than a list of records. `--host`/`--port` stay CLI flags since they're a
process-wide concern (where *this* gateway listens), not a per-route one.

**Alternatives considered**: Keep `--dest`/`--handlers` as CLI flags alongside a new
`--config`-based mode, supporting both (rejected — two parallel ways to configure the same
thing adds real ongoing complexity for no current benefit; this is pre-release internal
tooling with no external users depending on the old flags yet, so a clean breaking replacement
is simpler than maintaining both indefinitely, per the constitution's Code Quality principle
against speculative flexibility). JSON instead of YAML (rejected only for consistency with the
project's earlier default; no functional difference).

## 5s. Path-based routing

**Decision** (confirmed with the user): the first path segment of an inbound request is
matched, verbatim, against each configured route's `name` — `/app1/foo` matches route `app1`.
The matched segment is **stripped** before forwarding: the backend receives `/foo` (or `/` if
nothing remains, e.g. a request to exactly `/app1`). No match — including a request to `/`
itself, or an unrecognized first segment — returns 404 with a new `ROUTE_NOT_FOUND` error code
(see `contracts/error-format.md`).

**Rationale**: Stripping the prefix means a backend never needs to know which path segment it
happens to be mounted under in the gateway's routing table — the common convention for
path-based ingress/API-gateway routing, and the option the user chose when asked.

**Alternatives considered**: Forwarding the full, unmodified path (rejected by the user —
would require every backend to be written aware of its own gateway-assigned prefix); matching
by `Host` header / virtual-hosting instead of path segment (rejected — the user's example is
explicitly path-based on one `host:port`).

## 5t. Worker pool scoping under multiple routes

**Decision**: a worker pool is now scoped to one `(route name, handler file)` pair, not
globally to a handler file path across the whole process — even if two routes happen to
reference the same file, each route gets its own independent pool of workers running that
file.

**Rationale**: Preserves FR-006 (isolate a failing/slow handler so it affects only requests
that invoke it) at the new, finer grain: a shared pool would let route A's traffic exhaust or
crash-loop workers that route B's traffic also depends on, even though A and B are otherwise
unrelated. This is the same reasoning that ruled out a shared pool across unrelated routes
before multi-route support existed, just re-applied now that "unrelated route" is a real
in-process concept again.

## 5u. Discovery endpoint (`/_`)

**Decision**: a built-in, always-on endpoint at the exact path `/_` returns every configured
route's `name` and `description` as JSON, in the order they appear in `--config`:

```json
{
  "endpoints": [
    {"name": "app1", "description": "this app is doing foo...."},
    {"name": "app2", "description": "...."}
  ]
}
```

`_` becomes a **reserved** route `name` — the gateway fails to start if any configured route
is literally named `_`, since that path now belongs to discovery. The endpoint responds the
same way regardless of HTTP method (there's no meaningful body/side-effect distinction for a
read-only listing), and is checked before normal route matching (one cheap string comparison
per request, ahead of the O(1) map lookup — no measurable overhead per research.md §5s).

**Rationale**: Directly what was asked — a way for a caller to discover what's available
without reading `config.yaml` directly (e.g. a teammate hitting the gateway who doesn't have
filesystem access to it). JSON matches the shape the gateway's other structured responses
already use (`contracts/error-format.md`), rather than introducing a second, human-only format.

**Alternatives considered**: Including `dest` in the listing (rejected — the ask was
specifically "endpoints and the description"; exposing internal backend addresses to anyone
who can reach the gateway is a small unnecessary information leak with no stated need); an
HTML page instead of JSON (rejected — no stated need for a browsable UI, and it would be a
second response format to keep consistent as routes change); making the endpoint
configurable/toggleable (rejected as unwarranted complexity for a read-only listing with no
sensitive fields — YAGNI).

## 6. Testing approach

**Decision**: Three layers, per the constitution's Testing Standards principle: (a) contract
tests validating the Go gRPC client and a reference Python worker against `handler.proto`; (b)
integration tests running the real gateway binary against a stub backend and real (test) Python
handlers, one per acceptance scenario in each user story, now including multi-route scenarios
(two routes with independent handlers/backends, route-not-found, and the `/_` discovery
listing); (c) unit tests for the
Go-side YAML config parsing, path-based router, proxy, chain-execution, and worker-pool
supervisor logic in isolation.

**Rationale**: Matches FR-by-FR: contract tests cover FR-010/FR-011 (the handler boundary
itself); integration tests cover FR-001–FR-005 and FR-009 (end-to-end proxy behavior); unit tests
cover FR-006/FR-008/FR-012 (isolation, logging, fail-closed) at the component level where
failures are easiest to inject deterministically.
