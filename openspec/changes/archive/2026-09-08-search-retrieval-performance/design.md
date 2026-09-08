## Context

See `proposal.md` for motivation. Evidence-backed current state (from codebase recon):

- **N+1**: unified search expands relationships per result object at `domain/search/service.go:535-540`, calling `Repository.GetEdges` (`domain/graph/repository.go:726-774`), which has **no LIMIT or pagination** — returns all incoming + outgoing edges for an object.
- **No cutoff**: no similarity threshold exists in any search path; the repository clamps force a floor of 20 (text) / 50 (relationship) candidates (`domain/search/repository.go:87,144,217,429`).
- **Hard-coded knobs**: `ivfflat.probes = 10` per-query tx (`repository.go:37-47,148,270,433`); RRF `k=60` (`service.go:692,1010-1013`); weighted fusion `0.25/0.75/0` (`service.go:606-640`); chunk size/overlap `1000/200` hard-coded in `pkg/textsplitter/splitter.go:13-18` while a per-document `chunking_config` JSONB column exists (`migrations/00001_baseline.sql:1288`) but is unused by the Go rechunk path.
- **Worker imbalance**: chunk embedding concurrency 10 vs graph 200 (`chunk_embedding_jobs.go:33-65` vs `graph_embedding_jobs.go:33-65`); adaptive scaling defaults `false`.
- **pgx pool**: `MaxConns=25` / `MinConns=5` (`internal/config/config.go:110-111`, `internal/database/database.go:42-43`).
- **Traversal pagination broken**: `TraverseGraph` response hard-codes `HasNextPage:false` (`domain/graph/service.go:3390-3393`), so `pageSize` (default 50, cap 1000) is meaningless.
- **Retrieval traces not persisted**: per-result scores/rank/debug exist in-memory only (`domain/search/dto.go:159-221`), gated by `search:debug` scope; no retrieval-log table.
- **Config surface**: `caarlos0/env`-parsed `Config` struct (`internal/config/config.go`) is the established pattern; search default limit already reads `MEMORY_SEARCH_DEFAULT_LIMIT` (`domain/search/service.go:38`).

## Goals / Non-Goals

**Goals:**
- Remove the unbounded N+1 in unified-search relationship expansion; add a LIMIT and a batched edge-fetch path.
- Add an optional, configurable min-score cutoff across all search modes with sane validation.
- Persist bounded retrieval traces keyed by a stable ID for reproducibility + downstream caching.
- Make the retrieval-performance knobs (probes, RRF, fusion weights, chunking, worker concurrency, pgx pool, search limits) env-configurable with defaults that preserve current behavior.
- Fix `TraverseGraph`/`ExpandGraph` pagination so `HasNextPage`/truncation are real.

**Non-Goals:**
- HNSW index migration, cross-encoder re-ranking, result-cache implementation (only trace persistence — the enabler — is in scope here).
- Multi-project read, claim-level contradiction, object-level ACL, temporal-validity wiring — separate changes.

## Decisions

### 1. Batch edge fetch replaces per-object `GetEdges` in unified search
Add a repository method that fetches edges for a list of object IDs in one query (e.g., `GetEdgesForObjects(ctx, objectIDs, opts)` with `WHERE` on the relationship FK columns + `LIMIT`). Unified search collects result object IDs, calls it once, then groups edges by object. `GetEdges` gains a `LIMIT` (default bounded, cap configurable) for the single-object path.

*Rationale*: the per-object loop is the single largest scale risk. Batching removes N queries. A LIMIT bounds the pathological many-edges object.

*Alternative considered*: keep per-object but add LIMIT only — removes unboundedness but not the N queries; rejected as insufficient.

### 2. Cutoff applied at service layer, not SQL
Filter fused results by `min_score` after scoring (and per-mode candidates before fusion). Use a single clamp-validated float `[0,1]`. No SQL predicate (scores are a mix of lexical/vector/fused computed in Go).

*Rationale*: scores don't exist as a single SQL column; filtering in Go keeps one code path. When `min_score` is unset, behavior is identical to today.

*Alternative considered*: push vector-distance predicates into SQL (`1 - (embedding <=> ?) >= threshold`). Possible for vector-only mode but not for fused/lexical; rejected to keep one consistent semantic.

### 3. Config knobs via `caarlos0/env`, defaults preserved
Add fields to the `Config` struct (or a nested `SearchConfig`/`ChunkingConfig`) parsed from new env vars, with defaults matching current hard-coded values (`probes=10`, `k=60`, `0.25/0.75/0`, chunk `1000/200`, graph concurrency 200, chunk concurrency 10). Wire `ivfflat.probes` into the existing `SET LOCAL` per-query tx; wire chunk size/overlap into `textsplitter` and read per-document `chunking_config` first, then env default.

*Rationale*: matches the established config pattern; zero behavior change when unset. Per-document `chunking_config` finally honors an existing schema column instead of silently ignoring it.

*Alternative considered*: expose knobs via admin API. Larger surface + authz complexity; env is sufficient for the friend's deploy model (self-hosted binary, config at startup). Deferred.

### 4. Traversal pagination via real offset/cursor, not hard-coded flag
Compute `HasNextPage` from `len(returned) == pageSize` (or an explicit count check) and add an offset/cursor parameter to `TraverseGraph` so continuation is possible. `ExpandGraph` exposes a `HasMore` flag when it truncates at `MaxNodes`/`MaxEdges`.

*Rationale*: minimal change that makes existing `pageSize`/cap fields meaningful. Cursor vs offset: offset is simpler and traversal is bounded (depth≤8, nodes≤5000), so offset is adequate.

*Alternative considered*: keyset cursor — more robust under mutation but overkill for bounded traversal; rejected.

### 5. Retrieval traces as a new table + background cleanup
New table `kb.retrieval_traces` (project_id, trace_id, query, filters JSONB, candidate JSONB capped at N, selected_ids, scores JSONB, created_at). Written on search completion (async, best-effort, non-blocking on failure). Cleanup via scheduler job honoring a retention TTL. Trace ID returned in the search response (optionally gated).

*Rationale*: persistence is the friend's exact-packet-reproducibility requirement and the prerequisite for caching. Async write keeps it off the hot path. Bounded candidate storage prevents unbounded growth.

*Alternative considered*: reuse journal/OTel only — journal is mutation-focused, OTel is sampled and proxy-gated; neither gives addressable, stable-ID result reconstruction. Rejected.

## Risks / Trade-offs

- **[Cutoff drops good results under poor embeddings]** → default `min_score` unset (no filtering); document that tuning requires the eval harness; clamp + validate inputs.
- **[Trace persistence adds write load]** → async best-effort write, bounded candidate cap, configurable TTL + cleanup job.
- **[Batched edge fetch changes ordering/limit semantics]** → keep grouping deterministic (stable object order), mirror current edge ordering; guard with existing tests.
- **[Env knobs create config drift across deploys]** → defaults preserve today's values; document all new vars in the config surface.
- **[Adaptive scaling enabled-by-default could over-scale]** → bounded by min/max; keep syshealth monitor as the feeder (already wired).
- **[Offset pagination under concurrent mutation]** → traversal is bounded and read-mostly; document the accepted trade-off, keyset cursor deferred.

## Migration Plan

- Single Goose migration adds `kb.retrieval_traces` (additive; no data backfill). Safe rollback = drop table.
- Env var additions are additive with safe defaults; no migration required for config.
- No existing index/column changes required for the perf work (chunking honors existing `chunking_config` column; probes/RRF/fusion read from config).

## Open Questions

<!-- none — all decisions above are resolved and do not change specs/tasks -->
