package provider

import (
	"testing"
)

// TestParsePricingEntries verifies the provider mapping in parsePricingEntries,
// including the openai and deepseek additions, and that unknown providers are
// skipped.
func TestParsePricingEntries(t *testing.T) {
	raw := []pricingEntry{
		{Provider: "google", Model: "gemini-2.5-flash", TextInputPrice: 0.15, OutputPrice: 0.60},
		{Provider: "google-vertex", Model: "gemini-2.5-pro", TextInputPrice: 1.25, OutputPrice: 5.00},
		{Provider: "openai", Model: "gpt-4o", TextInputPrice: 2.50, OutputPrice: 10.00},
		{Provider: "deepseek", Model: "deepseek-v4-pro", TextInputPrice: 1.74, OutputPrice: 3.48},
		{Provider: "unknown-provider", Model: "mystery-model", OutputPrice: 1.0},
	}

	entries := parsePricingEntries(raw)
	if len(entries) != 4 {
		t.Fatalf("parsePricingEntries() returned %d entries, want 4 (unknown provider skipped)", len(entries))
	}

	want := []struct {
		provider       ProviderType
		model          string
		textInputPrice float64
		outputPrice    float64
	}{
		{ProviderGoogleAI, "gemini-2.5-flash", 0.15, 0.60},
		{ProviderVertexAI, "gemini-2.5-pro", 1.25, 5.00},
		{ProviderOpenAI, "gpt-4o", 2.50, 10.00},
		{ProviderDeepSeek, "deepseek-v4-pro", 1.74, 3.48},
	}

	for i, w := range want {
		got := entries[i]
		if got.Provider != w.provider {
			t.Errorf("entries[%d].Provider = %q, want %q", i, got.Provider, w.provider)
		}
		if got.Model != w.model {
			t.Errorf("entries[%d].Model = %q, want %q", i, got.Model, w.model)
		}
		if got.TextInputPrice != w.textInputPrice {
			t.Errorf("entries[%d].TextInputPrice = %v, want %v", i, got.TextInputPrice, w.textInputPrice)
		}
		if got.OutputPrice != w.outputPrice {
			t.Errorf("entries[%d].OutputPrice = %v, want %v", i, got.OutputPrice, w.outputPrice)
		}
		if got.LastSynced.IsZero() {
			t.Errorf("entries[%d].LastSynced should be set", i)
		}
	}
}
