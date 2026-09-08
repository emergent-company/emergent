## Purpose

Let a project manually override the per-model rate used for LLM cost estimation, so an operator can correct a missing or wrong retail price without editing global pricing.

## ADDED Requirements

### Requirement: Project-level pricing override records

The system SHALL store project-level pricing overrides keyed by `project_id`, `provider`, and `model`, with per-modality prices (text/image/video/audio input and output) in USD per 1 million tokens.

#### Scenario: Override is project-scoped and unique

- **WHEN** two projects set an override for the same `provider` and `model`
- **THEN** each override is stored independently and does not affect the other project

### Requirement: Manage project pricing overrides via API

The system SHALL expose authenticated, project-scoped endpoints to list, upsert, and delete a project's pricing overrides.

#### Scenario: Upsert an override

- **WHEN** an authorized caller upserts an override for a project with `provider` and `model` and a set of prices
- **THEN** the override is stored and returned, replacing any prior override for the same `provider` + `model` pair

#### Scenario: List overrides

- **WHEN** an authorized caller lists a project's pricing overrides
- **THEN** all overrides for that project are returned

#### Scenario: Delete an override

- **WHEN** an authorized caller deletes a `provider` + `model` override for a project
- **THEN** the override is removed and no longer affects cost resolution for that project

#### Scenario: Unauthorized or cross-project access

- **WHEN** a caller is not authorized for the project, or an override does not exist
- **THEN** the endpoint SHALL return an appropriate 401/403/404 error and SHALL NOT modify pricing

### Requirement: Project override takes precedence in cost resolution

The system SHALL consult a project's pricing override before global retail pricing when estimating the cost of a usage event for that project.

#### Scenario: Override wins over global retail

- **WHEN** a project has an override for `model="deepseek-v4-pro"` with an output price differing from the global retail price
- **THEN** the usage event cost SHALL use the override price

#### Scenario: No override falls through to global retail

- **WHEN** a project has no override for a usage event's `provider` + `model`
- **THEN** the system SHALL fall through to global retail pricing resolution
