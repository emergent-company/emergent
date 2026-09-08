package backups

import "testing"

func TestSelectColumnExpr(t *testing.T) {
	tests := []struct {
		name         string
		col          colInfo
		vectorListed bool
		wantExpr     string
		wantVector   bool
	}{
		{
			name:     "vector by udt",
			col:      colInfo{Name: "embedding", DataType: "USER-DEFINED", UDTName: "vector"},
			wantExpr: `t."embedding"::text AS "embedding"`, wantVector: true,
		},
		{
			name:     "halfvec by udt",
			col:      colInfo{Name: "embedding", DataType: "USER-DEFINED", UDTName: "halfvec"},
			wantExpr: `t."embedding"::text AS "embedding"`, wantVector: true,
		},
		{
			name:         "vector by config list",
			col:          colInfo{Name: "description_embedding", DataType: "USER-DEFINED", UDTName: "vector"},
			vectorListed: true,
			wantExpr:     `t."description_embedding"::text AS "description_embedding"`, wantVector: true,
		},
		{
			name:     "jsonb",
			col:      colInfo{Name: "properties", DataType: "jsonb", UDTName: "jsonb"},
			wantExpr: `t."properties"::text AS "properties"`, wantVector: false,
		},
		{
			name:     "json datatype",
			col:      colInfo{Name: "meta", DataType: "json", UDTName: "json"},
			wantExpr: `t."meta"::text AS "meta"`, wantVector: false,
		},
		{
			name:     "text array",
			col:      colInfo{Name: "labels", DataType: "ARRAY", UDTName: "_text"},
			wantExpr: `t."labels"::text AS "labels"`, wantVector: false,
		},
		{
			name:     "uuid array",
			col:      colInfo{Name: "created_object_ids", DataType: "ARRAY", UDTName: "_uuid"},
			wantExpr: `t."created_object_ids"::text AS "created_object_ids"`, wantVector: false,
		},
		{
			name:     "plain scalar",
			col:      colInfo{Name: "name", DataType: "text", UDTName: "text"},
			wantExpr: `t."name"`, wantVector: false,
		},
		{
			name:     "bytea stays raw",
			col:      colInfo{Name: "data", DataType: "bytea", UDTName: "bytea"},
			wantExpr: `t."data"`, wantVector: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, isVector := selectColumnExpr(tt.col, tt.vectorListed)
			if expr != tt.wantExpr {
				t.Errorf("expr = %q, want %q", expr, tt.wantExpr)
			}
			if isVector != tt.wantVector {
				t.Errorf("isVector = %v, want %v", isVector, tt.wantVector)
			}
		})
	}
}

func TestQuoteIdentAndSplitTable(t *testing.T) {
	if got := quoteIdent("weird\"name"); got != `"weird""name"` {
		t.Errorf("quoteIdent = %q", got)
	}
	schema, table := splitTable("kb.documents")
	if schema != "kb" || table != "documents" {
		t.Errorf("splitTable(kb.documents) = %q, %q", schema, table)
	}
	if schema, table := splitTable("documents"); schema != "kb" || table != "documents" {
		t.Errorf("splitTable(documents) = %q, %q", schema, table)
	}
}
