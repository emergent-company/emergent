package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveEmbeddings returns an httptest server that replies to POST /embeddings
// with one embedding per input row plus the given usage block. Requests are
// recorded on reqCh (JSON-decoded embedRequest).
func serveEmbeddings(t *testing.T, usageJSON string, reqCh chan embedRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if reqCh != nil {
			reqCh <- req
		}
		rows := make([]map[string]any, len(req.Input))
		for i, text := range req.Input {
			vec := make([]float32, 2)
			vec[0] = float32(len(text))
			vec[1] = float32(i)
			rows[i] = map[string]any{"embedding": vec, "index": i}
		}
		body := map[string]any{"data": rows}
		if usageJSON != "" {
			var usage map[string]any
			if err := json.Unmarshal([]byte(usageJSON), &usage); err != nil {
				t.Errorf("bad usage fixture: %v", err)
			}
			body["usage"] = usage
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(baseURL string) *Client {
	c, err := NewClient(Config{APIKey: "sk-test", BaseURL: baseURL, Model: "gemini-embedding-001", Dimensions: 768})
	if err != nil {
		panic(err)
	}
	return c
}

func TestEmbedQueryWithUsage(t *testing.T) {
	srv := serveEmbeddings(t, `{"prompt_tokens": 11, "total_tokens": 11}`, nil)
	client := newTestClient(srv.URL)

	res, err := client.EmbedQueryWithUsage(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("EmbedQueryWithUsage() error = %v", err)
	}
	if res.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", res.Provider)
	}
	if res.Model != "gemini-embedding-001" {
		t.Errorf("Model = %q, want gemini-embedding-001", res.Model)
	}
	if res.Usage == nil || res.Usage.PromptTokens != 11 {
		t.Errorf("Usage = %+v, want PromptTokens=11", res.Usage)
	}
	if len(res.Embedding) != 2 {
		t.Errorf("len(Embedding) = %d, want 2", len(res.Embedding))
	}
}

func TestEmbedQueryWithUsageMissingUsageBlock(t *testing.T) {
	srv := serveEmbeddings(t, "", nil)
	client := newTestClient(srv.URL)

	res, err := client.EmbedQueryWithUsage(context.Background(), "no usage")
	if err != nil {
		t.Fatalf("EmbedQueryWithUsage() error = %v", err)
	}
	if res.Usage == nil || res.Usage.PromptTokens != 0 {
		t.Errorf("Usage = %+v, want zero tokens when usage block absent", res.Usage)
	}
}

func TestEmbedDocumentsWithUsageSumsAcrossBatches(t *testing.T) {
	// Two inputs each 5 tokens → one batch, 10 total.
	srv := serveEmbeddings(t, `{"prompt_tokens": 5, "total_tokens": 5}`, nil)
	client := newTestClient(srv.URL)

	res, err := client.EmbedDocumentsWithUsage(context.Background(), []string{"aaaaa", "bbbbb"})
	if err != nil {
		t.Fatalf("EmbedDocumentsWithUsage() error = %v", err)
	}
	if res.Provider != "openai" || res.Model != "gemini-embedding-001" {
		t.Errorf("result = (%q, %q), want (openai, gemini-embedding-001)", res.Provider, res.Model)
	}
	if len(res.Embeddings) != 2 {
		t.Fatalf("len(Embeddings) = %d, want 2", len(res.Embeddings))
	}
	// Server reports 5 prompt tokens per request; a single batch of 2 docs.
	if res.Usage == nil || res.Usage.PromptTokens != 5 {
		t.Errorf("Usage = %+v, want PromptTokens=5", res.Usage)
	}
}

func TestEmbedRequestCarriesModelDimensionsAndAuth(t *testing.T) {
	reqCh := make(chan embedRequest, 1)
	srv := serveEmbeddings(t, `{"prompt_tokens": 3, "total_tokens": 3}`, reqCh)
	client := newTestClient(srv.URL)

	if _, err := client.EmbedQuery(context.Background(), "hi"); err != nil {
		t.Fatalf("EmbedQuery() error = %v", err)
	}
	req := <-reqCh
	if req.Model != "gemini-embedding-001" {
		t.Errorf("request model = %q, want gemini-embedding-001", req.Model)
	}
	if req.Dimensions != 768 {
		t.Errorf("request dimensions = %d, want 768", req.Dimensions)
	}
	if !strings.HasPrefix(client.baseURL, "http") {
		t.Errorf("baseURL sanity failed")
	}
}

func TestEmbedDocumentsEmpty(t *testing.T) {
	client := newTestClient("http://unused")
	got, err := client.EmbedDocuments(context.Background(), nil)
	if err != nil || got != nil {
		t.Fatalf("EmbedDocuments(nil) = (%v, %v), want (nil, nil)", got, err)
	}
	res, err := client.EmbedDocumentsWithUsage(context.Background(), nil)
	if err != nil {
		t.Fatalf("EmbedDocumentsWithUsage(nil) error = %v", err)
	}
	if res.Usage != nil {
		t.Errorf("Usage = %+v, want nil for empty batch", res.Usage)
	}
}
