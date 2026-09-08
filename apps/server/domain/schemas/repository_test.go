package schemas

import (
	"encoding/json"
	"testing"
)

// TestParseObjectTypeSchemas verifies that parseObjectTypeSchemas handles both
// the array format (user-uploaded YAML/JSON files) and the map format (blueprint
// seeds / epf-engine v3 schemas) without silently returning nil.
func TestParseObjectTypeSchemas(t *testing.T) {
	const packID = "pack-1"
	const packName = "test-schema"
	const packVersion = "1.0.0"

	t.Run("array format", func(t *testing.T) {
		data := json.RawMessage(`[
			{"name":"Belief","label":"Belief","description":"A belief","properties":{"text":{"type":"string"}}},
			{"name":"Person","label":"Person","ui":{"icon":"lucide--user","color":"#4F46E5"}}
		]`)

		got := parseObjectTypeSchemas(data, packID, packName, packVersion)
		if got == nil {
			t.Fatal("expected non-nil result for array format")
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 types, got %d", len(got))
		}
		names := map[string]bool{}
		for _, o := range got {
			names[o.Name] = true
			if o.SchemaID != packID {
				t.Errorf("expected SchemaID %q, got %q", packID, o.SchemaID)
			}
			if o.SchemaName != packName {
				t.Errorf("expected SchemaName %q, got %q", packName, o.SchemaName)
			}
			if o.SchemaVersion != packVersion {
				t.Errorf("expected SchemaVersion %q, got %q", packVersion, o.SchemaVersion)
			}
		}
		if !names["Belief"] || !names["Person"] {
			t.Errorf("expected type names Belief and Person, got %v", names)
		}
		// inline ui block is carried through to the compiled type
		for _, o := range got {
			if o.Name == "Person" && string(o.UI) != `{"icon":"lucide--user","color":"#4F46E5"}` {
				t.Errorf("expected ui block on Person, got %s", o.UI)
			}
		}
	})

	t.Run("map format (blueprint/epf-engine v3)", func(t *testing.T) {
		data := json.RawMessage(`{
			"Belief":  {"label":"Belief","description":"A belief","properties":{"text":{"type":"string"}}},
			"Person":  {"label":"Person","description":"A person","ui":{"icon":"lucide--user","color":"#4F46E5"}}
		}`)

		got := parseObjectTypeSchemas(data, packID, packName, packVersion)
		if got == nil {
			t.Fatal("expected non-nil result for map format")
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 types, got %d", len(got))
		}
		names := map[string]bool{}
		for _, o := range got {
			names[o.Name] = true
			if o.SchemaID != packID {
				t.Errorf("expected SchemaID %q, got %q", packID, o.SchemaID)
			}
		}
		if !names["Belief"] || !names["Person"] {
			t.Errorf("expected type names Belief and Person, got %v", names)
		}
		// inline ui block is carried through to the compiled type
		for _, o := range got {
			if o.Name == "Person" && string(o.UI) != `{"icon":"lucide--user","color":"#4F46E5"}` {
				t.Errorf("expected ui block on Person, got %s", o.UI)
			}
		}
	})

	t.Run("nil on empty data", func(t *testing.T) {
		got := parseObjectTypeSchemas(nil, packID, packName, packVersion)
		if got != nil {
			t.Errorf("expected nil for empty data, got %v", got)
		}
	})

	t.Run("nil on invalid json", func(t *testing.T) {
		got := parseObjectTypeSchemas(json.RawMessage(`not-json`), packID, packName, packVersion)
		if got != nil {
			t.Errorf("expected nil for invalid JSON, got %v", got)
		}
	})
}
