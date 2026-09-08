# backup-export-coverage

## Purpose

Defines the project backup export surface: the curated set of stateful project-scoped tables, raw-copy embedding serialization, archive checksums, and storage-key-based file deduplication.

## Requirements

### Requirement: Backup export covers the full project surface
The backup exporter SHALL export a curated set of stateful project-scoped tables, not just the original eight. The set SHALL include documents, chunks, graph objects and relationships, chat, extraction jobs, object type schemas and schema registries, branches, project settings and provider configs, embedding policies, agents and agent definitions, project-owned skills, MCP servers, tags, tasks, external sources, product versions, sandbox images, and project memberships.

The exporter SHALL exclude ephemeral, derived, and security-sensitive tables: document parsing jobs, discovery jobs, ACP sessions, LLM usage events, user recent items, invites, notifications, and API tokens.

#### Scenario: Full backup includes schemas and branches
- **WHEN** a user creates a backup of a project that has object type schemas, branches, and agent definitions
- **THEN** the backup archive SHALL contain NDJSON for `object_type_schemas`, `branches`, and `agent_definitions` in addition to the original eight tables
- **THEN** a restore from that archive SHALL recreate the schemas and branches

#### Scenario: Security-sensitive tables are never exported
- **WHEN** a user creates a backup of any project
- **THEN** the archive SHALL NOT contain `core.api_tokens` or any API token material
- **THEN** the archive SHALL NOT contain `llm_usage_events`, `acp_sessions`, `user_recent_items`, `invites`, `notifications`, or `document_parsing_jobs`

#### Scenario: Journal is opt-in
- **WHEN** a user creates a backup without `includeJournal`
- **THEN** the archive SHALL NOT contain `project_journal` or `project_journal_notes`
- **WHEN** a user creates a backup with `includeJournal: true`
- **THEN** the archive SHALL contain `project_journal` and `project_journal_notes`

#### Scenario: Project-owned skills only
- **WHEN** an org has org-level skills (`org_id` set, `project_id` NULL) and a project has project-owned skills
- **WHEN** a backup of the project is created
- **THEN** the archive SHALL contain only the project-owned skills
- **THEN** org-level skills SHALL NOT be exported

### Requirement: Embeddings are raw-copied as JSON float arrays
pgvector columns (`chunks.embedding`, `graph_objects.embedding_v2`, `graph_relationships.embedding`) SHALL be serialized in NDJSON as a JSON array of floats. Restore SHALL parse these arrays back into vectors and re-insert them with a `::vector` cast. No re-embedding SHALL occur during backup or restore.

#### Scenario: Vector round-trips faithfully
- **WHEN** a chunk with an embedding is backed up and then restored
- **THEN** the restored chunk SHALL have an embedding equal to the original (within float precision)
- **THEN** no embedding API call SHALL be made during restore

#### Scenario: Vector column is detected by type
- **WHEN** the exporter encounters a column of UDT type `vector` or `halfvec`
- **THEN** it SHALL serialize that column as a JSON float array
- **THEN** the importer SHALL parse the array and re-insert with the appropriate vector cast

### Requirement: File export deduplicates by storage key
File payloads in the backup archive SHALL be keyed by `storage_key` (path-sanitized), not `filename`. The manifest SHALL carry a mapping from `storage_key` to `{ filename, mime_type }` so the human-facing name is preserved on restore.

#### Scenario: Two documents share a filename
- **WHEN** a project has two documents both named `report.pdf` with distinct storage keys
- **WHEN** a backup is created
- **THEN** both files SHALL appear in the archive under distinct storage-key-derived paths
- **THEN** neither file SHALL overwrite the other

### Requirement: Archive checksums are computed and validated
The backup creator SHALL compute SHA-256 checksums for the manifest, the database NDJSON content, and the file payloads, writing them into `manifest.checksums` and the backup record's checksum columns. Restore SHALL validate checksums before applying and SHALL abort with a clear error on mismatch.

#### Scenario: Checksums are populated
- **WHEN** a backup completes successfully
- **THEN** `manifest.checksums.manifest`, `manifest.checksums.database`, and `manifest.checksums.files` SHALL be non-empty
- **THEN** the `Backup.manifest_checksum` and `Backup.content_checksum` columns SHALL be populated

#### Scenario: Corrupt archive is rejected
- **WHEN** a restore is requested from an archive whose checksum does not match its content
- **THEN** the restore SHALL fail with a checksum mismatch error and SHALL NOT modify any project data
