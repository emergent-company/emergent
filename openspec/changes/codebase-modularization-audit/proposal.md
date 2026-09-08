## Why

The Go server (49 domains, 400 files, ~160k LOC) carries cross-cutting technical debt that has **grown** since this audit was first scoped: 312 inline auth guards (was 289), 1168 `apperror` Style-A chains (was 664), and 21 cross-domain `SetXxx` setters (was 9). Half the original remediation has shipped (httputil, graph/journal decoupling, FeatureSet), but the helpers are defined yet **never applied** — `RequireProject()` wires 0 routes, and the `.golangci.yml` blanket-disables three linters, so nothing stops the debt from growing. Addressing this now unlocks a cleaner codebase and a lightweight deployment target.

## What Changes

- Introduce `pkg/httputil` package consolidating `APIResponse[T]`, `PaginatedResponse[T]`, and `SuccessResponse[T]` — **done**; finish residue (4 domain type aliases, `superadmin.SuccessResponse`, `pkg/sdk` copies)
- Add `auth.GetProjectUUID(c)` helper to `pkg/auth` and remove 3 copy-pasted local `getProjectID()` functions — **helper done**, 3 local copies remain
- Introduce `RequireProject()` middleware variant in `pkg/auth` to replace 312 inline user-nil-check blocks — **helper done, 0 routes migrated**
- Standardize on `apperror` Style B (`apperror.NewBadRequest/NewInternal/NewNotFound`) — migrate 1168 Style A usages
- Extract `RegisterWorkerLifecycle[W Worker](lc, w)` helper in `apps/server/domain/extraction` to collapse 3 remaining `fx.Lifecycle` hook blocks (was 6)
- Replace 21 setter-injection (`SetXxx()`) wiring points with explicit named interfaces defined in the receiving package
- Decouple `graph.Service` from direct `*journal.Service` embed via a `GraphEventSink` interface — **done** (`EventSink` + `GraphEventSinkAdapter`)
- Add `FeatureSet` config struct with env-var flags controlling conditional `fx.Options` in `main.go` — **done**
- Gate optional domains behind config-driven module inclusion to stop compiling debug code into production binaries — **done**
- **NEW** Add a prevention layer (L3): un-mask the linter and add CI gates that fail on re-introduction of these four patterns

> Removed from scope: deleting `domain/chat` / `pkg/llm/vertex` — both confirmed **live** (chat wired in `main.go`, `pkg/sdk/chat` client, `cmd/swiftbridge`; extraction depends on `vertex`). Keep feature-flagged.

## Capabilities

### New Capabilities

- `shared-http-types`: Consolidated `pkg/httputil` package with generic response types and constructors shared across all domains
- `auth-middleware-consolidation`: `RequireProject()` middleware + `MustGetUser(c)` / `GetProjectUUID(c)` helpers eliminating handler-level auth boilerplate
- `explicit-domain-interfaces`: Named interfaces replacing setter-injection anti-patterns across `mcp`, `mcpregistry`, `orgs`, and `mcprelay` domains
- `graph-journal-decoupling`: `GraphEventSink` interface decoupling `graph.Service` from direct `*journal.Service` dependency
- `worker-lifecycle-helper`: Generic `RegisterWorkerLifecycle` helper reducing extraction worker module boilerplate
- `feature-flag-infrastructure`: `FeatureSet` config struct + conditional `fx.Options` pattern in `main.go` enabling runtime feature toggling per deployment
- `lint-prevention-gates`: CI rules failing on re-introduction of the four duplication patterns (auth guards, setters, duplicate response types, Style-A apperror)

### Modified Capabilities

None — these are internal refactors. No public API or CLI behavior changes.

## Impact

**Packages created:**
- `apps/server/pkg/httputil/` — new shared response types

**Packages modified:**
- `apps/server/pkg/auth/` — new helpers + middleware variant
- `apps/server/pkg/apperror/` — Style A usages migrated to Style B
- `apps/server/internal/config/` — `FeatureSet` added
- `apps/server/cmd/server/main.go` — conditional fx module loading

**Domains modified (handler-level auth cleanup):**
All ~30 domains that currently repeat the 3-line auth guard block

**Domains with setter injection replaced:**
`domain/mcp`, `domain/mcpregistry`, `domain/orgs`, `domain/mcprelay`

**Domains decoupled:**
`domain/graph` (from `domain/journal`)

**Domains kept feature-flagged (not removed):**
`domain/chat`, `pkg/llm/vertex` (both confirmed live)

**New enforcement:**
`apps/server/.golangci.yml` — un-mask linters + custom duplication gates

**No breaking changes to external APIs, CLI, or database schema.**
