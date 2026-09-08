## 1. Config surface

- [x] 1.1 Add env-parsed fields to `Config` (nested `ChunkingConfig`/`EmbeddingQueueConfig`) for chunk size/overlap and embedding worker concurrency + adaptive scaling; search knobs (probes, RRF k, fusion weights, max limit) read via env in `search.NewService` — verify `go build ./...` passes and defaults equal current hard-coded values (probes=10, k=60, 0.25/0.75/0, 1000/200, graph 200/chunk 10)
- [x] 1.2 Wire `ivfflat.probes` into the existing `SET LOCAL` per-query transaction in `domain/search/repository.go` and `domain/graph/repository.go` — verify a search with `SEARCH_IVFFLAT_PROBES=40` executes `SET LOCAL ivfflat.probes = 40` (inspect query log / `DB_QUERY_DEBUG`)

## 2. Similarity-score cutoff

- [x] 2.1 Add `min_score` (float `[0,1]`, clamp-validated) to search request DTOs and validation — verify out-of-range values return HTTP 400
- [x] 2.2 Apply cutoff in `domain/search/service.go`: filter per-mode candidates and fused results below threshold — verify `min_score=0.75` with all-low-scoring corpus returns empty (no forced floor), and omitted threshold preserves baseline results
- [x] 2.3 Thread `min_score` through graph search and chunk text search service paths — verify `mode=both`, `text`, and `graph` each respect the threshold

## 3. N+1 relationship expansion

- [x] 3.1 Add `GetEdgesForObjects(ctx, objectIDs, opts)` batched repository method in `domain/graph/repository.go` with a `LIMIT` bound — verify unit test returns edges for N objects in one query
- [x] 3.2 Add `LIMIT` (bounded, configurable) to single-object `GetEdges` — verify a many-edges object returns at most the limit
- [x] 3.3 Replace the per-result `GetEdges` loop in unified search (`domain/search/service.go`) with the batched call + grouping — verify relationship expansion results are unchanged vs prior behavior on an existing fixture (order + content identical)

## 4. Traversal pagination

- [x] 4.1 Compute `HasNextPage` in `TraverseGraph` from `len(returned) == pageSize` and add offset/cursor param — verify traversal with `pageSize=50` over >50 nodes returns `HasNextPage=true`
- [x] 4.2 Add offset continuation so a follow-up request returns the next non-overlapping page — verify page 2 differs from page 1 and page N returns `HasNextPage=false`
- [x] 4.3 Expose a `HasMore`/truncation flag on `ExpandGraph` when `MaxNodes`/`MaxEdges` caps are hit — verify a bounded expansion over a dense graph reports truncation

## 5. Retrieval trace persistence

- [x] 5.1 Add Goose migration for `kb.retrieval_traces` (project_id, trace_id, query, filters JSONB, candidates JSONB capped, selected_ids, scores JSONB, created_at) — verify migration applies cleanly (`goose up`) and is additive
- [x] 5.2 Write Bun store + async best-effort trace writer, invoked on search completion — verify a completed search produces a trace row and a stable `trace_id` is returned in the response
- [x] 5.3 Add trace fetch by ID (API or internal) and reconstruction of ordered selected IDs — verify the exact ordered result list is recoverable from a trace ID without re-scoring
- [x] 5.4 Add scheduler cleanup job honoring retention TTL — verify expired traces are removed and candidate storage is bounded at the configured cap

## 6. Chunking config

- [x] 6.1 Wire `pkg/textsplitter` to read env chunk size/overlap defaults, and `domain/chunking/service.go` to honor project-level `kb.projects.chunking_config` JSONB (project > env > 1000/200) — verify re-chunking a doc in a project with `chunking_config {maxChunkSize:4000, overlap:800}` produces 4000/800 chunks, and no config falls back to 1000/200

## 7. Worker concurrency + pgx pool

- [x] 7.1 Make embedding worker concurrency/batch/adaptive-scaling env-configurable and default adaptive scaling to enabled with bounded min/max — verify chunk queue honors configured concurrency and adaptive scaling engages under load
- [x] 7.2 Confirm pgx `MaxConns`/`MinConns` env wiring and document multi-tenant guidance — verify `DB_MAX_OPEN_CONNS=100` yields `MaxConns=100` at startup

## 8. Verification

- [x] 8.1 Run `go build ./...`, `go vet ./...`, and the search/graph/chunking/scheduler/extraction unit suites — verify all pass with no behavior regressions when new knobs are unset
- [ ] 8.2 Manual smoke test: unified search with `min_score`, traversal pagination, and a persisted trace end-to-end via DevTools — verify each capability observable through the API (requires migration 00143 applied + server restart + live DB/auth)
