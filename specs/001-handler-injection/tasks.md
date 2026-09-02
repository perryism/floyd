---

description: "Task list for Custom Request/Response Handler Injection — multi-route + discovery revision"
---

# Tasks: Custom Request/Response Handler Injection (Multi-Route + Discovery Revision)

**Input**: Design documents from `/specs/001-handler-injection/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md) (see
§5r/§5s/§5t/§5u), [data-model.md](./data-model.md), [contracts/](./contracts/),
[quickstart.md](./quickstart.md)

**Supersedes** the original single-destination task list: US1–US3 were fully implemented and
merged (see git history), then the plan was amended twice — multi-route support via a
`--config` YAML file (research.md §5r/§5s/§5t) and a built-in `/_` discovery endpoint
(research.md §5u). This list covers the delta: reworking the existing implementation for
multiple named routes per process, plus the new discovery endpoint.

**Tests**: Per this project's constitution (Testing Standards, NON-NEGOTIABLE), automated tests
covering primary behavior and key edge cases are REQUIRED — every phase below includes them.

**Organization**: Foundational tasks rework shared infrastructure (blocking); US1–US3 phases
re-verify existing behavior still holds under the new router; a new cross-cutting phase covers
multi-route dispatch and discovery, which aren't yet tied to a numbered spec.md user story (see
Notes at the end).

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1/US2/US3 for tasks re-verifying an existing user story; omitted for
  Setup/Foundational/cross-cutting/Polish tasks, per the template's convention
- Paths are relative to the repository root

---

## Phase 1: Setup

- [X] T001 Promote `gopkg.in/yaml.v3` from an indirect to a direct dependency by importing it
  in `internal/config` (T002) and running `go mod tidy` — it's already present transitively
  (pulled in via another dependency) per `go.mod`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Rework config loading and add path-based routing + discovery — every route's
request handling depends on this

**⚠️ CRITICAL**: No user-story or cross-cutting work below can start until this phase is done

- [X] T002 Rewrite `internal/config/config.go`: replace the `--dest` flag and repeatable
  `--handlers` flag with `--host`, `--port`, `--config <path>`; replace `GatewayConfig.Dest`/
  `.Handlers` with `GatewayConfig.Routes []RouteConfig`; define
  `RouteConfig{Name, Description string; Dest *url.URL; Handlers []HandlerRef}`; load and
  YAML-unmarshal the `--config` file into `[]RouteConfig` (data-model.md `GatewayConfig`/
  `RouteConfig`, research.md §5r, depends on T001)
- [X] T003 [P] In `internal/config`, give a route's `handlers` YAML key a custom
  `UnmarshalYAML` so it accepts either one scalar path or a list of paths, normalized into an
  ordered `[]HandlerRef` (`Position` = list index, `TimeoutMS` = `DefaultHandlerTimeoutMS`
  unless overridden) — same semantics as the old repeatable `--handlers` flag, scoped per route
  (data-model.md `HandlerRef`, research.md §5r)
- [X] T004 In `internal/config/config.go`, add validation, failing startup with a descriptive
  error on: `--config` missing/unparseable; an empty `routes` list; a `RouteConfig.name` that's
  empty, duplicated (case-sensitive), contains `/`, or equals `_` (reserved for discovery,
  research.md §5u); a malformed `dest`; an unloadable `handlers` file (FR-011, data-model.md
  `GatewayConfig`/`RouteConfig` validation rules, depends on T002)
- [X] T005 Create `internal/router/router.go`: an `http.Handler` that (a) for the exact path
  `/_`, on any HTTP method, writes the discovery JSON — every route's `name`/`description` in
  config order, excluding `dest`/`handlers` (contracts/discovery.md) — before any route
  matching happens; (b) otherwise matches the request's first path segment against configured
  route names via an O(1) map lookup; (c) on a match, strips that segment (rewriting to `/` if
  nothing remains) and delegates to that route's own `http.Handler`; (d) on no match (including
  a request to `/` itself), writes `ROUTE_NOT_FOUND` (404) via the standard error envelope
  (research.md §5s/§5u, data-model.md "Routing", depends on T002)
- [X] T006 Update `internal/observability/errors.go`: add
  `CodeRouteNotFound = "ROUTE_NOT_FOUND"` mapped to HTTP 404; add a `Route string`
  (`json:"route,omitempty"`) field and make `Handler` (`json:"handler,omitempty"`) optional too,
  so `ROUTE_NOT_FOUND` can omit both; update `WriteError`'s signature to take a route name
  (contracts/error-format.md, contracts/discovery.md)
- [X] T007 [P] Update `internal/observability/log.go`: add a `Route string` (`json:"route"`)
  field to `HandlerExecutionRecord` (data-model.md `HandlerExecutionRecord`)
- [X] T008 Update `internal/handler/chain.go`: add a route name to `Runner` (change
  `NewRunner(forwarder *proxy.Forwarder, handlers []Handler) *Runner` to
  `NewRunner(routeName string, forwarder *proxy.Forwarder, handlers []Handler) *Runner`) and
  thread it through every `logInvocation`/`observability.WriteError` call site so records and
  errors carry the correct route (FR-006 at the per-route grain, depends on T006, T007)
- [X] T009 Rewrite `cmd/gateway/main.go`: parse `--host`/`--port`/`--config` (T002); for each
  `RouteConfig`, start one `workerpool.Pool` per `HandlerRef` (name it `"<route>/<handler>"`
  for clear logs — pools are already isolated per route simply by looping per `RouteConfig`,
  since each loop iteration creates fresh `*workerpool.Pool` instances; no `workerpool` package
  change needed, per research.md §5t); build that route's `handler.Runner` (T008); assemble an
  `internal/router.Router` (T005) covering every route plus discovery; listen on
  `--host`:`--port` (depends on T002, T005, T008)

**Checkpoint**: the gateway builds, loads a multi-route YAML config, and dispatches requests
through the new router with correct per-route observability — ready to re-verify US1–US3

---

## Phase 3: User Story 1 — Register a custom request handler (Priority: P1), re-verified under multi-route

**Goal**: Confirm request-handler injection (spec.md US1) still works exactly as before, now
routed through a named route instead of a process's single implicit destination

- [X] T010 [P] [US1] Rewrite `tests/testutil/gateway.go`'s `NewTestGateway`: replace the
  `(t, backend *httptest.Server, refs []config.HandlerRef)` signature with one that builds a
  full multi-route `internal/router.Router` from a list of
  `{Name string; Backend *httptest.Server; Handlers []config.HandlerRef}` entries (depends on
  T005, T008, T009's shapes)
- [X] T011 [US1] Update `tests/integration/request_handler_test.go`'s
  `TestRequestHandler_AddHeader` and `TestRequestHandler_RejectMissingField` to the new
  `testutil.NewTestGateway` signature and route-prefixed URLs (e.g. `gw.URL + "/test/anything"`
  instead of `gw.URL + "/anything"`); confirm both still pass (spec.md US1 Acceptance Scenarios
  1–2, depends on T010)

**Checkpoint**: US1 re-verified end-to-end through the new router

---

## Phase 4: User Story 2 — Register a custom response handler (Priority: P2), re-verified under multi-route

- [X] T012 [P] [US2] Update `tests/integration/response_handler_test.go`'s
  `TestResponseHandler_RemoveField` and `TestResponseHandler_AddHeader` to the new
  `testutil.NewTestGateway` signature and route-prefixed URLs; confirm both still pass (spec.md
  US2 Acceptance Scenarios 1–2, depends on T010)
- [X] T013 [P] [US2] Spot-check `tests/contract/process_response_test.go` for any dependency on
  removed `config`/`workerpool` shapes — the Go↔Python contract itself doesn't change with
  routing, so this should need no code changes, only confirmation it still passes

**Checkpoint**: US2 re-verified end-to-end through the new router

---

## Phase 5: User Story 3 — Chain multiple handlers and fail gracefully (Priority: P3), re-verified and extended for multi-route isolation

- [X] T014 [P] [US3] Update `tests/integration/handler_chain_test.go`'s
  `TestHandlerChain_Order` and `handler_failure_test.go`'s
  `TestHandlerFailure_IsolatesAndKeepsServing`/`TestHandlerFailure_Timeout` to the new
  `testutil.NewTestGateway` signature and route-prefixed URLs; confirm all still pass (spec.md
  US3 Acceptance Scenarios 1–2, depends on T010)
- [X] T015 [P] [US3] New integration test `tests/integration/handler_isolation_multiroute_test.go`:
  configure two routes that both reference the *same* handler file (e.g.
  `tests/fixtures/always_fail.py`); confirm one route's requests fail closed while the other
  route's requests — through the same file — keep succeeding, proving worker pools are scoped
  per `(route, file)`, not shared (research.md §5t, extends FR-006, depends on T009, T010)

**Checkpoint**: US3 re-verified, including the new per-route pool isolation guarantee

---

## Phase 6: Multi-Route Dispatch & Discovery (cross-cutting — see Notes)

**Goal**: Verify the two new capabilities this plan revision actually added: serving several
independent routes from one process, and discovering them via `/_`

**Independent Test**: Configure two routes with two distinct backends; confirm each is reached
independently at its own path prefix, an unconfigured prefix 404s, and `/_` lists both by name
and description.

- [X] T016 [P] Integration test `tests/integration/multiroute_test.go`: two routes, two
  distinct stub backends, one gateway process — confirm `/routeA/...` and `/routeB/...` each
  reach their own backend with the matched segment stripped (`/routeA/foo` → backend `/foo`),
  independently of each other (research.md §5s, depends on T009, T010)
- [X] T017 [P] Integration test for `GET /_` (and one other method, e.g. `POST /_`, to confirm
  method-independence) returning the discovery JSON — every configured route's `name` and
  `description`, in config order, with `dest`/`handlers` absent (contracts/discovery.md,
  depends on T009)
- [X] T018 [P] Integration test for `ROUTE_NOT_FOUND`: a request to an unconfigured first path
  segment, and a request to `/` itself, both return 404 with the standard error envelope and
  `error.route`/`error.handler` omitted (contracts/error-format.md, depends on T005, T006)
- [X] T019 [P] Rewrite `tests/unit/config_test.go` for the new YAML-based `Parse`: a valid
  multi-route file; missing/unparseable `--config`; an empty `routes` list; a duplicate route
  `name`; a `name` containing `/`; a `name` equal to `_`; a malformed `dest`; an unloadable
  `handlers` file; `handlers` given as a scalar string vs. as a list, both normalized correctly
  with `Position` in list order (depends on T002, T003, T004)
- [X] T020 [P] New unit test `tests/unit/router_test.go` for `internal/router`: first-segment
  matching; prefix-stripping including the "nothing remains → `/`" case; discovery taking
  precedence over route matching; `ROUTE_NOT_FOUND` on no match (depends on T005)

**Checkpoint**: multi-route dispatch and discovery are fully covered, independently of US1–US3

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T021 [P] Update `README.md`'s "Running the gateway" section: replace the removed
  `--dest`/`--handlers` example with `--host`/`--port`/`--config` and a `config.yaml` snippet,
  matching `quickstart.md`
- [X] T022 [P] Update `tests/integration/performance_test.go` to the new
  `testutil.NewTestGateway` signature; confirm the ≤5ms/≤20ms p95 budgets (SC-004) still hold
  with route matching in the hot path (depends on T010)
- [X] T023 [P] Run `tests/unit/workerpool_security_test.go` unmodified and confirm it still
  passes — the `workerpool.Pool` API itself is unchanged by this revision
- [X] T024 Run `make lint` (`go vet` + `golangci-lint` + `ruff`) and `make test` end-to-end;
  fix anything the rework surfaces

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS everything below
- **US1/US2/US3 (Phases 3–5)**: All depend on Foundational, and specifically on T010 (the
  rewritten test harness) before their test updates can compile
- **Cross-cutting (Phase 6)**: Depends on Foundational; independent of US1–US3, so it can run
  in parallel with Phases 3–5 if staffed separately
- **Polish (Phase 7)**: Depends on everything above being in place

### Within Foundational

T002 → T003, T004 (same file, `internal/config/config.go`) must be sequential; T005 (new file)
and T007 (different file) can run in parallel with T002's early work but T008/T009 depend on
T005/T006/T007/T002 all landing first.

### Parallel Opportunities

- T003 can run alongside T005–T007 (different packages)
- T007 (log.go) can run alongside T006 (errors.go) — different files in the same package, no
  shared state
- Once T010 lands, T011/T012/T014 (US1/US2/US3 test updates) can all run in parallel — each
  touches a different test file
- All of Phase 6 (T016–T020) can run in parallel with Phases 3–5 once Foundational is done

---

## Implementation Strategy

### Minimum to unblock everything

1. Complete Phase 1 (trivial) and Phase 2 (Foundational) — this is the actual rework; nothing
   else compiles against the new shapes until it's done
2. Land T010 (test harness rewrite) immediately after — every other test file depends on it

### Then, in any order / in parallel

3. Re-verify US1, US2, US3 (Phases 3–5) — mechanical updates confirming nothing regressed
4. Build out Phase 6 — the actual new capabilities this revision was for
5. Phase 7 to close out (docs, performance/security re-confirmation, full lint+test pass)

---

## Notes

- Phase 6's tasks aren't tied to a numbered spec.md user story: multi-route support and the
  `/_` discovery endpoint were introduced directly through two `/speckit-plan` amendments, not
  through `/speckit-specify`/`/speckit-clarify`. If you want them as formally traceable as
  US1–US3, consider a follow-up spec amendment (e.g. an FR-013 and a short user story) — not
  done here since it wasn't asked for.
- Every existing US1/US2/US3 integration test needs a route-name prefix added to its request
  URLs — there is no more "no route configured" mode; every request now needs a route to match.
- [P] tasks touch different files with no unmet dependency — safe to parallelize
- Verify each REQUIRED test fails (or doesn't compile) before the rework, and passes after
- Commit after each phase or logical group
