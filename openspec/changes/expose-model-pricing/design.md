## Context

See proposal.md - Why. Retail pricing is stored in `kb.provider_pricing` (entity `ProviderPricing`, table `provider_pricing`, `UNIQUE(provider, model)`), synced by `PricingSyncService`. The repository already has `GetPricing` / `GetPricingByModel` / `UpsertPricing` but no "list all pricing" method or route. Provider routes register under `/api/v1` (`routes.go`).

## Goals / Non-Goals

**Goals:** expose retail pricing read-only via API.

**Non-Goals:** no write path, no combined effective-rate endpoint.

## Decisions

1. **Dedicated `GET /api/v1/pricing` endpoint** (not embedding pricing into the model catalog). Rationale: catalog (`provider_supported_models`, from models.dev) and pricing (`provider_pricing`, synced separately) can diverge; a dedicated endpoint keeps them decoupled and returns exactly the pricing rows. Alternative: embed pricing in `ListModels`/`ListAllModels` — rejected (would couple two separately-synced tables and change existing response shapes).

2. **Repository `ListPricing(ctx) ([]ProviderPricing, error)`** — `SELECT ... FROM provider_pricing ORDER BY provider, model`, mirroring `UpsertPricing`/`GetPricing` bun style. Return empty slice (not nil) on no rows.

3. **Handler + route.** `ListPricing` handler returns `c.JSON(200, rows)` with no org/project scoping (pricing is global retail data, gated by `RequireAuth`). Register `api.GET("/pricing", h.ListPricing)` in `routes.go`.

4. **SDK client.** Add `ListPricing(ctx) ([]ProviderPricing, error)` to `pkg/sdk/provider/client.go` using `doJSON`; add a `ProviderPricing` type with `json` tags matching the server (server structs have no `json` tags → keys are Go field names, e.g. `Provider`, `Model`, `TextInputPrice`, `OutputPrice`).

5. **CLI (optional).** Add `memory provider pricing list` under the existing `pricing` subcommand group.

## Risks / Trade-offs

- **Pricing rows without a matching catalog model** (and vice versa) — acceptable; consumers merge defensively.
- **`ProviderPricing` JSON key casing** — server emits PascalCase (no tags); SDK must match exactly (see decision 4).

## Migration Plan

No migration (read-only). Deploy alongside gateway change.

## Open Questions

- Whether to add a project-scoped "effective pricing" endpoint later — deferred.
