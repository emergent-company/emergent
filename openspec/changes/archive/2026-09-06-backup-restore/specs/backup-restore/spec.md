## ADDED Requirements

### Requirement: Restore supports overwrite and clone modes
The system SHALL restore a project from a backup archive in one of two modes: **overwrite** (restore into the existing project) or **clone** (restore as a new project). The mode SHALL be specified in the restore request.

#### Scenario: Overwrite restore
- **WHEN** a user requests `mode="overwrite"` for a project with a `backupId`
- **THEN** the existing project SHALL be replaced with the snapshot state from the backup
- **THEN** the project SHALL retain its original ID

#### Scenario: Clone restore
- **WHEN** a user requests `mode="clone"` with a `backupId`
- **THEN** a new project SHALL be created with the snapshot's content
- **THEN** the new project SHALL have a distinct ID from the source project
- **THEN** the source project SHALL be left unmodified

### Requirement: Overwrite restore is transactional
Overwrite restore SHALL run in a single database transaction: delete existing project rows in FK-safe order, insert snapshot rows, then commit. If any step fails, the entire transaction SHALL roll back, leaving the project unchanged.

#### Scenario: Mid-restore failure leaves project intact
- **WHEN** an overwrite restore fails partway through inserting snapshot rows
- **THEN** the transaction SHALL roll back
- **THEN** the project SHALL contain its pre-restore state, not a partial snapshot

### Requirement: Pre-restore snapshot on overwrite
Before an overwrite restore modifies the live project, the system SHALL create a backup of the current project state (a pre-restore snapshot) so the overwrite is reversible. This SHALL be enabled by default and can be disabled with `preRestoreSnapshot: false`.

#### Scenario: Pre-restore snapshot is created
- **WHEN** an overwrite restore begins with `preRestoreSnapshot: true`
- **THEN** a backup of the live project SHALL be created before any data is wiped
- **THEN** the snapshot SHALL be marked as a pre-restore snapshot in its metadata

### Requirement: Clone remaps all entity IDs
Clone restore SHALL remap every entity UUID through an ID-map, rewriting foreign-key references in dependency order. Original IDs SHALL NOT be reused in the clone.

#### Scenario: Clone has no ID collisions
- **WHEN** a clone restore completes
- **THEN** no entity in the clone SHALL share a UUID with the source project
- **THEN** foreign-key references within the clone (chunks → documents, relationships → objects) SHALL resolve to the remapped IDs

### Requirement: Clone copies memberships and supports cross-org
Clone restore SHALL copy `project_memberships` from the snapshot onto the new project. Clone SHALL support cross-org restore via `targetOrgId`; the restorer MUST be a member of the target org. For cross-org clones, memberships SHALL be copied only for users who resolve to the target org.

#### Scenario: Same-org clone copies memberships
- **WHEN** a clone restore is performed into the same org
- **THEN** the new project SHALL have the same membership set as the snapshot

#### Scenario: Cross-org clone filters memberships
- **WHEN** a clone restore is performed into a different org (`targetOrgId`)
- **WHEN** some snapshot members do not exist in the target org
- **THEN** only members who resolve to the target org SHALL be copied
- **THEN** the restorer SHALL be added as a member of the new project

#### Scenario: Clone default name includes parenthetical
- **WHEN** a clone restore is requested without `targetProjectName`
- **THEN** the new project SHALL be named `"{original name} (restored {YYYY-MM-DD})"`

### Requirement: Restore is asynchronous with job tracking
Restore SHALL run asynchronously and record progress in a `kb.restores` job. The client SHALL be able to poll restore status.

#### Scenario: Restore returns a job ID
- **WHEN** a restore is requested
- **THEN** the server SHALL return the restore job with status `pending` (HTTP 202)
- **THEN** the job SHALL progress through `pending` → `running` → `completed` or `failed`

#### Scenario: Restore status is pollable
- **WHEN** a client requests `GET /api/v1/restores/{restoreId}`
- **THEN** the server SHALL return the job's status, progress percentage, and error message if failed

### Requirement: Restore re-uploads files
Restore SHALL re-upload file payloads from the archive to MinIO under their original `storage_key`, and SHALL restore the `documents.storage_key` column to reference them.

#### Scenario: Files are restored
- **WHEN** a restore completes for a project whose snapshot contains files
- **THEN** the files SHALL be present in MinIO under their original storage keys
- **THEN** restored documents SHALL reference their correct storage keys

### Requirement: Restore validates the archive before applying
Restore SHALL validate the manifest and checksums before modifying any data. An invalid or mismatched archive SHALL abort the restore with no data changes.

#### Scenario: Invalid archive aborts cleanly
- **WHEN** a restore is requested from an archive with a missing manifest or checksum mismatch
- **THEN** the restore SHALL fail with a validation error
- **THEN** no project data SHALL be modified
