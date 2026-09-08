## Purpose

Expose the retail pricing used to estimate LLM usage cost, so the gateway can display each model's "automatic" rate and let users override it. Retail pricing lives in `kb.provider_pricing` but is not currently reachable via any API.

## ADDED Requirements

### Requirement: List retail pricing via API

The system SHALL expose a read-only, authenticated endpoint returning the global retail pricing per provider and model.

#### Scenario: Pricing listed

- **WHEN** an authenticated caller requests the pricing list
- **THEN** each entry SHALL include the provider, model, and per-modality prices (text/image/video/audio input and output) in USD per 1 million tokens

#### Scenario: No pricing rows

- **WHEN** no pricing rows exist
- **THEN** the endpoint SHALL return an empty list, not an error

### Requirement: Pricing is read-only over the API

The system SHALL NOT allow clients to modify global retail pricing through this API (retail pricing is managed by the internal sync).

#### Scenario: Write attempted

- **WHEN** a client attempts to mutate retail pricing
- **THEN** there is no endpoint to do so (only the read endpoint exists)
