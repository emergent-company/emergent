package modelconfig

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

// fakeStore implements modelConfigStore in memory.
type fakeStore struct {
	cfg *ProjectModelConfig
	err error
}

func (f *fakeStore) GetProjectModelConfig(_ context.Context, _ uuid.UUID) (*ProjectModelConfig, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cfg, nil
}

func (f *fakeStore) UpsertProjectModelConfig(_ context.Context, cfg *ProjectModelConfig) error {
	f.cfg = cfg
	return nil
}

func (f *fakeStore) DeleteProjectModelConfig(_ context.Context, _ uuid.UUID) error {
	f.cfg = nil
	return nil
}

// fakeResolver implements generativeDefaultResolver.
type fakeResolver struct {
	model string
	err   error
}

func (f *fakeResolver) DefaultGenerativeModel(_ context.Context, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.model, nil
}

func testService(store modelConfigStore, resolver generativeDefaultResolver) *Service {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(store, log).WithGenerativeDefaultResolver(resolver)
}

func TestResolveGenerativeModelProjectConfigWins(t *testing.T) {
	projectID := uuid.New()
	svc := testService(&fakeStore{cfg: &ProjectModelConfig{
		ProjectID:       projectID,
		GenerativeModel: "deepseek/deepseek-v4-flash",
	}}, &fakeResolver{model: "google/gemini-2.5-flash"})

	model, source, err := svc.ResolveGenerativeModel(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "deepseek/deepseek-v4-flash" || source != ModelSourceProject {
		t.Errorf("ResolveGenerativeModel = (%q, %q), want project config (deepseek/deepseek-v4-flash, project)", model, source)
	}
}

func TestResolveGenerativeModelFallsBackToProviderCredential(t *testing.T) {
	projectID := uuid.New()
	svc := testService(&fakeStore{cfg: nil}, &fakeResolver{model: "deepseek/deepseek-v4-flash"})

	model, source, err := svc.ResolveGenerativeModel(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "deepseek/deepseek-v4-flash" || source != ModelSourceProvider {
		t.Errorf("ResolveGenerativeModel = (%q, %q), want provider fallback (deepseek/deepseek-v4-flash, provider)", model, source)
	}
}

func TestResolveGenerativeModelNoneWhenNothingConfigured(t *testing.T) {
	projectID := uuid.New()
	svc := testService(&fakeStore{cfg: nil}, &fakeResolver{model: ""})

	model, source, err := svc.ResolveGenerativeModel(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "" || source != ModelSourceNone {
		t.Errorf("ResolveGenerativeModel = (%q, %q), want (\"\", none)", model, source)
	}
}

func TestResolveGenerativeModelNilResolverStaysProjectOnly(t *testing.T) {
	projectID := uuid.New()
	svc := NewService(&fakeStore{cfg: nil}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	model, source, err := svc.ResolveGenerativeModel(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "" || source != ModelSourceNone {
		t.Errorf("without resolver ResolveGenerativeModel = (%q, %q), want (\"\", none)", model, source)
	}
}

func TestResolveGenerativeModelPropagatesStoreError(t *testing.T) {
	projectID := uuid.New()
	want := errors.New("db down")
	svc := testService(&fakeStore{err: want}, &fakeResolver{model: "google/gemini-2.5-flash"})

	if _, _, err := svc.ResolveGenerativeModel(context.Background(), projectID); err == nil {
		t.Fatal("expected store error to propagate")
	}
}

func TestResolveGenerativeModelIgnoresResolverError(t *testing.T) {
	projectID := uuid.New()
	// The executor ignores resolver failures on its fallback; the reporting
	// path must not fail the request because a provider lookup errored.
	svc := testService(&fakeStore{cfg: nil}, &fakeResolver{err: errors.New("boom")})

	model, source, err := svc.ResolveGenerativeModel(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "" || source != ModelSourceNone {
		t.Errorf("on resolver error ResolveGenerativeModel = (%q, %q), want (\"\", none)", model, source)
	}
}
