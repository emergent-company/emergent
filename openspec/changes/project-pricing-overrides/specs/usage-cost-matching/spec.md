## Purpose

Resolve LLM usage events to a price optimistically — matching on the model name when the exact provider+model lookup misses — so models served through an OpenAI-compatible/LiteLLM proxy still receive an estimated cost instead of $0.

## ADDED Requirements

### Requirement: Optimistic price resolution order

The system SHALL resolve a usage event's price in this order: (1) project override for the exact `provider + model`, (2) global retail pricing for the exact `provider + model`, (3) global retail pricing by model name only, (4) global retail pricing by normalized model name. The first non-nil match SHALL be used.

#### Scenario: Exact provider+model match

- **WHEN** a usage event has `provider="openai"` and `model="gpt-4o"` and global pricing contains that exact pair
- **THEN** the exact-pair price SHALL be used

#### Scenario: Model served through a different provider

- **WHEN** a usage event has `provider="openai"` and `model="deepseek-v4-pro"` with no global `openai` entry for that model, but a global `deepseek` entry for `deepseek-v4-pro` exists
- **THEN** the model-only lookup SHALL match the `deepseek` entry and produce a non-zero cost

#### Scenario: Vendor-prefixed model name

- **WHEN** a usage event has a model name with a vendor prefix such as `deepseek/deepseek-v4-pro` that does not match exactly
- **THEN** the normalized model name (`deepseek-v4-pro`) SHALL be used for the model-only lookup

### Requirement: Zero only when no optimistic match exists

The system SHALL return an estimated cost of $0 only when no price is found after all optimistic resolution steps.

#### Scenario: No price anywhere

- **WHEN** a usage event's model has no project override, no exact global entry, and no model-only entry
- **THEN** the estimated cost SHALL be $0

### Requirement: Per-modality cost computation is preserved

The system SHALL compute cost from the matched price row using the event's per-modality token counts (text/image/video/audio input and output), unchanged from the current `computeCost` behavior.

#### Scenario: Cost uses matched row prices

- **WHEN** a price row is matched with distinct input and output prices
- **THEN** the estimated cost SHALL multiply each modality's token count by that modality's price, divided by 1,000,000
