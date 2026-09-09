## Why

MCP tool results returned by the server are machine-unclassifiable: each tool uses its own ad-hoc envelope (or none), `success` is sometimes an int count and sometimes a bool, `message` is set on both success and failure paths, and `remember`/`forget` return run results only as human prose. Consumers (Alfred chat UI chips, agent executors, external MCP clients) cannot determine success/error or extract `run_id` generically without per-tool regexes and heuristics (GitHub issue #319). Some partial work has landed (batch tools carry top-level `ok` + `created`; remember/forget sync prose includes `run_id`; loopback HTTP errors now surface missing scopes via `mcpHTTPError`) but no uniform contract exists and the remaining inconsistencies keep the classification problem open.

## What Changes

- **Introduce a uniform result envelope** `{ ok: bool, error?: string, data: {…tool-specific…}, meta?: {…} }` via one shared helper in `apps/server/domain/mcp/` for the tools named in #319 and their envelope siblings: `entity-create`, `entity-update`, `entity-delete`, `relationship-create`, `relationship-delete`, `search-hybrid`, `search-semantic`, `entity-query`, `remember`, `forget`. New tools MUST use the helper (enforced by a registry test). Remaining structured-result tools migrate in a Phase 2 follow-up.
- **Replace the ambiguous `success` fields** with bool `ok` everywhere: drop the top-level int `success` count from `entity-create` / `relationship-create` and rename per-item `results[].success` → `results[].ok` (**BREAKING** for clients reading either). Numeric counts stay as `created` / `failed` / `total`.
- **Reserve `message` as human summary only**: never a status signal; status lives in `ok` / `error`.
- **Return structured `run_id` (and document/status data) from `remember` / `forget`** so agents and UIs can call `remember-status(run_id)` without parsing prose (**BREAKING**: tool result becomes a JSON object instead of bare text).
- **Fix `forget` async copy** that tells callers to see "what was created" (should be removed) while touching these paths.
- **Keep slim read responses** (already default for `search-hybrid` via `slimEntity`; confirm `entity-query` parity) — no re-verbosification.
- **Update internal consumers in lockstep** so counts do not silently zero: `agents/remember_status.go` aggregation (`parseEntityCreate`/`parseEntityUpdate`/`parseRelationshipCreate`/`parseEntityDelete`/`parseRelationshipDelete`) parses the current shapes and must read the new envelope in the same change as the producer.
- **Delete dead legacy result structs** (`CreateEntityResult`/`CreatedEntity`/`CreateRelationshipResult`/`CreatedRelationship` in `mcp/entity.go`) and the deprecated int `success` — one coordinated breaking release, no grace period (envelope restructure is breaking for external consumers regardless).
- **Preserve existing tool-specific payload field names inside `data`**: search keeps `data`/`total`/`has_more`, entity-query keeps `entities`/`pagination`, batch results keep `results` — only the wrapping layer changes.

## Capabilities

### New Capabilities

- `mcp-tool-results`: Uniform `{ok, error, data, meta?}` result envelope contract for MCP tools — status semantics (bool `ok`, `error` string, numeric counts, `message` as human-only) and slim-by-default entity responses.

### Modified Capabilities

(none — no existing main spec governs MCP result envelopes; `mcp-tool-naming-convention` covers names only, unchanged here)

## Impact

- **`apps/server/domain/mcp/`**: `service.go` (batch create envelopes ~3875, remember/forget executors ~5250-5400), `graph_tools.go`, `entity.go` (dead structs ~588), `response_contract.go`; new shared envelope helper + registry test.
- **`apps/server/domain/agents/`**: `remember_status.go` aggregation parsers (~315-425) must read the new envelope; `toolpool.go` `convertToolResult` normalization (~743-793) needs a passthrough test for the nested envelope (no code change expected — nested JSON flows through, `ok` already present); prompt text in `agents/repository.go` (~576) referencing `pagination.total`/`has_more`/`version` and `personal_kb_agent.go` updated to reflect nesting under `data`/`meta`.
- **Tests**: fixtures encoding current shapes (`remember_status_test.go`, `mcp/entity_create_dedup_test.go`, `toolpool_test.go`) updated in the same change — stale fixtures would silently pass while production counts zero out. Add negative fixture asserting old top-level shape yields zero counts.
- **External consumers (not in this repo)**: Alfred chat UI tool-chip classifier and any MCP client reading top-level tool fields — coordinated breaking release + release note; separate-repo follow-up, not a blocker.
- **Docs**: swagger annotations on `agents/handler.go`/chat handlers if they document affected response shapes, `apps/server/domain/mcp/README.md`, `docs/site/` MCP pages.
- **REST parity check**: chat REST remember/forget already returns structured `{run_id, status, summary, document_id}` — MCP path consumes it; no REST change expected.
