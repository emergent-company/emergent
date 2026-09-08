## Why

Project backups exist — create, list, get, download, delete — plus a superadmin `pg_dump` job. But the restore endpoints are hard stubs returning HTTP 501 (`apps/server/domain/backups/handler.go:289,305`). There is no importer, no restore job model, and no reverse path for the archive format. The backup feature currently provides **zero recovery value**.

Worse, the backup is not a complete project: the exporter captures only **8 of 42 project-scoped tables** (documents, chunks, graph_objects, graph_relationships, chat_conversations, chat_messages, object_extraction_jobs, project_memberships). It silently omits schemas, branches, agents, skills, tags, settings, provider configs, and journal history — so even a hypothetical restore would produce a semantically broken project whose graph objects reference object types that no longer exist.

Two further correctness gaps compound the problem:

- **pgvector embeddings** (`chunks.embedding`, `graph_objects.embedding_v2`, `graph_relationships.embedding`) are scanned into `map[string]any` and JSON-encoded, losing their typed vector form. Re-insertion is impossible without an explicit cast and serialization contract.
- **File export** keys ZIP entries by `filename`, so two documents named `report.pdf` silently overwrite each other (`creator.go:295`).
- **Checksums** are empty (`creator.go:345` `TODO: Calculate actual checksums`), so archive integrity is unverifiable.

## What Changes

- **Expand export coverage** to a curated full-project surface (~25 stateful tables) driven by an explicit include-list with a documented triage that excludes ephemeral, derived, and security-sensitive tables.
- **Raw-copy embeddings**: serialize pgvector columns as JSON float arrays in NDJSON; the importer special-cases vector columns and re-inserts with a `::vector` cast. Re-embedding on model change is explicitly out of scope (separate future story).
- **Fix file export**: dedupe ZIP entries by `storage_key`, with a manifest mapping `storage_key → filename/mime_type` for faithful re-upload.
- **Compute real checksums** (manifest, database, files) so restore can validate integrity before applying.
- **Implement the restore pipeline** with two modes:
  - **Overwrite** — point-in-time restore into the existing project: single-transaction, FK-ordered, wipe-then-insert (all-or-nothing).
  - **Clone** — restore as a new project (possibly in a different org): remap all IDs, copy memberships, caller-supplied name with a parenthetical default.
- **Pre-restore snapshot** on overwrite: auto-backup the live project before wiping it, so overwrite is itself reversible.
- **Add a `kb.restores` job table** and progress tracking, wiring the stubbed `POST /restore` and `GET /restores/:restoreId` endpoints into a real async job.

## Capabilities

### New Capabilities

- `backup-export-coverage`: full-project export via curated table triage, raw-copy vector serialization, real checksums, and storage_key-based file dedup.
- `backup-restore`: restore pipeline with overwrite and clone modes, ID remapping, membership copy, cross-org clone, pre-restore snapshot, and async restore job tracking.

## Impact

- **`backups/exporter.go`**: table-driven export over the curated table set; vector raw-copy; file dedup.
- **`backups/creator.go`**: manifest checksums; `includeJournal` flag.
- **`backups/importer.go`** (new): reverse pipeline — parse NDJSON, special-case vector columns, FK-ordered insert.
- **`backups/restorer.go`** (new): overwrite/clone orchestration, ID remap, pre-restore snapshot.
- **`backups/entity.go`**: `Restore` job entity + request/response DTOs.
- **`backups/handler.go`**: implement `RestoreBackup` + `GetRestoreStatus`.
- **`backups/routes.go`**: restore endpoints (overwrite + clone + status).
- **Migrations**: new `kb.restores` table.
- **`internal/storage`**: file re-upload during restore (preserving/remapping `storage_key`).
