package chunking

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/emergent-company/emergent.memory/domain/chunks"
	"github.com/emergent-company/emergent.memory/internal/config"
	"github.com/emergent-company/emergent.memory/pkg/apperror"
	"github.com/emergent-company/emergent.memory/pkg/logger"
	"github.com/emergent-company/emergent.memory/pkg/textsplitter"
)

type Service struct {
	db  bun.IDB
	log *slog.Logger

	// defaultChunkSize / defaultChunkOverlap are the env-backed defaults
	// (CHUNK_SIZE / CHUNK_OVERLAP, falling back to textsplitter defaults
	// 1000/200). Per-document chunking_config overrides them per document.
	defaultChunkSize    int
	defaultChunkOverlap int
}

func NewService(db bun.IDB, cfg *config.Config, log *slog.Logger) *Service {
	def := textsplitter.DefaultConfig()
	chunkSize, chunkOverlap := def.ChunkSize, def.ChunkOverlap
	if cfg != nil {
		if cfg.Chunking.ChunkSize > 0 {
			chunkSize = cfg.Chunking.ChunkSize
		}
		if cfg.Chunking.ChunkOverlap >= 0 {
			chunkOverlap = cfg.Chunking.ChunkOverlap
		}
	}
	return &Service{
		db:                  db,
		log:                 log.With(logger.Scope("chunking.svc")),
		defaultChunkSize:    chunkSize,
		defaultChunkOverlap: chunkOverlap,
	}
}

type RecreateChunksResponse struct {
	Status  string                `json:"status"`
	Summary RecreateChunksSummary `json:"summary"`
}

type RecreateChunksSummary struct {
	OldChunks int            `json:"oldChunks"`
	NewChunks int            `json:"newChunks"`
	Strategy  string         `json:"strategy"`
	Config    map[string]any `json:"config,omitempty"`
}

func (s *Service) RecreateChunks(ctx context.Context, projectID, documentID string) (*RecreateChunksResponse, error) {
	if _, err := uuid.Parse(projectID); err != nil {
		return nil, apperror.ErrBadRequest.WithMessage("Invalid project ID format")
	}

	docUUID, err := uuid.Parse(documentID)
	if err != nil {
		return nil, apperror.ErrBadRequest.WithMessage("Invalid document ID format")
	}

	var content sql.NullString
	err = s.db.NewSelect().
		TableExpr("kb.documents").
		Column("content").
		Where("id = ?", documentID).
		Where("project_id = ?", projectID).
		Scan(ctx, &content)

	if err == sql.ErrNoRows {
		return nil, apperror.ErrNotFound.WithMessage("Document not found")
	}
	if err != nil {
		return nil, apperror.ErrInternal.WithInternal(err)
	}

	if !content.Valid || content.String == "" {
		return nil, apperror.ErrBadRequest.WithMessage("Document has no content to chunk")
	}

	var oldCount int
	err = s.db.NewSelect().
		TableExpr("kb.chunks").
		ColumnExpr("COUNT(*)").
		Where("document_id = ?", documentID).
		Scan(ctx, &oldCount)
	if err != nil {
		s.log.Warn("failed to count existing chunks", slog.String("error", err.Error()))
		oldCount = 0
	}

	cfg := textsplitter.Config{
		ChunkSize:    s.defaultChunkSize,
		ChunkOverlap: s.defaultChunkOverlap,
	}
	if projChunkSize, projChunkOverlap := s.loadProjectChunkingConfig(ctx, projectID); projChunkSize != nil || projChunkOverlap != nil {
		if projChunkSize != nil && *projChunkSize > 0 {
			cfg.ChunkSize = *projChunkSize
		}
		if projChunkOverlap != nil && *projChunkOverlap >= 0 {
			cfg.ChunkOverlap = *projChunkOverlap
		}
	}
	textChunks := textsplitter.Split(content.String, cfg)

	if len(textChunks) == 0 {
		return &RecreateChunksResponse{
			Status: "success",
			Summary: RecreateChunksSummary{
				OldChunks: oldCount,
				NewChunks: 0,
				Strategy:  "recursive_character",
				Config: map[string]any{
					"chunkSize":    cfg.ChunkSize,
					"chunkOverlap": cfg.ChunkOverlap,
				},
			},
		}, nil
	}

	now := time.Now().UTC()

	// Build chunk and embedding-job slices upfront so we can batch-insert both
	// inside a single transaction — replacing the previous N individual round-trips.
	chunkRows := make([]*chunks.Chunk, 0, len(textChunks))
	for i, text := range textChunks {
		chunkRows = append(chunkRows, &chunks.Chunk{
			ID:         uuid.New(),
			DocumentID: docUUID,
			ChunkIndex: i,
			Text:       text,
			Metadata: &chunks.ChunkMetadata{
				Strategy: "recursive_character",
			},
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	type embeddingJob struct {
		bun.BaseModel `bun:"table:kb.chunk_embedding_jobs"`
		ID            uuid.UUID `bun:"id,pk,type:uuid"`
		ChunkID       uuid.UUID `bun:"chunk_id,type:uuid,notnull"`
		Status        string    `bun:"status,notnull"`
		Priority      int       `bun:"priority,notnull"`
		CreatedAt     time.Time `bun:"created_at,notnull"`
		UpdatedAt     time.Time `bun:"updated_at,notnull"`
	}

	jobRows := make([]*embeddingJob, 0, len(chunkRows))
	for _, c := range chunkRows {
		jobRows = append(jobRows, &embeddingJob{
			ID:        uuid.New(),
			ChunkID:   c.ID,
			Status:    "pending",
			Priority:  1,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	// Execute the delete, chunk batch-insert, and job batch-insert atomically
	// inside a single transaction. This reduces N+2 round-trips to 3.
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if oldCount > 0 {
			if _, err := tx.NewDelete().
				TableExpr("kb.chunks").
				Where("document_id = ?", documentID).
				Exec(ctx); err != nil {
				return err
			}
		}

		if _, err := tx.NewInsert().Model(&chunkRows).Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewInsert().Model(&jobRows).Exec(ctx); err != nil {
			// Embedding jobs are best-effort: log but don't abort the transaction.
			s.log.Warn("failed to enqueue chunk embedding jobs",
				slog.String("documentId", documentID),
				slog.String("error", err.Error()))
		}

		return nil
	})
	if err != nil {
		return nil, apperror.ErrInternal.WithMessage("Failed to create chunks").WithInternal(err)
	}

	s.log.Info("recreated chunks",
		slog.String("documentId", documentID),
		slog.Int("oldChunks", oldCount),
		slog.Int("newChunks", len(chunkRows)),
		slog.Int("embeddingJobs", len(jobRows)))

	return &RecreateChunksResponse{
		Status: "success",
		Summary: RecreateChunksSummary{
			OldChunks: oldCount,
			NewChunks: len(textChunks),
			Strategy:  "recursive_character",
			Config: map[string]any{
				"chunkSize":    cfg.ChunkSize,
				"chunkOverlap": cfg.ChunkOverlap,
			},
		},
	}, nil
}

// loadProjectChunkingConfig reads the project-level kb.projects.chunking_config
// JSONB and returns the effective chunk overrides. The JSON carries
// maxChunkSize (chunk size) and overlap (chunk overlap); pointers distinguish
// "absent" from an explicit zero overlap. Missing column, missing row, or
// malformed JSON yield nils (callers fall back to env/default).
func (s *Service) loadProjectChunkingConfig(ctx context.Context, projectID string) (chunkSize *int, chunkOverlap *int) {
	var raw []byte
	err := s.db.NewSelect().
		TableExpr("kb.projects").
		Column("chunking_config").
		Where("id = ?", projectID).
		Scan(ctx, &raw)
	if err != nil || len(raw) == 0 {
		return nil, nil
	}
	var projCfg struct {
		MaxChunkSize *int `json:"maxChunkSize"`
		Overlap      *int `json:"overlap"`
	}
	if jsonErr := json.Unmarshal(raw, &projCfg); jsonErr != nil {
		return nil, nil
	}
	return projCfg.MaxChunkSize, projCfg.Overlap
}
