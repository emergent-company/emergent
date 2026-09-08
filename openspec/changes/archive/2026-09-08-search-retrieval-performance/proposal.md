## Why

Memory is being evaluated as the backend for a multi-tenant marketing-execution platform (Sequence). That adoption depends on retrieval being fast, correct, and tunable under concurrent multi-tenant load. Current retrieval paths have three structural problems that surface under load: an unbounded N+1 relationship expansion (`GetEdges` has no LIMIT, called once per result), no similarity-score cutoff (every search forces a floor of 20/50 candidates even when they are all noise), and a hard-coded set of performance knobs (`ivfflat.probes=10`, RRF `k=60`, chunk size 1000/200, worker concurrency, pgx pool) that cannot be adjusted per deployment or per tenant. Broken graph-traversal pagination (`HasNextPage` hard-coded `false`) and non-persisted retrieval traces round out the gaps.

## What Changes

- **New**: Similarity-score cutoff on unified/graph/text/relationship search paths. Results below a configurable minimum score are dropped instead of force-filling a candidate floor.
- **New**: Persisted retrieval traces — every search records query, filters, candidate set, per-result scores, selected node/passage IDs, and stable trace ID. Enables exact-packet reproducibility and downstream result caching.
- **New**: Correct pagination on graph traversal (`TraverseGraph`, `ExpandGraph`): `HasNextPage` computed, `pageSize`/offset honored, removed hard-coded `false`.
- **New**: Configurable retrieval-performance knobs exposed through the existing `caarlos0/env` config surface: `ivfflat.probes`, RRF constant, fusion weights, chunk size/overlap (wiring the currently-unused per-document `chunking_config` column), worker concurrency + adaptive scaling, pgx pool sizing, and search result limits.
- **Changed**: Unbounded per-result relationship expansion (`GetEdges`) gets a LIMIT and a batched edge fetch path to remove the N+1 in unified search.
- **Changed**: Embedding worker concurrency balanced and adaptive scaling enabled by default so the chunk path (stuck at 10) no longer bottlenecks behind the graph path (200).

Out of scope: HNSW index migration, cross-encoder re-ranking, multi-project read, claim-level contradiction, object-level ACL/audience, temporal-validity wiring. Those are separate changes.

## Capabilities

### New Capabilities

- `search-similarity-threshold`: Min-score cutoff across all search modes, configurable, with a defined behavior when all candidates fall below threshold.
- `graph-traversal-pagination`: `TraverseGraph` and `ExpandGraph` return correct `HasNextPage`, honor `pageSize`, and support offset continuation.
- `retrieval-trace-persistence`: Persisted, addressable retrieval traces (query, filters, candidates, scores, selections) with stable trace IDs.
- `retrieval-performance-config`: Environment-driven knobs for probes, RRF, fusion weights, chunking, worker concurrency, pgx pool, and search limits.

### Modified Capabilities

<!-- No existing spec-level requirements change. New capabilities only. -->

## Impact

- `domain/search/service.go` + `repository.go`: score cutoff, probe/RRF/fusion config, batched edge fetch, persisted traces.
- `domain/graph/service.go` + `repository.go`: traversal pagination fix, `GetEdges` LIMIT.
- `pkg/textsplitter/splitter.go`: chunk size/overlap driven by config / per-document `chunking_config`.
- `internal/config/config.go` (+ features): new env knobs.
- `domain/extraction/*_embedding_jobs.go`: worker concurrency + adaptive scaling defaults.
- `internal/database/database.go`: pgx pool env surface.
- New migration: retrieval trace table.
