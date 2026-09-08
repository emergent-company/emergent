# Verification Plan — `backup-restore`

Evidence path for the backup-restore change. Follow this after implementation to establish, limit, or refute the round-trip fidelity claim. Reference during apply-change.

## 1. Claims

**Primary claim:** A full backup of a rich project can be restored — overwrite (same ID) or clone (new ID, possibly cross-org) — to a state *semantically equivalent* to the snapshot: same curated tables, same row content, equal embeddings, intact FK/unique constraints, valid file references, correct membership; overwrite is atomic, clone leaves the source untouched.

**Traps that invalidate a confident conclusion:**

1. **Silent success** — restore reports `completed` but dropped a table, mangled a vector, or left a dangling FK. Invisible, irreversible, and the failure that matters most.
2. **ID-invariance confusion** — a naive before/after diff of clone always differs (IDs are remapped). If evidence doesn't normalize IDs, a correct clone looks broken and a broken clone looks fine.
3. **Vector precision** — float round-trip through JSON `map[string]any` → string → `[]float32` may lose precision or change representation. Equality must use tolerance, not exact string match.
4. **File identity** — "two files named `report.pdf`" collision on export and byte-corruption on re-upload both look identical at the row level.

## 2. Evidence path

Oracle derived from the system itself — **the export path is its own verification oracle**.

```
            seed P ──backup──▶ archive A1
                                    │
                              restore (overwrite|clone)
                                    ▼
            P'  ──backup──▶ archive A2
                                    │
                        compare A1 ≡ A2
```

**Double-backup differential:** if backup→restore→backup produces an archive equivalent to the original, the round-trip is faithful — without any direct DB access. It exercises the real server path (async job, storage, importer, exporter) end-to-end.

**Comparison rules (per archive):**

| Dimension | Overwrite | Clone |
|---|---|---|
| Table set | identical | identical |
| Row counts (per curated table) | equal | equal |
| Row content | full hash incl. IDs (IDs preserved) | multiset of content-column hashes, IDs + FK excluded |
| Embeddings | multiset equality (tolerance `1e-4`) | same |
| Files | byte-equal, keyed by `storage_key` | byte-equal |
| FK integrity | all FKs resolve | all FKs resolve *within clone* |
| Membership | — | copied set equals snapshot |
| Source intact | n/a | source unmodified, zero shared UUIDs |

The **content-fingerprint multiset** (order-independent set of row hashes with identity/FK columns stripped) is what makes clone verifiable — it sidesteps ID remapping and FK renumbering.

**Alternatives considered:**

| Path | Verdict |
|---|---|
| A1. Double-backup differential | **Chosen.** Decisive, reuses built path, no MinIO/DB helper needed |
| A2. API-level smoke (restore then query) | Weak — can't see silent table drops or vector corruption |
| A3. `MemoryDB()` direct diff | Strong *complement* for FK integrity + `api_tokens` absence; secondary, not primary |

**Limitation of A1:** it trusts the exporter. A bug that drops a table identically in both exporter and importer would evade the diff. Mitigated by A3 (`MemoryDB()` asserts every curated table has rows after restore) — a cross-check that breaks exporter/importer symmetry.

## 3. Verification affordances

Decisive truth is currently indirect (archive → archive). Two small affordances make it direct and reusable:

1. **`projectFingerprint` helper** (durable, e2e framework): given a project ID, returns normalized per-table `{count, content-multiset-hash, vector-multiset}`. Encoding of "identity vs FK vs content" columns lives here once. Reusable for any future round-trip or migration test.
2. **`seedRichProject` fixture** (durable): deterministically creates documents + files, chunks + embeddings, graph objects/relationships + embeddings, object-type schemas, branches, agents, skills, tags, settings, memberships. The "repeatable known state" every scenario starts from.

**Lifecycle:** both durable — they encode the curated-table list and are reused by overwrite/clone/cross-org scenarios.

## 4. Research — spike, not @librarian

The only unknown dependency behavior: **what pgx/bun emits when scanning a `vector` column into `map[string]any`** (string form? `[]float64`? hex?). Stable, version-pinned local behavior — empirically testable against the live DB, strictly better than docs research.

**Spike (pre-implementation):** `SELECT embedding FROM kb.chunks LIMIT 1` into `map[string]any`, print the JSON. Fixes the exporter serialization format *and* the tolerance the importer needs.

## 5. Run budget — claims → owner → minimum evidence

| # | Claim | Owner | Evidence (non-duplicative) |
|---|---|---|---|
| 1 | Curated tables exported; ephemeral/security absent | unit (`domain/backups`) | manifest lists expected tables; `api_tokens`/`llm_usage_events` absent |
| 2 | Vector round-trip fidelity | unit (importer) + e2e | spike result fixes format; A1 vector-multiset equality within tolerance |
| 3 | File dedup + byte fidelity | e2e | A1 file-byte equality; two same-named files both present |
| 4 | Overwrite transactional/atomic | e2e | A1 overwrite equivalence + pre-restore snapshot exists + counts intact |
| 5 | Clone: remap + membership + cross-org + source intact | e2e | A1 clone equivalence; `MemoryDB()` FK-integrity + zero shared UUIDs + membership set |
| 6 | Checksums computed + corrupt-archive aborts | unit (creator/importer) | truncated archive → restore fails, no data change |
| 7 | Async job progress | e2e | poll `GET /restores/:id` transitions pending→running→completed |

**Repository/release checks still apply** (gates, not evidence): `go build ./...`, `templ generate` if templ changes (none expected), `task lint`, server hot-reload check, `task test` + `task test:e2e`.

## 6. Close — how to read the result

- **Established** if A1 archives equivalent + A3 FK/absence cross-check passes + claims 1–7 green.
- **Limited** if A1 passes but A3 finds a dangling FK or missing-table row → exporter/importer symmetry bug, fix and re-run both.
- **Refuted** if vector multiset differs beyond tolerance, or clone shares a UUID with source.
