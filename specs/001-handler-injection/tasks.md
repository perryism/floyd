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

## Phase 8: Config Schema — Nest Routes Under `endpoints`, Support Top-Level `verbose`

**Goal**: Change the `--config` YAML file's top-level shape from a bare list of routes to a
map with an `endpoints` key (aligning it with the `/_` discovery response's existing
`endpoints` key, contracts/discovery.md) so additional top-level keys can be added later; add
`verbose` as the first such key, purely as a demonstration that the top level now accepts more
than one key — no verbose-logging *behavior* is implemented here, since none was requested
beyond the schema change itself

**Independent Test**: Point `--config` at a file with `verbose: true` and an `endpoints:` list;
confirm the gateway starts and serves those routes exactly as before. Confirm a file that omits
`verbose` still starts (defaults to `false`), and a file with an empty or missing `endpoints`
key fails to start with the same "no routes configured" error as before.

- [X] T025 In `internal/config/config.go`, replace the top-level YAML unmarshal target: add
  `type yamlConfig struct { Verbose bool \`yaml:"verbose"\`; Endpoints []yamlRoute
  \`yaml:"endpoints"\` }`, rename `loadRoutes` to `loadConfig` and change it to unmarshal into
  `yamlConfig` instead of `var raw []yamlRoute`, returning `([]RouteConfig, bool, error)` (routes
  plus the verbose flag); add `GatewayConfig.Verbose bool` and populate it from `loadConfig`'s
  result in `Parse` (data-model.md `GatewayConfig`)
- [X] T026 In `internal/config/config.go`'s `loadConfig`, update the empty-list check from
  `len(raw) == 0` to `len(raw.Endpoints) == 0` and adjust its error message from "defines no
  routes" to "defines no endpoints" (depends on T025)
- [X] T027 [P] Update `examples/config.yaml`: nest the three existing route entries under an
  `endpoints:` key (indent each `- name: ...` entry one level), and add a top-level `verbose:
  true` line above it demonstrating the new key (depends on T025)
- [X] T028 [P] Update `tests/unit/config_test.go`: re-indent every `writeConfigFile` YAML
  fixture under an `endpoints:` key (all tests except
  `TestConfig_Parse_UnparseableConfigFile`, whose fixture is deliberately invalid YAML and
  needs no change), including `TestConfig_Parse_EmptyRoutesList`'s fixture becoming `endpoints:
  []`; add `TestConfig_Parse_VerboseFlag` asserting `verbose: true` sets `cfg.Verbose == true`
  and that omitting `verbose` entirely defaults it to `false` (depends on T025, T026)
- [X] T029 [P] Update the config.yaml snippets in `README.md` (~lines 16-25) and
  `specs/001-handler-injection/quickstart.md` (~lines 26-35) to the new nested `endpoints:`
  shape, and add `verbose` to each doc's field-by-field description list (depends on T025)
- [X] T030 [P] Update `specs/001-handler-injection/data-model.md`'s `GatewayConfig` table: add a
  `verbose` field row (`bool`, defaults to `false`), and note the `--config` file's top-level
  shape is now `{verbose?: bool, endpoints: list[RouteConfig]}` rather than a bare list,
  matching contracts/discovery.md's `endpoints` key (depends on T025)
- [X] T031 Run `make lint` and `make test` to confirm the schema change compiles and every
  existing and new config test passes (depends on T025-T030)

**Checkpoint**: `--config` files use the nested `endpoints:` shape and accept a top-level
`verbose` key; example, docs, and tests are aligned to the new shape

---

## Phase 9: Verbose Request Logging

**Goal**: Wire up the `verbose` config flag added in Phase 8 (there, deliberately parsed but
left unused) — when true, log each request's method, URL path, and final response status code
to the gateway's standard log output, for every request the gateway serves (a matched route,
`ROUTE_NOT_FOUND`, discovery, and a handler rejection/error alike, since they all ultimately
write a response through the same top-level handler)

**Independent Test**: Start the gateway with `verbose: true` and send a request; confirm a log
line appears containing the request's method, path, and status code. Start it with `verbose:
false` (or omitted) and confirm no such line appears while the request still succeeds normally.

- [X] T032 In `internal/observability`, add `requestlog.go`: a `LogVerboseRequest(method, path
  string, status int)` function (mirrors `LogHandlerExecution`'s seam pattern in log.go) that
  writes one log line via the standard `log` package, e.g. `api-gateway: GET /app1/foo -> 200`
- [X] T033 In `internal/observability/requestlog.go`, add `VerboseRequestLogger(next
  http.Handler) http.Handler`: middleware that wraps the `http.ResponseWriter` to capture the
  status code passed to `WriteHeader` (defaulting to 200 if `next` never calls it explicitly),
  calls `next.ServeHTTP`, then calls `LogVerboseRequest` with `r.Method`, `r.URL.Path`, and the
  captured status — reading `r.URL.Path` before `next` runs, since `internal/router.Router`
  clones the request before stripping the matched segment, so the original request passed to
  this middleware always keeps the un-stripped inbound path (depends on T032)
- [X] T034 In `cmd/gateway/main.go`, wrap `topRouter` with `observability.VerboseRequestLogger`
  when `cfg.Verbose` is true, before assigning `server.Handler`, so every request is logged
  when verbose is on (depends on T033, and on Phase 8's T025 for `GatewayConfig.Verbose`)
- [X] T035 [P] New unit test `tests/unit/requestlog_test.go` for `internal/observability`:
  confirm `VerboseRequestLogger` logs a line containing the method, path, and status for a
  stub `http.HandlerFunc` that writes a given status; confirm it defaults to 200 when the
  wrapped handler never calls `WriteHeader` (depends on T032, T033)
- [X] T036 [P] New integration test `tests/integration/verbose_logging_test.go`: start a test
  gateway with `verbose: true`, redirect the standard logger's output (`log.SetOutput`) to a
  buffer, send a request, and confirm the buffer contains the request's method/path/status;
  confirm no such line appears when `verbose` is omitted (depends on T034)
- [X] T037 Update the `verbose` field description added in Phase 8's T029 (`README.md`,
  `specs/001-handler-injection/quickstart.md`) to state what it now actually does: logs each
  request's method, path, and response status to the gateway's standard log output (depends on
  T034)

**Checkpoint**: `verbose: true` produces one log line per request with method, path, and status
code; `verbose` omitted/false produces none, and request handling itself is unchanged either way

---

## Phase 10: Single-Endpoint Root Mounting (no subpath prefix)

**Goal**: Support an optional top-level `single_endpoint: <name>` config key that, when set,
mounts that one endpoint at the gateway's root — every path except the reserved `/_` discovery
endpoint forwards straight to that route's handler chain and backend with the path unchanged
(no first-segment stripping), instead of requiring a `/<name>/...` prefix. Restricted to the
"exactly one endpoint" case the user described: the gateway fails to start if `single_endpoint`
is set while `endpoints` has more than one entry, or if its value doesn't match that sole
entry's `name` — promoting one endpoint to root *among several* configured routes is a
different, unrequested feature and is deliberately not supported here.

**Independent Test**: Configure `single_endpoint: app1` with exactly one endpoint named `app1`;
confirm `GET /` and `GET /orders/123` both reach `app1`'s backend with the full original path
unchanged (`/orders/123`, not stripped to anything). Confirm `GET /_` still returns the
discovery listing. Confirm the gateway fails to start if `single_endpoint` doesn't match the
sole endpoint's name, or if it's set alongside more than one configured endpoint.

- [X] T038 In `internal/config/config.go`, add `SingleEndpoint string
  \`yaml:"single_endpoint"\`` to `yamlConfig`, and change `loadConfig`'s signature from
  `([]RouteConfig, bool, error)` to `([]RouteConfig, yamlConfig, error)` (returning the whole
  decoded `yamlConfig` instead of just `Verbose`, so `Parse` can read both `Verbose` and the
  new `SingleEndpoint` without the return tuple growing again next time); add
  `GatewayConfig.SingleEndpoint string` (empty means disabled) populated from it in `Parse`.
  After building `routes`, if `raw.SingleEndpoint != ""`: fail with a descriptive error if
  `len(routes) != 1`, and fail with a descriptive error if `raw.SingleEndpoint != routes[0].Name`
  (data-model.md `GatewayConfig`)
- [X] T039 [P] In `internal/router/router.go`, add a functional-option constructor: `type
  Option func(*Router)` and `func WithRootRoute(name string) Option` that, if `name` matches a
  configured route's name, sets a new `root *Route` field on `Router` to that route; change
  `New(routes []Route) *Router` to `New(routes []Route, opts ...Option) *Router`, applying
  `opts` after building `byName`. In `ServeHTTP`, after the existing `discoveryPath` check and
  before first-segment matching, add: if `rt.root != nil`, delegate directly to
  `rt.root.Handler.ServeHTTP(w, r)` with `r` unmodified (no path stripped) and return — this
  bypasses first-segment matching entirely, so a root-mounted route's own `name` is never
  looked for in the path (research.md "Routing" in data-model.md)
- [X] T040 In `cmd/gateway/main.go`, when `cfg.SingleEndpoint != ""`, pass
  `router.WithRootRoute(cfg.SingleEndpoint)` to `router.New(routes, ...)` when building
  `topRouter` (depends on T038, T039)
- [X] T041 [P] New unit tests in `tests/unit/config_test.go`: `TestConfig_Parse_SingleEndpoint`
  asserting `cfg.SingleEndpoint` is set correctly when it matches the sole configured
  endpoint's name; `TestConfig_Parse_SingleEndpointMismatch` asserting an error when
  `single_endpoint` doesn't match the sole endpoint's name; and
  `TestConfig_Parse_SingleEndpointWithMultipleEndpoints` asserting an error when
  `single_endpoint` is set alongside two or more `endpoints` entries (depends on T038)
- [X] T042 [P] New unit test `TestRouter_WithRootRoute_ServesAnyPathUnprefixed` in
  `tests/unit/router_test.go`: build a `Router` via `router.New(routes,
  router.WithRootRoute("app1"))` with one route named `app1`; confirm both `GET /` and `GET
  /orders/123` reach that route's handler with `r.URL.Path` unchanged; confirm `GET /_` still
  returns the discovery listing rather than being swallowed by the root route (depends on T039)
- [X] T043 [P] In `tests/testutil/gateway.go`, add `WithRootRoute(name string) Option`
  mirroring `cmd/gateway/main.go`'s wiring from T040 — passes `router.WithRootRoute(name)`
  through to `router.New` inside `NewTestGateway` (parallel to Phase 9's `WithVerbose`,
  depends on T039)
- [X] T044 [P] New integration test `tests/integration/single_endpoint_test.go`: build a real
  test gateway with one route via `testutil.NewTestGateway(t, []testutil.TestRoute{{Name:
  "app1", Backend: backend}}, testutil.WithRootRoute("app1"))`; confirm requests to `/` and to
  `/orders/123` both reach the backend with the same unmodified path, and that `/_` still
  returns the discovery listing (depends on T040, T043)
- [X] T045 [P] Update `README.md` and `specs/001-handler-injection/quickstart.md` with a
  `single_endpoint` example and description (optional, valid only with exactly one configured
  endpoint, routes every path — including `/` — to that endpoint's `dest` unchanged); update
  `specs/001-handler-injection/data-model.md`'s `GatewayConfig` table with a `single_endpoint`
  field row and its validation rule (depends on T038)
- [X] T046 Run `make lint` and `make test` to confirm the new field, router behavior, and every
  existing and new test all pass (depends on T038-T045)

**Checkpoint**: `single_endpoint: <name>` mounts that one configured endpoint at the gateway's
root with paths passed through unchanged; discovery (`/_`) still works; startup fails clearly
if `single_endpoint` doesn't match the sole endpoint's name or if more than one endpoint is
configured alongside it

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
- **Config schema (Phase 8)**: Depends only on Foundational (it reworks `internal/config`,
  which Phase 2 already established) — independent of Phases 3–7, so it can run in parallel
  with any of them
- **Verbose request logging (Phase 9)**: Depends on Phase 8 (needs `GatewayConfig.Verbose` to
  exist) and Foundational (wraps `topRouter` from `cmd/gateway/main.go`); independent of
  Phases 3–7
- **Single-endpoint root mounting (Phase 10)**: Depends on Phase 8 (T038 reuses/extends
  `loadConfig`'s `yamlConfig` return value introduced there) and Foundational (`internal/router`
  and `cmd/gateway/main.go`); independent of Phases 3–7 and Phase 9 (a different config key,
  no shared code path with verbose logging)

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
- Likewise, Phase 8 (the `endpoints`/`verbose` config schema change) and Phase 9 (wiring
  `verbose` up to actual request logging) both went straight to `/speckit-tasks` without a
  `/speckit-plan` amendment first, since neither introduces an architectural decision beyond
  what's already in plan.md — Phase 8 reshapes `internal/config`'s YAML unmarshal target, and
  Phase 9 adds one small, self-contained logging middleware in `internal/observability`.
  Phase 9 supersedes Phase 8's note that `verbose` was "deliberately unused beyond being parsed
  and stored" — it's now wired to something.
- Phase 10 (`single_endpoint`) also skipped a `/speckit-plan` amendment for the same reason.
  Its scope is deliberately narrow: root-mounting is valid *only* when exactly one endpoint is
  configured, matching what was asked for. Supporting root-mounting for one endpoint chosen
  out of *several* configured routes (e.g. `single_endpoint: app1` alongside `app2`, `app3`)
  is a different feature with real ambiguity to resolve (does `app2` still need a prefix? does
  it become unreachable? does root-mounting take priority over prefix matching for every
  route?) and isn't implemented — if you want that, it needs its own follow-up.
- Every existing US1/US2/US3 integration test needs a route-name prefix added to its request
  URLs — there is no more "no route configured" mode; every request now needs a route to match.
- [P] tasks touch different files with no unmet dependency — safe to parallelize
- Verify each REQUIRED test fails (or doesn't compile) before the rework, and passes after
- Commit after each phase or logical group
