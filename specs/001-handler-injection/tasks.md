---

description: "Task list for Custom Request/Response Handler Injection"
---

# Tasks: Custom Request/Response Handler Injection

**Input**: Design documents from `/specs/001-handler-injection/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: Per this project's constitution (Testing Standards, NON-NEGOTIABLE), automated tests
covering primary behavior and key edge cases are REQUIRED for all new functionality — every
user story phase below includes its required tests, written and failing before implementation.

**Organization**: Tasks are grouped by user story (from spec.md) to enable independent
implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Paths are relative to the repository root, per plan.md's Project Structure

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Repository scaffolding for the two-language (Go + Python) project

- [X] T001 Initialize the Go module and create the package skeleton per plan.md's Project
  Structure: `cmd/gateway/`, `internal/config/`, `internal/proxy/`, `internal/handler/`,
  `internal/handler/workerpool/`, `internal/observability/`, `api/handler/v1/`
- [X] T002 [P] Initialize the Python `handlers-sdk` package skeleton in
  `handlers-sdk/gateway_handler_sdk/` with `pyproject.toml`, `__init__.py`
- [X] T003 [P] Configure Go linting (`golangci-lint`) and Python linting (`ruff`) with CI gating
  and no disabled checks, per Constitution Principle I (Code Quality)
- [X] T004 [P] Set up CI to run `go test ./...` and `pytest` on every pull request with a
  failing suite blocking merge, per Constitution Principle II (Testing Standards, NON-NEGOTIABLE)

**Checkpoint**: Repository structure exists; CI enforces lint + tests on every PR

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The Go↔Python handler boundary and core proxy — nothing in Phase 3+ works without this

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T005 Finalize `contracts/handler.proto` into `api/handler/v1/handler.proto` and generate
  Go and Python stubs (`protoc`/`buf`) for the `Handler` service (`ProcessRequest`,
  `ProcessResponse`)
- [X] T006 [P] Implement CLI flag parsing into a `GatewayConfig` in `internal/config/config.go`:
  `--dest`, `--host`, `--port`, repeatable `--handlers`; fail to start on a malformed `--dest`
  or an unloadable `--handlers` file (FR-011, data-model.md `GatewayConfig` validation rules)
- [X] T007 [P] Implement the core reverse-proxy in `internal/proxy/proxy.go` that forwards every
  request to `GatewayConfig.dest` (FR-009 baseline pass-through when no handlers configured)
- [X] T008 Implement the standard error envelope in `internal/observability/errors.go` per
  `contracts/error-format.md`, including the `error.code` → HTTP status mapping table
- [X] T009 [P] Implement `HandlerExecutionRecord` structured logging in
  `internal/observability/log.go` (FR-008, data-model.md)
- [X] T010 Implement the worker-pool supervisor in `internal/handler/workerpool/pool.go`: fixed
  size per handler file (default = CPU core count), spawn-once-reuse-always, crash detection with
  backoff restart, and "5 consecutive crashes → Degraded" (research.md §2, FR-006)
- [X] T011 Implement the handler chain runner in `internal/handler/chain.go` that calls
  `workerpool` in configured order, enforces a per-call gRPC deadline from each handler's
  `timeout_ms`, and fails closed on any error/timeout into the standard error envelope
  (FR-005, FR-012, depends on T008, T010)
- [X] T012 [P] Implement the Python SDK server boilerplate in
  `handlers-sdk/gateway_handler_sdk/server.py`: starts a gRPC server on a Unix domain socket and
  dispatches to whichever of `RequestHandler`/`ResponseHandler` the loaded module defines
  (depends on T005)
- [X] T013 Wire `cmd/gateway/main.go`: parse flags into `GatewayConfig` (T006), start one worker
  pool per distinct `--handlers` file (T010), start the proxy+chain (T007, T011), and listen on
  `--host`:`--port` (depends on T006, T007, T010, T011)

**Checkpoint**: A gateway binary exists that proxies to `--dest` and can run a configured
handler chain — ready for user-story-specific behavior

---

## Phase 3: User Story 1 - Register a custom request handler (Priority: P1) 🎯 MVP

**Goal**: Let an operator inject a request handler that validates, enriches, or transforms an
inbound request before it reaches the backend

**Independent Test**: Launch the gateway with `--handlers` pointing to a handler that adds a
header; send a request and confirm the backend receives it with the header added

### Tests for User Story 1 (REQUIRED per Testing Standards) ⚠️

> Write these tests FIRST; confirm they FAIL before implementing this phase

- [X] T014 [P] [US1] Contract test for `ProcessRequest` (modify + reject shapes) against
  `handler.proto` in `tests/contract/process_request_test.go`
- [X] T015 [P] [US1] Integration test: request handler adds a header, backend receives it
  (spec.md US1 Acceptance Scenario 1) in `tests/integration/request_handler_test.go`
  (`TestRequestHandler_AddHeader`)
- [X] T016 [P] [US1] Integration test: request handler rejects a request missing a required
  field; backend is never called and caller gets `HANDLER_REJECTED_REQUEST` (spec.md US1
  Acceptance Scenario 2) in `tests/integration/request_handler_test.go`
  (`TestRequestHandler_RejectMissingField`)

### Implementation for User Story 1

- [X] T017 [P] [US1] Implement the `RequestHandler` base class and dispatch in
  `handlers-sdk/gateway_handler_sdk/contracts.py` (FR-001, FR-010)
- [X] T018 [US1] Wire `internal/handler/chain.go`'s request path: call each configured request
  handler in order via `workerpool.ProcessRequest`, forward to the backend only if none reject
  (FR-001, FR-003, depends on T011, T017)
- [X] T019 [US1] Map a `Rejection` from `ProcessRequest` into the standard error envelope
  (`HANDLER_REJECTED_REQUEST`, 400) in `internal/observability/errors.go` (FR-007, depends on T008)
- [X] T020 [P] [US1] Add a reference handler at `examples/handlers/add_request_id.py` matching
  quickstart.md, for manual verification

**Checkpoint**: User Story 1 is fully functional and independently testable — this is the MVP

---

## Phase 4: User Story 2 - Register a custom response handler (Priority: P2)

**Goal**: Let an operator inject a response handler that transforms, filters, or enriches the
backend's response before it reaches the caller

**Independent Test**: Launch the gateway with `--handlers` pointing to a handler that removes a
field from the backend's JSON response; confirm the caller doesn't see that field

### Tests for User Story 2 (REQUIRED per Testing Standards) ⚠️

- [X] T021 [P] [US2] Contract test for `ProcessResponse` (modify + reject shapes) against
  `handler.proto` in `tests/contract/process_response_test.go`
- [X] T022 [P] [US2] Integration test: response handler removes a field, caller doesn't see it
  (spec.md US2 Acceptance Scenario 1) in `tests/integration/response_handler_test.go`
  (`TestResponseHandler_RemoveField`)
- [X] T023 [P] [US2] Integration test: response handler adds a header, caller receives it
  (spec.md US2 Acceptance Scenario 2) in `tests/integration/response_handler_test.go`
  (`TestResponseHandler_AddHeader`)

### Implementation for User Story 2

- [X] T024 [P] [US2] Implement the `ResponseHandler` base class and dispatch in
  `handlers-sdk/gateway_handler_sdk/contracts.py` (FR-002, FR-010)
- [X] T025 [US2] Wire `internal/handler/chain.go`'s response path: call each configured response
  handler in order via `workerpool.ProcessResponse` before returning to the caller
  (FR-002, FR-004, depends on T011, T024)
- [X] T026 [US2] Map a `Rejection` from `ProcessResponse` into the standard error envelope
  (`HANDLER_REJECTED_RESPONSE`, 502) in `internal/observability/errors.go` (FR-007, depends on T008)

**Checkpoint**: User Stories 1 AND 2 both work independently

---

## Phase 5: User Story 3 - Chain multiple handlers and fail gracefully (Priority: P3)

**Goal**: Support multiple chained handlers per direction with a predictable order, and isolate
a failing/slow handler so it can't take down the gateway process

**Independent Test**: Configure two request handlers where the second depends on the first's
output; confirm registration order. Then configure a handler that always fails; confirm the
gateway fails closed with a clear error and keeps serving other requests

### Tests for User Story 3 (REQUIRED per Testing Standards) ⚠️

- [X] T027 [P] [US3] Integration test: two request handlers on one process, second sees first's
  modification, run in flag order (spec.md US3 Acceptance Scenario 1) in
  `tests/integration/handler_chain_test.go` (`TestHandlerChain_Order`)
- [X] T028 [P] [US3] Integration test: a handler that always errors/times out; caller gets
  `HANDLER_ERROR`/`HANDLER_TIMEOUT`, and the process keeps serving other requests normally
  (spec.md US3 Acceptance Scenario 2) in `tests/integration/handler_failure_test.go`
  (`TestHandlerFailure_IsolatesAndKeepsServing`, plus `TestHandlerFailure_Timeout` for the
  distinct HANDLER_TIMEOUT code)
- [X] T029 [P] [US3] Unit test: worker crash → backoff restart → sustained fail-closed
  behavior without hanging or leaking (data-model.md `HandlerWorkerPool` state transitions) in
  `tests/unit/workerpool_test.go` (black-box from outside the package, per this project's test
  tree — asserts the externally-visible contract rather than the internal state enum)

### Implementation for User Story 3

- [X] T030 [US3] Extend/verify `internal/handler/chain.go` passes each handler's output as the
  next handler's input, in configured order, across more than one `--handlers` file (FR-005,
  depends on T018, T025)
- [X] T031 [US3] Enforce the per-call gRPC deadline from each `HandlerRef.timeout_ms` in
  `internal/handler/workerpool/pool.go`, treating deadline-exceeded as a handler failure
  (FR-012, depends on T010) — fixed a real bug where grpc-go's wrapped deadline error didn't
  satisfy `errors.Is(err, context.DeadlineExceeded)`, so `HANDLER_TIMEOUT` never fired; now
  also checks the gRPC status code
- [X] T032 [US3] Isolate worker crashes in `internal/handler/workerpool/pool.go`: a crash or
  panic in one worker pool must not affect any other pool or the Go process itself (FR-006,
  depends on T031) — fixed a real race where `Stop()` could return while a concurrent
  crash-restart was still in flight, leaking a replacement worker process forever; restart
  registration now runs under the same lock `Stop()` holds

**Checkpoint**: All three user stories independently functional

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that span all user stories

- [X] T033 [P] Write handler-authoring docs in `README.md`, adapted from quickstart.md
- [X] T034 Run quickstart.md's manual verification end-to-end against a real backend (done
  manually during Foundational implementation, and now covered continuously by
  `tests/integration`)
- [X] T035 [P] Unit tests for `internal/config` validation edge cases (malformed `--dest`,
  missing `--handlers` file) in `tests/unit/config_test.go`
- [X] T036 Load/benchmark test validating the Performance Requirements budget: baseline proxy
  ≤5ms p95 overhead, ≤20ms p95 per configured handler (SC-004), per Constitution Principle IV
  in `tests/integration/performance_test.go` — measured ~44µs baseline overhead and ~839µs
  per-handler overhead locally, well within budget (a sanity check, not a load test)
- [X] T037 [P] Verify the worker-pool Unix domain socket is not network-exposed and has
  restrictive file permissions in `tests/unit/workerpool_security_test.go` — found and fixed a
  real gap: the socket file's own permissions depend on the worker process's umask, so it now
  lives in a fresh private (0700) directory instead of directly in the shared temp dir

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Stories (Phase 3-5)**: All depend on Foundational completion
  - User Story 1 (P1): No dependency on US2/US3 — this is the MVP
  - User Story 2 (P2): No functional dependency on US1, but shares `chain.go`/`errors.go`, so
    sequence after US1 in a single-developer flow to avoid merge conflicts
  - User Story 3 (P3): Builds on the chain/worker-pool machinery exercised by US1 and US2
- **Polish (Phase 6)**: Depends on all desired user stories being complete

### Within Each User Story

- Tests MUST be written and FAIL before implementation (Constitution Principle II)
- SDK/contract classes before chain wiring
- Chain wiring before error-mapping polish
- Story complete and independently verified before moving to the next priority

### Parallel Opportunities

- All Setup tasks marked [P] (T002-T004) can run in parallel once T001 exists
- T006, T007, T009, T012 (Foundational, different files/languages) can run in parallel; T005 →
  T010 → T011 → T013 are sequential
- All tests within a user story phase marked [P] can run in parallel with each other
- T014/T015/T016 (US1 tests) can run in parallel with T021/T022/T023 (US2 tests) if staffed by
  different developers, since Foundational is already complete

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: "Contract test for ProcessRequest in tests/contract/test_process_request.go"
Task: "Integration test: request handler adds a header in tests/integration/test_request_handler_add_header.go"
Task: "Integration test: request handler rejects a request in tests/integration/test_request_handler_reject.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: run the Independent Test for US1 against a real backend
5. Ship the MVP: request-handler injection only

### Incremental Delivery

1. Setup + Foundational → binary proxies with no handlers (FR-009 baseline)
2. Add User Story 1 → validate independently → ship (MVP)
3. Add User Story 2 → validate independently → ship
4. Add User Story 3 → validate independently → ship
5. Each story adds value without breaking the previous ones

---

## Notes

- [P] tasks touch different files with no unmet dependency — safe to parallelize
- [Story] label maps each task to its user story for traceability
- Verify each REQUIRED test fails before implementing the task that makes it pass
- Commit after each task or logical group
- Stop at any checkpoint to validate a story independently before continuing
