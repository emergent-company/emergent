package provider

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/emergent-company/emergent.memory/domain/scheduler"
	"github.com/emergent-company/emergent.memory/pkg/logger"
)

const (
	// modelCatalogSyncSchedule re-syncs configured OpenAI-compatible provider
	// catalogs every 6 hours. Scheduler uses seconds-precision (6-field) cron:
	// "second minute hour dom month dow".
	modelCatalogSyncSchedule = "0 30 */6 * * *"

	// modelCatalogSyncTimeout caps each individual provider's catalog fetch so
	// one slow proxy cannot stall the whole pass.
	modelCatalogSyncTimeout = 15 * time.Second
)

// ModelCatalogSyncService periodically re-syncs provider_supported_models for
// configured OpenAI-compatible (openai / LiteLLM) providers. SyncModels only
// runs on provider-config upsert today, so a LiteLLM model list that changes
// afterwards leaves the rates panel and default-model dropdown stale until the
// provider is re-saved. A recurring job (plus a startup pass) re-resolves each
// configured credential and refreshes the catalog snapshot.
type ModelCatalogSyncService struct {
	repo    *Repository
	credsvc *CredentialService
	catalog *ModelCatalogService
	sched   *scheduler.Scheduler
	log     *slog.Logger
}

// NewModelCatalogSyncService creates the service and registers the recurring
// catalog re-sync cron job.
func NewModelCatalogSyncService(repo *Repository, credsvc *CredentialService, catalog *ModelCatalogService, sched *scheduler.Scheduler, log *slog.Logger) *ModelCatalogSyncService {
	s := &ModelCatalogSyncService{
		repo:    repo,
		credsvc: credsvc,
		catalog: catalog,
		sched:   sched,
		log:     log.With(logger.Scope("provider.model_catalog_sync")),
	}

	if err := sched.AddCronTask("provider:model_catalog:sync", modelCatalogSyncSchedule, func(ctx context.Context) error {
		return s.Sync(ctx)
	}); err != nil {
		log.Warn("failed to register model catalog sync cron job", logger.Error(err))
	}

	return s
}

// Sync re-resolves every configured OpenAI-compatible provider credential and
// refreshes its model catalog. It never fails the process: per-config errors
// (missing encryption key, unreachable proxy) are logged and skipped, matching
// the non-fatal SyncModels behavior on the upsert path. The catalog sync
// itself falls back to the configured models when the proxy's /v1/models list
// is unreachable, so a stale snapshot is strictly better than an empty one.
func (s *ModelCatalogSyncService) Sync(ctx context.Context) error {
	configs, err := s.repo.ListProjectProviderConfigsByProvider(ctx, ProviderOpenAI)
	if err != nil {
		return fmt.Errorf("model catalog resync: list openai provider configs: %w", err)
	}
	if len(configs) == 0 {
		return nil
	}

	synced := 0
	for i := range configs {
		cfg := configs[i]
		cred, err := s.credsvc.decryptProjectConfig(&cfg)
		if err != nil {
			s.log.Debug("model catalog resync: skipping provider config (credential decryption failed)",
				logger.Error(err),
				slog.String("projectID", cfg.ProjectID),
			)
			continue
		}

		syncCtx, cancel := context.WithTimeout(ctx, modelCatalogSyncTimeout)
		if err := s.catalog.SyncModels(syncCtx, ProviderOpenAI, cred); err != nil {
			s.log.Warn("model catalog resync failed for provider config",
				logger.Error(err),
				slog.String("projectID", cfg.ProjectID),
			)
		} else {
			synced++
		}
		cancel()
	}

	s.log.Info("model catalog resync complete",
		slog.Int("configs", len(configs)),
		slog.Int("synced", synced),
	)
	return nil
}
