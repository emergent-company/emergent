## 1. Database Migrations

- [x] 1.1 Write Goose migration: create `kb.restores` table (`id`, `organization_id`, `backup_id`, `mode`, `source_project_id`, `target_project_id`, `status`, `progress`, `error_message`, `stats` JSONB, `created_at`, `created_by`, `completed_at`) with status CHECK (`pending`/`running`/`completed`/`failed`) and indexes on `(organization_id)`, `(status)`, `(backup_id)`
- [x] 1.2 Verify migration runs cleanly against local Postgres (:5436)

## 2. Export Coverage

- [x] 2.1 Introduce a table-driven export config: an ordered list of `{ table, type, projectFilterColumn, vectorColumns }` entries replacing the hardcoded 8 methods in `exporter.go`
- [x] 2.2 Add export methods for the curated stateful tables (object_type_schemas, graph_schemas, project_schemas, schema registries, schema_migration_jobs/_runs, branches, project_settings, project_model_config, project_provider_configs, embedding_policies, agents, agent_definitions, agent_webhook_hooks, skills, mcp_servers, tags, tasks, external_sources, product_versions, sandbox_images)
- [x] 2.3 Re-derive `branch_lineage` from `branches.parent_branch_id` on export (no direct project_id column)
- [x] 2.4 Filter `skills` to project-owned rows only (`project_id = X`)
- [x] 2.5 Add `includeJournal` flag to `CreateBackupRequest` + `CreateBackupRequestDTO`; gate `project_journal`/`project_journal_notes`
- [x] 2.6 Serialize vector columns as JSON float arrays; detect by UDT (`vector`/`halfvec`)
- [x] 2.7 Dedupe file export by `storage_key` (path-sanitized); write `files` map (`storage_key → {filename, mime_type}`) into manifest
- [x] 2.8 Compute SHA-256 checksums (manifest, database NDJSON, files) and populate `manifest.checksums` + `Backup.manifest_checksum`/`content_checksum`
- [x] 2.9 Update `BackupStats`/`Manifest` structs for the expanded surface + checksums + files map

## 3. Importer (reverse pipeline)

- [x] 3.1 Create `importer.go`: stream-unzip the archive, read `manifest.json`, validate checksums
- [x] 3.2 Topo-sort tables by FK dependencies: projects → documents → chunks → graph_objects → graph_relationships → chat_conversations → chat_messages → memberships → (curated set)
- [x] 3.3 Parse NDJSON rows; special-case vector columns (parse float array → `[]float32`, cast `::vector` on insert)
- [x] 3.4 Re-upload `files/*` to MinIO under original `storage_key`; restore `documents.storage_key`

## 4. Restore Orchestration

- [x] 4.1 Add `Restore` entity + status constants to `entity.go`
- [x] 4.2 Add `Repository` methods: `CreateRestore`, `GetRestore`, `UpdateRestore`
- [x] 4.3 Create `restorer.go` with `Restore(ctx, job, req)` dispatch on `mode`
- [x] 4.4 Implement overwrite: single transaction, pre-restore snapshot (Decision 7), FK-ordered wipe-then-insert, progress updates
- [x] 4.5 Implement clone: create `kb.projects` row, ID remap table, rewrite FKs in dependency order, copy memberships (cross-org filtered), default name `"{name} (restored {date})"`
- [x] 4.6 Wire async job: `Service.CreateRestore` enqueues goroutine; update progress/status on each phase
- [x] 4.7 Implement pre-restore snapshot via existing `Creator` with `metadata.reason = "pre-restore"`

## 5. API Surface

- [x] 5.1 Implement `Handler.RestoreBackup` (overwrite + clone) replacing the 501 stub; return 202 with job
- [x] 5.2 Implement `Handler.GetRestoreStatus` (top-level `GET /api/v1/restores/:restoreId`)
- [x] 5.3 Update `routes.go`: `POST /projects/{projectId}/restore`, `POST /organizations/{orgId}/restore`, `GET /restores/:restoreId`
- [x] 5.4 Add request/response DTOs: `RestoreRequest` (`backupId`, `mode`, `preRestoreSnapshot`, `includeJournal`, `targetProjectName`), `RestoreResponse`

## 6. Verification

- [x] 6.1 `go build ./...` in `apps/server`
- [x] 6.2 `task test` (unit) in `apps/server`
- [x] 6.3 `task test:e2e` — backup → overwrite restore round-trip; backup → clone restore round-trip
- [x] 6.4 Manual round-trip: create project with schemas/branches/agents/skills/vectors, backup, overwrite-restore, verify vectors + schemas + branches survive
- [x] 6.5 Manual cross-org clone: verify membership filtering and new project ID
- [x] 6.6 Update spec files if behavior diverges from these artifacts during implementation
