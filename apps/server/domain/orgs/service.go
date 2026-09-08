package orgs

import (
	"context"
	"log/slog"
	"strings"

	"go.uber.org/fx"

	"github.com/emergent-company/emergent.memory/pkg/apperror"
	"github.com/emergent-company/emergent.memory/pkg/logger"
)

const (
	// MaxOrgsPerUser is the maximum number of organizations a user can create
	MaxOrgsPerUser = 100
	// MaxOrgNameLength is the maximum length of an organization name
	MaxOrgNameLength = 120
)

// Service handles business logic for organizations
type Service struct {
	repo                *Repository
	log                 *slog.Logger
	toolPoolInvalidator ToolPoolInvalidator
}

// ServiceParams bundles dependencies for NewService.
type ServiceParams struct {
	fx.In

	Repo *Repository
	Log  *slog.Logger

	// Optional cross-domain dependency (nil-safe when the agents feature is off).
	ToolPoolInvalidator ToolPoolInvalidator `optional:"true"`
}

// NewService creates a new organization service
func NewService(p ServiceParams) *Service {
	return &Service{
		repo:                p.Repo,
		log:                 p.Log.With(logger.Scope("orgs.svc")),
		toolPoolInvalidator: p.ToolPoolInvalidator,
	}
}

// List returns all organizations the user is a member of
func (s *Service) List(ctx context.Context, userID string) ([]OrgDTO, error) {
	if userID == "" {
		return []OrgDTO{}, nil
	}
	return s.repo.List(ctx, userID)
}

// GetByID returns an organization by ID
func (s *Service) GetByID(ctx context.Context, id string) (*OrgDTO, error) {
	org, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	dto := org.ToDTO()
	return &dto, nil
}

// Create creates a new organization
func (s *Service) Create(ctx context.Context, name string, userID string) (*OrgDTO, error) {
	// Validate and sanitize name
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperror.ErrBadRequest.WithMessage("Organization name is required")
	}
	if len(name) > MaxOrgNameLength {
		return nil, apperror.ErrBadRequest.WithMessage("Organization name must be at most 120 characters")
	}

	// Check user's organization limit
	if userID != "" {
		count, err := s.repo.CountUserMemberships(ctx, userID)
		if err != nil {
			return nil, err
		}
		if count >= MaxOrgsPerUser {
			return nil, apperror.New(409, "conflict", "Organization limit reached (100). You can create up to 100 organizations.")
		}
	}

	// Create the organization
	org, err := s.repo.Create(ctx, name, userID)
	if err != nil {
		return nil, err
	}

	s.log.Info("organization created",
		slog.String("orgID", org.ID),
		slog.String("name", org.Name),
		slog.String("userID", userID))

	dto := org.ToDTO()
	return &dto, nil
}

// Delete deletes an organization by ID
func (s *Service) Delete(ctx context.Context, id string) error {
	deleted, err := s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return apperror.ErrNotFound.WithMessage("Organization not found")
	}

	s.log.Info("organization deleted", slog.String("orgID", id))
	return nil
}

// ListMembers returns all members of an organization
func (s *Service) ListMembers(ctx context.Context, orgID string) ([]OrgMemberDTO, error) {
	return s.repo.ListMembers(ctx, orgID)
}
