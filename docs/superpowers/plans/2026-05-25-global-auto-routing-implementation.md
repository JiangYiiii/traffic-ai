# Global AUTO Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement administrator-managed global AUTO routing from `docs/superpowers/specs/2026-05-25-global-auto-routing-design.md`.

**Architecture:** Add virtual-model metadata to `models`, add AUTO policy/candidate repositories, extend the routing service with `SelectRoute`, and make gateway forwarding rewrite virtual models to resolved real models. Billing and usage logging use the resolved model while preserving requested/resolved route explanation fields.

**Tech Stack:** Go, Gin, MySQL migrations, Redis-backed existing limiter/breaker, vanilla JS admin/user console.

---

### Task 1: Schema And Domain Types

**Files:**
- Create: `migrations/000015_auto_routing.up.sql`
- Create: `migrations/000015_auto_routing.down.sql`
- Modify: `internal/domain/model/entity.go`
- Modify: `internal/domain/model/repository.go`
- Modify: `internal/domain/routing/entity.go`
- Modify: `internal/domain/routing/repository.go`
- Modify: `internal/domain/routing/service.go`
- Modify: `internal/infrastructure/persistence/mysql/model_repo.go`
- Modify: `internal/infrastructure/persistence/mysql/usage_log_repo.go`
- Modify: `internal/interfaces/api/dto/usage_log_dto.go`

- [ ] Add migrations for virtual model columns, AUTO policy/candidate tables, and usage log route fields.
- [ ] Extend model and usage log entities with fields from the design.
- [ ] Add AUTO policy/candidate domain structs and repository interface.
- [ ] Update MySQL model and usage log scans/inserts.
- [ ] Run `go test ./internal/domain/... ./internal/infrastructure/persistence/mysql/...`.

### Task 2: AUTO Repository

**Files:**
- Create: `internal/infrastructure/persistence/mysql/auto_route_repo.go`
- Create: `internal/infrastructure/persistence/mysql/auto_route_repo_test.go`

- [ ] Write failing repository tests for create/list/update/delete policy, candidate CRUD, and validation-neutral persistence.
- [ ] Implement MySQL repository methods.
- [ ] Run `go test ./internal/infrastructure/persistence/mysql/...`.

### Task 3: Routing Selection

**Files:**
- Modify: `internal/application/routing/usecase.go`
- Create: `internal/application/routing/auto_route_test.go`

- [ ] Write failing tests for non-virtual delegation, AUTO candidate resolution, token group/protocol filtering, circuit filtering, capability validation, cheap strategy cost behavior, and no-route cases.
- [ ] Implement `SelectRoute(ctx, RouteRequest)` and keep existing methods delegating safely.
- [ ] Implement deterministic request-feature and scoring helpers needed by routing tests.
- [ ] Run `go test ./internal/application/routing/...`.

### Task 4: Gateway Chat AUTO Flow

**Files:**
- Modify: `internal/application/gateway/usecase.go`
- Create or extend: `internal/application/gateway/*_test.go`

- [ ] Write failing tests that `model=auto` rewrites upstream body to the resolved model.
- [ ] Write failing tests that pre-deduction and settlement use the resolved model.
- [ ] Write failing tests for cross-model fallback refund/re-deduct behavior before first byte.
- [ ] Implement route-decision handling in chat completions without changing unrelated protocol paths.
- [ ] Run `go test ./internal/application/gateway/...`.

### Task 5: List Models And Usage Logs

**Files:**
- Modify: `internal/application/routing/usecase.go`
- Modify: `internal/interfaces/gateway/handler.go`
- Modify: `internal/interfaces/api/dto/usage_log_dto.go`

- [ ] Write failing tests or focused assertions for loose `/v1/models` AUTO visibility.
- [ ] Ensure user/admin usage logs expose requested/resolved model and route fields.
- [ ] Run relevant Go tests.

### Task 6: Admin AUTO APIs

**Files:**
- Create: `internal/application/routing/auto_policy.go`
- Create: `internal/interfaces/api/dto/auto_route_dto.go`
- Create: `internal/interfaces/api/handler/auto_route_handler.go`
- Modify: `internal/interfaces/api/deps.go`
- Modify: `internal/interfaces/api/handler/model_handler.go` or register a separate handler from `deps.go`

- [ ] Write handler/usecase tests for create/list/update/delete policies and candidates.
- [ ] Enforce validation: strategy enum, capability tags, bounded scores/weights, optimistic version.
- [ ] Implement dry-run endpoint without forwarding upstream, balance deduction, or rate-limit consumption.
- [ ] Register routes under `/admin/auto-routes`.
- [ ] Run `go test ./internal/application/routing/... ./internal/interfaces/api/...`.

### Task 7: Admin And User Console UI

**Files:**
- Modify: `internal/interfaces/api/static/admin.html`
- Modify: `internal/interfaces/api/static/js/admin.js`
- Modify: `internal/interfaces/api/static/js/i18n.js`
- Modify: `internal/interfaces/api/static/app.html`
- Modify: `internal/interfaces/api/static/js/app.js`
- Modify mirrored `web/console/*` files only if this repo keeps generated/static copies in sync.

- [ ] Add AUTO routes admin section with list, create/edit, candidate management, publish toggle, and dry-run tester.
- [ ] Show `auto -> resolved` in user/admin usage logs.
- [ ] Add copy explaining AUTO is charged by final routed model.
- [ ] Run existing UI smoke/build command if available.

### Task 8: Full Verification

**Files:**
- All touched files.

- [ ] Run `go test ./...`.
- [ ] Run `go test -race` on routing/gateway packages if feasible.
- [ ] Run any existing smoke scripts that do not require unavailable services.
- [ ] Review `git diff` against the spec and remove anything outside scope.
