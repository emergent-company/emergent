## 1. Shared HTTP Types (pkg/httputil)

- [x] 1.1 Create `apps/server/pkg/httputil/response.go` with `APIResponse[T any]`, `PaginatedResponse[T any]`, `SuccessResponse[T any]` types and `NewSuccessResponse[T]` constructor
- [x] 1.2 Migrate `domain/agents/dto.go` — replace local `APIResponse[T]` and `SuccessResponse` with imports from `pkg/httputil`
- [x] 1.3 Migrate `domain/extraction/dto.go` — replace local `APIResponse[T]` with `pkg/httputil` import
- [x] 1.4 Migrate `domain/mcpregistry/entity.go` — replace local `APIResponse[T]` with `pkg/httputil` import
- [x] 1.5 Migrate `domain/sandboximages/entity.go` — replace local `APIResponse[T]` with `pkg/httputil` import
- [x] 1.6 Migrate `pkg/sdk/` client files — replace any duplicate response type definitions with `pkg/httputil` imports
- [x] 1.7 Verify build passes: `task build`

## 2. Auth Middleware Consolidation (pkg/auth)

> Re-sync (Sep 2026): helpers **exist** but are **never applied** — `RequireProject()` has 0 route usages, and 312 `user == nil` guards remain in 46 files. **Correction:** routes are *already* behind `auth.Middleware.RequireAuth()` + `RequireProjectID()`. The 312 guards are redundant defense-in-depth — this track is now **deletion**, not middleware rollout.

- [x] 2.1 Add `GetProjectUUID(c echo.Context) (uuid.UUID, error)` — now in `pkg/auth/middleware.go:969`
- [x] 2.2 Add `RequireProject() echo.MiddlewareFunc` — now in `pkg/auth/middleware.go:983` (but see 2.7: redundant, retire it)
- [x] 2.3 Add `MustGetUser(c echo.Context) *AuthUser` — now in `pkg/auth/middleware.go:959`
- [ ] 2.4 Remove local `getProjectID()` helper from `domain/chunks/handler.go:25` — replace with `auth.GetProjectUUID(c)` (already a thin wrapper)
- [ ] 2.5 Remove local `getProjectID()` helper from `domain/journal/handler.go:169` — replace with `auth.GetProjectUUID(c)`
- [ ] 2.6 Remove local `getProjectID()` helper from `domain/graph/handler.go:83` — replace with `auth.GetProjectUUID(c)`
- [ ] 2.7 Replace 312 inline `user := auth.GetUser(c); if user == nil { return apperror.ErrUnauthorized }` guards with `user := auth.MustGetUser(c)` — batch by domain, only for handlers behind `RequireAuth()`/`RequireProjectID()`. Handle the 4 no-`routes.go` domains (`extraction`, `useraccess`, `invites`, `events`) individually.
- [ ] 2.8 Retire unused package-level `RequireProject()` (0 usages) — delete or document as deprecated in favor of `auth.Middleware.RequireAuth()`
- [ ] 2.9 Verify build passes and all auth-protected routes return 401 for unauthenticated requests: `task build && task test`

> **DONE (Sep 2026):** 304 standard guards replaced (164 → `MustGetUser`, 140 removed as dead where `user` was unused). 10 remaining are correct edge cases (tuple-return helpers, DB-lookup nil checks, `agentcompat` custom errors, `invites` login redirect). 3 `getProjectID()` wrappers deleted (49 call sites → `auth.GetProjectUUID`). `RequireProject()` deleted. Build + vet + unit tests pass.

## 3. apperror Style Standardization

- [ ] 3.1 Audit `pkg/apperror/` to confirm Style B constructors (`NewBadRequest`, `NewInternal`, `NewNotFound`, etc.) exist and are equivalent to Style A chaining
- [ ] 3.2 Write a migration script (or `sed` one-liner set) to convert Style A patterns (`apperror.ErrBadRequest.WithMessage(...)`, `apperror.ErrInternal.WithInternal(...)`) to Style B equivalents across the codebase
- [ ] 3.3 Run migration script and verify all 664 Style A usages are converted
- [ ] 3.4 Verify build passes: `task build`
- [ ] 3.5 Run test suite to confirm no behavior changes: `task test`

## 4. Worker Lifecycle Helper (domain/extraction)

> Re-sync (Sep 2026): `Worker` interface + `RegisterWorkerLifecycle` **exist** (`domain/extraction/worker.go:15,23`), but 3 `lc.Append(fx.Hook{...})` blocks still remain in `module.go` (lines 129, 377, 423).

- [x] 4.1 Create `apps/server/domain/extraction/worker.go` — define `Worker` interface with `Start(context.Context) error` and `Stop(context.Context) error`
- [x] 4.2 Add `RegisterWorkerLifecycle(lc fx.Lifecycle, w Worker)` function to `worker.go`
- [x] 4.3 Verify all extraction workers satisfy the `Worker` interface
- [ ] 4.4 Replace remaining 3 `lc.Append(fx.Hook{...})` blocks in `extraction/module.go` (lines 129, 377, 423) with `RegisterWorkerLifecycle(lc, worker)` calls
- [ ] 4.5 Verify build passes: `task build`

## 5. Explicit Domain Interfaces (setter injection removal)

> **DONE (Sep 2026).** All 21 cross-domain `SetXxx` wiring setters removed and replaced with interfaces injected via `fx` constructor params (`fx.In` optional fields + `fx.Provide` adapters in `module.go`/`main.go`). `mcp.Service` (13), `projects` (2), `mcpregistry` (1), `orgs` (1), `blueprints` (1), `sandbox` (1), `agents.MCPToolHandler` (2). `mcp.GraphObjectPatcher` is a named func type. Build + vet + unit tests pass.
>
> **Left as-is (out of scope):** `mcprelay.Service.OnChange(...)` (§5.7/§5.11) — it's an event-subscription callback, not a dependency-injection setter; no interface extracted.

- [x] 5.1–5.13 Setter injection removed (21 setters → constructor injection)

## 6. Graph/Journal Decoupling (GraphEventSink)

> Re-sync (Sep 2026): **DONE** — shipped outside this change. `domain/graph/events.go` defines `EventSink` + `NoopEventSink`; `domain/journal/graph_sink.go` provides `GraphEventSinkAdapter`.

- [x] 6.1 Create `apps/server/domain/graph/events.go` — define `EventSink` interface
- [x] 6.2 Add `NoopEventSink` struct
- [x] 6.3 Replace `*journal.Service` field with `EventSink`; default `NoopEventSink{}`
- [x] 6.4 Replace `s.journal.Log*(...)` calls with `s.eventSink.Log*(...)`
- [x] 6.5 Verify `domain/graph` no longer imports `domain/journal`
- [x] 6.6 Add `GraphEventSinkAdapter` in `domain/journal/graph_sink.go`
- [x] 6.7 Wire journal → graph event sink via fx
- [x] 6.8 Update `cmd/server/main.go` wiring
- [x] 6.9 Verify graph mutations still logged

## 7. Feature Flag Infrastructure (FeatureSet + conditional fx.Options)

> Re-sync (Sep 2026): **DONE** — `internal/config/features.go` defines `FeatureSet`; `cmd/server/main.go` uses `coreFxOptions()` + `featureFxOptions(f)`. Note: `FEATURE_CHAT` defaults to `true` (not `false` as originally designed) — chat is live.

- [x] 7.1 Audit `/api/chat` route usage — chat is LIVE: wired in `main.go`, has `pkg/sdk/chat` client, used by `cmd/swiftbridge`
- [x] 7.2 Create `apps/server/internal/config/features.go` — `FeatureSet` struct
- [x] 7.3 Add `Features FeatureSet` field to `Config`
- [x] 7.4 Refactor `cmd/server/main.go` — `coreFxOptions()` + `featureFxOptions(f)`
- [x] 7.5 Add conditional `fx.Options` blocks for feature-flagged domains
- [x] 7.6 Verify default behavior unchanged
- [x] 7.7 Verify feature toggle works
- [x] 7.8 Document `FEATURE_*` env vars

## 8. Verification & Cleanup

- [ ] 8.1 Run full test suite: `task test`
- [ ] 8.2 Run e2e tests: `task test:e2e`
- [ ] 8.3 Run linter: `task lint`
- [ ] 8.4 Confirm no cross-domain `SetXxx` setter methods remain: `grep -r "func.*Set[A-Z]" apps/server/domain/` (21 remain, see §5)
- [ ] 8.5 Confirm no inline `user == nil` auth guards remain: `grep -rn "user == nil" apps/server/domain/` (312 remain)
- [ ] 8.6 Confirm `APIResponse`, `PaginatedResponse`, `SuccessResponse` defined only in `pkg/httputil` (residue: 4 domain aliases + `superadmin` + `pkg/sdk`)
- [ ] 8.7 Confirm no Style A `apperror` usage remains: `grep -rn "\.WithMessage\|\.WithInternal" apps/server/domain/` (1168 remain)

## 9. Prevention Layer (L3) — stop re-introduction

> Re-sync (Sep 2026): **NEW.** The existing `.golangci.yml` disables `errcheck`, `staticcheck`, and `unused` via blanket `text: "."` exclusions — the linter is a no-op. Debt grew 3 months straight with zero gates. This track makes the cleanup stick.

- [x] 9.1 Remove the three blanket `text: "."` exclusions for `errcheck`, `staticcheck`, `unused`; replace with targeted `exclude-rules` only for confirmed-legacy violations (or a `//nolint` per-line baseline)
- [x] 9.2 Add a custom `golangci-lint` check (or `forbidigo`/`revive` rule) that fails CI on inline `user == nil` auth guards in `domain/*/handler.go` — force `RequireProject()` + `MustGetUser()`
- [x] 9.3 Add a rule failing on new `func (s *Service) Set[A-Z]` cross-domain setters — force constructor/`fx.Provide` injection
- [x] 9.4 Add a rule failing on new `type APIResponse|PaginatedResponse|SuccessResponse` definitions outside `pkg/httputil`
- [x] 9.5 Add a rule failing on new `apperror.Err*.With*` Style A chaining — force Style B `apperror.New*`
- [x] 9.6 Wire the rules into CI (lefthook pre-commit + a CI job) so violations block merge, not just flag
- [x] 9.7 Re-run `task lint` and get a clean baseline; record the current violation counts as the ratchet floor

> **DONE (Sep 2026):** §9 implemented as a grep-based ratchet rather than custom golangci plugins. `.golangci.yml` migrated to v2 + un-masked (148 staticcheck / 48 unused / 2 gofmt now visible). CI lint job uses `only-new-issues: true` + `fetch-depth: 0`. `apps/server/scripts/lint-ratchet.sh` gates the 4 structural patterns against baselines (auth guards 10, setters 21, Style-A 1167, response-type dupes 6); wired into lefthook pre-commit + the CI lint job.
