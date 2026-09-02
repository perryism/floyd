# Implementation Plan: Custom Request/Response Handler Injection

**Branch**: `001-handler-injection` | **Date**: 2026-09-01 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-handler-injection/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Let API teams customize request and response handling — validate/transform inbound requests,
transform/filter outbound responses — without touching their backend service. The gateway core
is a Go proxy launched as a single-destination CLI process (`api-gateway --dest <backend>
--host <listen-host> --port <listen-port> --handlers <file.py>`); handlers are written in
Python. The Go process supervises a bounded pool of long-lived Python worker processes per
`--handlers` file (resolved from CLI flags at startup, not a runtime API) and calls them over a
local gRPC channel with a per-call timeout, failing closed on any handler error or timeout so
one broken handler can't take down this instance's traffic.

## Technical Context

**Language/Version**: Go 1.23 (gateway core); Python 3.11+ (request/response handlers)

**Primary Dependencies**: Go: standard `net/http`, standard `flag` (CLI parsing — the flag set
is small enough that a third-party CLI framework would be unjustified complexity per the
Code Quality principle), `google.golang.org/grpc`, `google.golang.org/protobuf`. Python:
`grpcio` + generated stubs from the shared `.proto` contract, packaged behind a small
`handlers-sdk` so handler authors implement two callbacks rather than hand-writing gRPC
boilerplate.

**Storage**: N/A — all configuration is CLI flags resolved once at process startup; no config
file, no database.

**Testing**: Go: `go test` for unit/integration/contract suites. Python: `pytest` for the
handler SDK's own contract tests.

**Target Platform**: Linux server, containerized deployment; one process per upstream
destination (sidecar/per-service proxy pattern).

**Project Type**: Single CLI-launched web-service (network proxy) plus a small Python SDK
package consumed by handler authors.

**Performance Goals**: Baseline proxy (zero `--handlers` configured) adds ≤5ms p95 over direct
backend latency. Each configured handler adds ≤20ms p95 over the local gRPC hop (matches
SC-004). Reference target throughput ≥5,000 req/s per gateway instance on 4 vCPU — a default
pending real load-test data (see Complexity Tracking: none required, but flagged here for
follow-up during implementation).

**Constraints**: Every handler call MUST carry a bounded, configurable timeout (default 100ms)
enforced via gRPC deadline; on timeout or error the gateway MUST fail closed (FR-012). The
Python worker pool per `--handlers` file MUST be fixed-size and bounded (default = CPU core
count) — no per-request process spawning, no unbounded pools. A worker crash or stuck call MUST
NOT crash or block the Go process (FR-006).

**Scale/Scope**: A single gateway process proxies to exactly one destination (`--dest`); serving
multiple destinations means running multiple gateway processes, each independently configured.
Each process supports a small number (≤5, default assumption) of chained handlers per direction
via repeated `--handlers` flags. Any runtime handler-management API is explicitly out of scope
(FR-011: startup-flag-only).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|---|---|---|
| I. Code Quality | CLI-flag parsing, proxy, chain-execution, and worker-pool supervision are separate, single-responsibility Go packages (`internal/config`, `internal/proxy`, `internal/handler`); no speculative abstraction beyond what FR-001–FR-012 require — no route table, since a process serves exactly one destination. Linting (`golangci-lint` for Go, `ruff` for Python) gates CI; PR review required. | PASS |
| II. Testing Standards (NON-NEGOTIABLE) | Three-layer test plan (contract / integration / unit) defined in research.md §6, mapped to specific FRs. CI runs the full suite; no merge on red build. | PASS |
| III. User Experience Consistency | One standard error envelope (`contracts/error-format.md`) used for every handler rejection and failure (FR-007, FR-012), identical across every gateway process — no per-instance custom error shape. | PASS |
| IV. Performance Requirements | Latency/throughput budget stated above before implementation. Worker pool is fixed-size and bounded; every handler call has an enforced timeout. No unbounded queues/caches/pools introduced. | PASS |

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
    └── main.go              # Entrypoint: parses --dest/--host/--port/--handlers, starts proxy + pools

internal/
├── config/                  # Parses and validates CLI flags into a GatewayConfig (FR-011)
├── proxy/                   # Core reverse-proxy: forwards every request to --dest
├── handler/
│   ├── chain.go             # Runs the configured request/response handler chain in flag order (FR-005)
│   └── workerpool/          # Supervises the bounded Python worker pool per --handlers file; speaks
│                            # the handler.proto contract; enforces timeout + fail-closed (FR-006, FR-012)
└── observability/           # Structured logging of handler execution records (FR-008)

api/
└── handler/v1/
    └── handler.proto        # gRPC contract shared by the Go supervisor and Python workers

handlers-sdk/                # Python package handler authors implement against
└── gateway_handler_sdk/
    ├── __init__.py
    ├── server.py            # Boilerplate gRPC server loop; authors implement only the two callbacks
    └── contracts.py          # RequestHandler / ResponseHandler base classes matching handler.proto

tests/
├── contract/                # Go<->Python handler contract tests (handler.proto conformance)
├── integration/             # End-to-end: gateway binary + real Python handler + stub backend, per user story
└── unit/                    # Go unit tests (config, proxy, chain, workerpool)

python/
└── tests/                   # pytest suite validating handlers-sdk against handler.proto
```

**Structure Decision**: A single Go-owned deployable (`cmd/gateway`), launched as a plain CLI
process with no route table — it parses `--dest`/`--host`/`--port`/`--handlers` and proxies
every request it receives to that one destination, running the configured handler chain around
it. An internal handler subsystem (`internal/handler`) supervises a bounded Python worker pool
over a generated gRPC contract (`api/handler/v1/handler.proto`). A thin Python SDK
(`handlers-sdk/`) gives handler authors two base classes to implement, hiding the gRPC
boilerplate. Serving multiple destinations means running multiple instances of this same binary,
each with its own flags — not adding a route table to one process. This single-project,
two-language layout was chosen over splitting the Python handlers into an independently deployed
service because FR-011 requires handler wiring to be resolved entirely from the gateway's own
startup configuration — an independently deployed handler service would reintroduce the
externally-called-service model the spec explicitly ruled out.

## Complexity Tracking

*No entries — Constitution Check above has no unjustified violations.*
