<!--
Sync Impact Report
- Version change: (none) → 1.0.0
- Modified principles: n/a (initial ratification)
- Added sections:
  - Core Principles: I. Code Quality, II. Testing Standards, III. User Experience Consistency, IV. Performance Requirements
  - Governance
- Removed sections: [SECTION_2_NAME]/[SECTION_2_CONTENT] and [SECTION_3_NAME]/[SECTION_3_CONTENT] template
  slots (not populated — this project's principles are fully covered by the four Core Principles below;
  no additional constraints or workflow section was needed at ratification time)
- Templates requiring updates:
  - ✅ .specify/templates/plan-template.md (Constitution Check gate already generic; no changes required)
  - ✅ .specify/templates/spec-template.md (no constitution-specific references; no changes required)
  - ✅ .specify/templates/tasks-template.md (updated: testing task guidance now reflects
    NON-NEGOTIABLE testing discipline from Principle II instead of "tests are optional")
  - N/A .specify/templates/commands/*.md (directory does not exist in this project)
- Follow-up TODOs: none
-->

# API Gateway Constitution

## Core Principles

### I. Code Quality

Code MUST be reviewed and approved by at least one other engineer before merging to the
default branch. Static analysis / linting MUST pass in CI before a change can merge, with
no disabled or skipped checks unless explicitly justified in the PR description.

Every function, module, or service MUST have a single, clear responsibility. Do not add
abstractions, configuration options, or extension points for hypothetical future needs
(YAGNI); prefer duplication over a premature shared abstraction. Comments MUST explain
*why* code exists (non-obvious constraints, workarounds, invariants), never *what* it does.

**Rationale**: Gateway code sits on every request path for every downstream service; low-quality
or overly clever code here has outsized blast radius, so review and simplicity are non-negotiable.

### II. Testing Standards (NON-NEGOTIABLE)

All new functionality MUST ship with automated tests covering its primary behavior and its
key edge cases. Every bug fix MUST include a regression test that fails before the fix and
passes after it. Every externally-facing route, contract, or shared schema change MUST have
a corresponding contract/integration test.

CI MUST run the full automated test suite on every pull request, and a failing suite MUST
block merge — there is no override. Manual "it worked on my machine" verification does not
substitute for an automated test.

**Rationale**: The gateway is shared infrastructure — a regression here breaks every service
behind it simultaneously, so automated, enforced testing is the primary safety net.

### III. User Experience Consistency

For this project, "user" means any consumer of the gateway's API (service clients, internal
teams, external partners). All routes MUST follow one documented convention for request/response
shape, status codes, and error format — no endpoint may invent its own error envelope or
success shape. Error responses MUST carry a stable, documented error code plus a human-readable
message that tells the caller what to do next.

Any breaking change to a public contract MUST be versioned and communicated before it ships;
existing consumers' behavior MUST NOT change silently. Consumer-facing documentation MUST be
updated in the same change that introduces or modifies the behavior it describes.

**Rationale**: Consumers integrate against the gateway's contracts, not its implementation;
inconsistent shapes or silent breaking changes push integration cost onto every team downstream.

### IV. Performance Requirements

Every feature that adds or modifies a request-path operation MUST state its latency and
throughput budget (e.g., target p95/p99 latency, expected req/s) during planning, before
implementation begins. A change MUST NOT regress p95/p99 latency or throughput of an existing
route by more than 10% without explicit justification and sign-off recorded in the plan's
Complexity Tracking section.

Performance-sensitive code paths MUST be load- or benchmark-tested before release. Resource
usage (memory, connections, goroutines/threads, cache size, queue depth) MUST be bounded and
monitored; unbounded queues, caches, or connection pools are prohibited.

**Rationale**: As a gateway, this service's latency and failure modes are additive to every
request it proxies — an unbudgeted regression here degrades every client simultaneously.

## Governance

This constitution supersedes any conflicting engineering practice, template default, or prior
convention. Amendments require: a written proposal stating the change and rationale, a version
bump per the policy below, and sign-off from the project's designated maintainer(s) before merge.

Versioning policy (semantic versioning for this document):
- **MAJOR**: Backward-incompatible governance changes, or removal/redefinition of a principle.
- **MINOR**: A new principle or section added, or materially expanded guidance on an existing one.
- **PATCH**: Wording, typo, or clarification fixes with no semantic change.

Every plan MUST pass the Constitution Check gate before Phase 0 research and again after Phase 1
design; any unresolved violation MUST be justified in that plan's Complexity Tracking table or
the plan MUST be revised until it complies. CLAUDE.md and any other agent-specific guidance files
MUST be kept consistent with this constitution for day-to-day development guidance.

**Version**: 1.0.0 | **Ratified**: 2026-09-01 | **Last Amended**: 2026-09-01
