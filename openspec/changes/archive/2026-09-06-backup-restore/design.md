## Context

The `backups` domain (`apps/server/domain/backups/`) currently implements a one-directional pipeline: `Exporter` streams 8 tables to NDJSON, `Creator` zips them (plus raw files + a manifest) and uploads to MinIO, and `Service`/`Handler` expose create/list/get/download/delete. Restore is stubbed (`handler.go:289,305` → 501).

Investigation against the live DB (Postgres :5436) found:

- **42 project-scoped tables** exist across `kb` and `core`; the exporter captures 8.
- **pgvector columns** (`chunks.embedding`, `graph_objects.embedding_v2`, `graph_relationships.embedding`) are `vector` UDTs; the exporter scans into `map[string]any` and JSON-encodes, which does not preserve the typed vector form.
- **`documents` has a unique `file_hash` index** and **graph objects have an upsert-unique `(namespace, type, key)`** — these constrain restore ordering and ID collision handling.
- **`skills` is dual-scoped** (`org_id` + `project_id`); `branch_lineage` has no `project_id` (derived from `branches.parent_branch_id`).
- **`kb.restores` does not exist** — no entity, migration, or job model despite the stubbed status endpoint.

## Goals / Non-Goals

**Goals:**
- Restore a project from a backup archive into a fully working state, in two modes: **overwrite** (same project) and **clone** (new project, optionally cross-org).
- Expand export coverage to a curated set of stateful project tables so a restore does not silently drop schemas/branches/agents/skills/settings.
- Preserve embeddings by raw-copying vector columns with an explicit serialization contract.
- Fix file dedup (by `storage_key`) and compute real archive checksums.
- Track restore progress via an async job with a `kb.restores` table.
- Make overwrite safe via a pre-restore snapshot (auto-backup of the live project).

**Non-Goals:**
- Re-embedding on model change — a restore that must swap embedding models is a separate, later story.
- Incremental-backup restore — the schema supports `parent_backup_id`/`baseline_backup_id` but this change restores full snapshots only.
- Merge restore (replay into a live project with conflict resolution) — out of scope; overwrite and clone are the two supported modes.
- Journal replay / branch revert — the journal remains an audit log, not a replay source, in this change.
- Restoring `pg_dump` database backups (superadmin `database-backups`) — those remain download-only.

## Decisions

### 1. Restore supports two modes: overwrite and clone

**Decision**: `POST /api/v1/projects/{projectId}/restore` takes a `mode` field (`overwrite` | `clone`). Overwrite restores into `projectId`; clone creates a new project (optionally in a different org via `targetOrgId`) from the backup.

**Rationale**: Overwrite matches the existing route and the point-in-time-recovery need. Clone supports "give me a copy of this project" and cross-org migration, which the manifest already makes representable (`ProjectInfo` carries `org_id` + `name`).

**Alternative considered**: A single "restore-as-new" endpoint keyed on the backup rather than the project. Rejected because overwrite semantics are naturally scoped to the target project, and keeping both under one verb with a `mode` discriminator is clearer than two parallel verbs.

### 2. Export coverage: curated table triage, not literal "all 42"

**Decision**: The exporter becomes table-driven over an explicit include-list of stateful project tables. Ephemeral, derived, and security-sensitive tables are excluded. New `includeJournal` flag (default `false`) gates `project_journal` + `project_journal_notes`, mirroring the existing `includeChat` flag.

**Include-list** (project-scoped, stateful):

| Area | Tables |
|---|---|
| Core content | `documents`, `chunks`, `graph_objects`, `graph_relationships`, `object_extraction_jobs`, `chat_conversations`, `chat_messages` |
| Graph/schema | `object_type_schemas`, `graph_schemas`, `project_schemas`, `project_object_schema_registry`, `project_edge_schema_registry`, `schema_migration_jobs`, `schema_migration_runs` |
| Branches | `branches` (and `branch_lineage` re-derived from `parent_branch_id`) |
| Config | `project_settings`, `project_model_config`, `project_provider_configs`, `embedding_policies` |
| Agents/skills | `agents`, `agent_definitions`, `agent_webhook_hooks`, `skills` (project-owned only), `mcp_servers` |
| Taxonomy | `tags`, `tasks`, `external_sources`, `product_versions`, `sandbox_images` |
| Membership | `project_memberships` |
| Journal (opt-in) | `project_journal`, `project_journal_notes` |

**Exclude-list** (with reason): `document_parsing_jobs` (transient), `schema_migration_jobs` is re-runnable but kept for fidelity of migration archive — see below, `discovery_jobs` (re-discoverable), `acp_sessions` (transient), `llm_usage_events` (billing/analytics), `user_recent_items` (per-user UI state), `invites`/`notifications` (transient), `core.api_tokens` (security — must never appear in a backup).

**Dual-scoping**: `skills` has both `org_id` and `project_id`. Project-owned skills (`project_id = X`) are exported; org-level skills (`org_id = Y, project_id IS NULL`) are org resources and excluded from a project backup.

**Rationale**: A literal for-loop over all 42 tables would export billing events, auth tokens, and transient jobs — wrong and, in the case of `api_tokens`, dangerous. The triage is the meaningful "expand" decision.

**Alternative considered**: Export every table mechanically and filter at restore. Rejected — it bakes ephemeral data into archives and exposes secrets to download.

### 3. Vector serialization: JSON float array, raw-copy

**Decision**: pgvector columns are serialized in NDJSON as a JSON array of floats (e.g. `"embedding": [0.123, -0.456, ...]`). The importer detects vector columns by UDT name (`vector`, `halfvec`), parses the array to `[]float32`, and re-inserts with an explicit `::vector` cast. No re-embedding is performed.

**Rationale**: Raw-copy is deterministic, fast, and cheap. It preserves the exact vectors that were indexed. Re-embedding depends on the active embedding policy/model at restore time and is a separate concern.

**Alternative considered**: Re-embed on restore. Rejected — costs money/time, may not match the original model, and fails if the original provider/model is unavailable. Raw-copy is the correct default; re-embed becomes an explicit opt-in story later.

### 4. File export: dedupe by storage_key

**Decision**: ZIP entries under `files/` are keyed by `storage_key` (path-sanitized), not `filename`. The manifest gains a `files` map: `storage_key → { filename, mime_type }`. On restore, files are re-uploaded to MinIO under their original `storage_key`, preserving the `documents.storage_key` column.

**Rationale**: `filename` collides (two docs named `report.pdf`). `storage_key` is unique per document and is what the documents table actually references. Keeping filename as manifest metadata preserves the human-facing name for download.

### 5. Real checksums

**Decision**: `creator.go` computes SHA-256 checksums for the manifest, the concatenated database NDJSON stream, and the file payloads, writing them into `manifest.checksums` and the `Backup.manifest_checksum`/`content_checksum` columns. Restore validates checksums before applying and aborts with a clear error on mismatch.

**Rationale**: The archive currently self-reports integrity with empty values. Without real checksums, restore cannot distinguish corruption from a valid archive.

### 6. Overwrite mechanics: transactional wipe-then-insert

**Decision**: Overwrite restore runs in a single DB transaction: (1) snapshot the live project (Decision 7), (2) delete existing project rows in FK-safe order, (3) insert snapshot rows, (4) commit. Any failure rolls back the whole transaction, leaving the project untouched.

**Rationale**: Wipe-then-insert is the only semantics that actually produce a point-in-time snapshot — upsert would leave live-only rows intact, which is merge, not restore. Wrapping it in one transaction removes the partial-state risk of a naive wipe. This is the future-proof choice: incremental restore can later compose by applying baseline then deltas, each in its own transaction.

**Alternative considered**: Upsert (`ON CONFLICT DO UPDATE`). Rejected — it does not produce a true restore; rows present in the live project but absent from the snapshot would persist.

### 7. Pre-restore snapshot on overwrite

**Decision**: Before an overwrite restore, the service automatically creates a backup of the live project (a "pre-restore snapshot", `backup_type = "full"`, marked via `metadata.reason = "pre-restore"`). Enabled by default (`preRestoreSnapshot: true` in the request).

**Rationale**: Overwrite is destructive. A pre-restore snapshot makes it reversible and costs one extra archive. The existing `Creator` is reused unchanged.

### 8. Clone mechanics: ID remap, membership copy, cross-org

**Decision**: Clone restore creates a new `kb.projects` row and remaps every entity UUID (documents, chunks, graph objects, relationships, chat, etc.) through an ID-map, rewriting FK references in dependency order. `project_memberships` from the snapshot are copied onto the new project. Cross-org is allowed via `targetOrgId`; the restorer must be a member of the target org. For cross-org, memberships are copied only for users who resolve to the target org.

**Name**: Caller supplies `targetProjectName`; default is `"{original name} (restored {YYYY-MM-DD})"`.

**Rationale**: Remapping is mandatory for clone — preserving original UUIDs would collide with the source project. Copying memberships preserves the team; cross-org filtering avoids dangling member references.

**Alternative considered**: Clone with preserved IDs (only if source project is deleted first). Rejected — it forbids side-by-side copies and cross-org use.

### 9. Restore job model and API surface

**Decision**: New `kb.restores` table tracks async restore jobs. API:

- `POST /api/v1/projects/{projectId}/restore` — overwrite mode (`mode="overwrite"`, body: `{ backupId, preRestoreSnapshot, includeJournal }`)
- `POST /api/v1/organizations/{orgId}/restore` — clone mode (`mode="clone"`, body: `{ backupId, targetProjectName, includeJournal }`)
- `GET /api/v1/restores/{restoreId}` — job status/progress (top-level, since clone crosses orgs)

**Rationale**: Restore is long-running (streaming unzip + bulk insert + file re-upload), so it must be async with progress — matching the create path's async model. The status endpoint moves to top-level because a clone may target a different org than the source.

**Alternative considered**: Synchronous restore. Rejected — large projects would exceed request timeouts.

### 10. Restore job record shape

```go
type Restore struct {
    bun.BaseModel `bun:"table:kb.restores,alias:r"`
    ID            string     `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
    OrganizationID string    `bun:"organization_id,notnull,type:uuid" json:"organizationId"`
    BackupID      string     `bun:"backup_id,notnull,type:uuid" json:"backupId"`
    Mode          string     `bun:"mode,notnull" json:"mode"` // overwrite | clone
    SourceProjectID string   `bun:"source_project_id,type:uuid" json:"sourceProjectId"`
    TargetProjectID string   `bun:"target_project_id,type:uuid" json:"targetProjectId"`
    Status        string     `bun:"status,notnull,default:'pending'" json:"status"` // pending|running|completed|failed
    Progress      int        `bun:"progress,notnull,default:0" json:"progress"`
    ErrorMessage  *string    `bun:"error_message" json:"errorMessage,omitempty"`
    Stats         map[string]any `bun:"stats,type:jsonb" json:"stats,omitempty"`
    CreatedAt     time.Time  `bun:"created_at,notnull,default:now()" json:"createdAt"`
    CreatedBy     *string    `bun:"created_by,type:uuid" json:"createdBy,omitempty"`
    CompletedAt   *time.Time `bun:"completed_at" json:"completedAt,omitempty"`
}
```
