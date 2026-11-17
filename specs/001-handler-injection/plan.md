# Implementation Plan: Custom Request/Response Handler Injection

**Branch**: `001-handler-injection` | **Date**: 2026-09-01 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-handler-injection/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Let API teams customize request and response handling — validate/transform inbound requests,
transform/filter outbound responses — without touching their backend service. The gateway core
is a Go proxy launched as `api-gateway --host <listen-host> --port <listen-port> --config
<routes.yaml>`, where the YAML file lists one or more named routes (`name`, `description`,
`dest`, optional `handlers`); handlers are written in Python. A request's first path segment
(`/app1/...`) selects its route by name, with that segment stripped before forwarding. The Go
process supervises a bounded pool of long-lived Python worker processes per `(route,
handler-file)` pair (resolved from the config file at startup, not a runtime API) and calls
them over a local gRPC channel with a per-call timeout, failing closed on any handler error or
timeout so one broken handler can't take down its route — let alone any other route. A
built-in `GET /_` endpoint lists every configured route's name and description, so a caller
can discover what's available without reading `config.yaml` directly.

**Path rewriting (revision 3)**: pages proxied under `/<route>/` break when they reference
root-relative URLs (`/assets.css` → blank page). Routes gain an opt-in `rewrite_paths` flag
that re-prefixes URLs in HTML/CSS bodies and `Location`/`Set-Cookie` headers, plus a
`Referer`-based 307 fallback for JS-initiated requests that miss routing. See research.md §5v
and `contracts/path-rewriting.md`. New dependency: `golang.org/x/net/html` (tokenizer).

**Revision note**: this supersedes the previous single-destination-per-process design (`--dest`
+ repeatable `--handlers`, no route table). See research.md §5r/§5s/§5t for what changed and
why; §5/§6's original text is kept, struck through, rather than deleted.

## Technical Context

**Language/Version**: Go 1.23 (gateway core); Python 3.11+ (request/response handlers)

**Primary Dependencies**: Go: standard `net/http`, standard `flag` (CLI parsing — the flag set
is small enough that a third-party CLI framework would be unjustified complexity per the
Code Quality principle), `gopkg.in/yaml.v3` (routes config), `golang.org/x/net/html` (HTML tokenizer for `rewrite_paths`), `google.golang.org/grpc`,
`google.golang.org/protobuf`. Python: `grpcio` + generated stubs from the shared `.proto`
contract, packaged behind a small `handlers-sdk` so handler authors implement two callbacks
rather than hand-writing gRPC boilerplate.

**Storage**: N/A for persistent storage — `--host`/`--port` are CLI flags, and the route table
(names, destinations, handler chains) comes from the YAML file at `--config`, loaded once at
startup; no database, no runtime config API.

**Testing**: Go: `go test` for unit/integration/contract suites. Python: `pytest` for the
handler SDK's own contract tests.

**Target Platform**: Linux server, containerized deployment; one process now serves multiple
named routes (previously: one process per upstream destination).

**Project Type**: Single CLI-launched web-service (network proxy, now with an in-process
path-based router) plus a small Python SDK package consumed by handler authors.

**Performance Goals**: Baseline proxy (a route with no `handlers` configured) adds ≤5ms p95
over direct backend latency. Each configured handler adds ≤20ms p95 over the local gRPC hop
(matches SC-004). Route matching itself is an O(1) map lookup by first path segment, so it must
add no measurable overhead as the route count grows. Reference target throughput ≥5,000 req/s
per gateway instance on 4 vCPU — a default pending real load-test data (unchanged from before).

**Constraints**: Every handler call MUST carry a bounded, configurable timeout (default 100ms)
enforced via gRPC deadline; on timeout or error the gateway MUST fail closed (FR-012). The
Python worker pool per `(route, handler-file)` pair MUST be fixed-size and bounded (default =
CPU core count) — no per-request process spawning, no unbounded pools, and no pool shared
across routes even if they reference the same handler file (research.md §5t). A worker crash
or stuck call on one route MUST NOT crash or block the Go process, nor affect any other route
(FR-006).

**Scale/Scope**: A single gateway process now serves multiple named routes (a low-to-moderate
number, e.g. tens) defined in one `--config` YAML file; each route independently gets the same
handler-chaining and isolation guarantees a whole process used to get. Running multiple
gateway processes (e.g. for horizontal scaling, or separate listen ports) is still supported,
but is no longer the only way to add a second destination. Any runtime route/handler-management
API is explicitly out of scope (FR-011: startup-config-only).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|---|---|---|
| I. Code Quality | Config parsing, path-based routing, proxy, chain-execution, and worker-pool supervision are separate, single-responsibility Go packages (`internal/config`, `internal/router`, `internal/proxy`, `internal/handler`). `internal/router` is genuinely warranted now (not the speculative abstraction it would have been before this request) — it exists because the feature now requires matching a request to one of several named routes. Linting (`golangci-lint` for Go, `ruff` for Python) gates CI; PR review required. | PASS |
| II. Testing Standards (NON-NEGOTIABLE) | Three-layer test plan (contract / integration / unit) defined in research.md §6, mapped to specific FRs, now including multi-route and route-not-found scenarios. CI runs the full suite; no merge on red build. | PASS |
| III. User Experience Consistency | One standard error envelope (`contracts/error-format.md`, now including `ROUTE_NOT_FOUND`) used for every handler rejection, failure, and routing miss (FR-007, FR-012), identical across every route and every gateway process. | PASS |
| IV. Performance Requirements (path rewriting) | Rewriting is opt-in per route and runs one linear pass over the already-buffered body for `text/html`/`text/css` only; budget ≤5ms p95 added for a 256 KiB HTML page; bodies >10 MiB skipped. Referer fallback is O(1) (map lookup). | PASS |
| IV. Performance Requirements | Latency/throughput budget stated above before implementation. Route lookup is O(1) regardless of route count. Worker pool is fixed-size and bounded per `(route, handler-file)` pair; every handler call has an enforced timeout. No unbounded queues/caches/pools introduced. | PASS |

No violations — Complexity Tracking table below is empty by design.

## Project Structure

### Documentation (this feature)

```text
specs/001-handler-injection/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
cmd/
└── gateway/
    └── main.go              # Entrypoint: parses --host/--port/--config, starts proxy + per-route pools

internal/
├── config/                  # Parses --host/--port and loads/validates the --config YAML into
│                            # a GatewayConfig (list of RouteConfig) (FR-011)
├── router/                  # Matches an inbound request's first path segment to a RouteConfig,
│                            # strips it before forwarding, 404s (ROUTE_NOT_FOUND) on no match;
│                            # also serves the built-in GET /_ discovery listing (research.md §5u)
├── proxy/                   # Core reverse-proxy: forwards a request to a route's dest
├── rewrite/                 # NEW: pure functions that re-prefix root-relative URLs in HTML/CSS
│                            # bodies and Location/Set-Cookie headers (research.md §5v)
├── handler/
│   ├── chain.go             # Runs one route's request/response handler chain in configured order (FR-005)
│   └── workerpool/          # Supervises the bounded Python worker pool per (route, handler-file)
│                            # pair; speaks the handler.proto contract; enforces timeout +
│                            # fail-closed (FR-006, FR-012)
└── observability/           # Structured logging of handler execution records, now including
                             # which route each record belongs to (FR-008)

api/
└── handler/v1/
    └── handler.proto        # gRPC contract shared by the Go supervisor and Python workers —
                             # unaffected by routing, which is a Go-side-only concern

handlers-sdk/                # Python package handler authors implement against
└── gateway_handler_sdk/
    ├── __init__.py
    ├── server.py            # Boilerplate gRPC server loop; authors implement only the two callbacks
    └── contracts.py          # RequestHandler / ResponseHandler base classes matching handler.proto

tests/
├── contract/                # Go<->Python handler contract tests (handler.proto conformance)
├── integration/             # End-to-end: gateway binary + real Python handler + stub backend,
│                            # per user story, now including multi-route, route-not-found, and
│                            # discovery (GET /_) cases
└── unit/                    # Go unit tests (config, router, proxy, chain, workerpool)

python/
└── tests/                   # pytest suite validating handlers-sdk against handler.proto
```

**Structure Decision**: A single Go-owned deployable (`cmd/gateway`) now owns a small route
table: it parses `--host`/`--port`/`--config`, loads the YAML file into a list of
`RouteConfig`s, and adds one new package, `internal/router`, to match each inbound request to
its route by first path segment (O(1)) and strip that segment before proxying. Everything
downstream of routing is unchanged in shape: `internal/handler` still supervises a bounded
Python worker pool per handler file over the generated gRPC contract
(`api/handler/v1/handler.proto`), just now keyed by `(route, handler-file)` instead of just
`handler-file`, and the Python SDK (`handlers-sdk/`) is untouched. Serving multiple
*processes* (for horizontal scaling or separate listen ports) is still fully supported — this
change adds a route table *within* one process, it doesn't remove the ability to run more than
one process. The two-language, single-project layout is unchanged from before, for the same
reason: FR-011 requires handler wiring resolved entirely from the gateway's own startup
configuration, and an independently deployed handler service would reintroduce the
externally-called-service model the spec explicitly ruled out.

## Complexity Tracking

*No entries — Constitution Check above has no unjustified violations.*
