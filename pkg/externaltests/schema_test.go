package externaltests

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/externaltests/runner"
)

// SHA-256 of the supplied credimi-conformance-result.schema.v1.1.json.
const suppliedSchemaSHA256 = "813b9c9a7f4df4d50e53ce44479632b4fe6ead8ce6b0b6231069a880a4bfbf55"

func TestEmbeddedSchemaMatchesSuppliedOriginal(t *testing.T) {
	if got := hexSHA256(resultSchemaJSON); got != suppliedSchemaSHA256 {
		t.Fatalf("embedded schema hash %s drifted from the supplied original", got)
	}
	original, err := os.ReadFile(filepath.Join("..", "..", "integration", "credimi", "source-of-truth", "credimi-conformance-result.schema.v1.1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, resultSchemaJSON) {
		t.Fatal("embedded schema differs from integration/credimi/source-of-truth")
	}
}

func TestGoldenDocumentsAndPackExampleValidate(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "golden", "*.v1.1.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no golden documents: %v", err)
	}
	paths = append(paths, filepath.Join("testdata", "normalized-ingested-but-failed.v1.1.json"))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateDocument(data); err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
	}
}

func mutateExample(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "normalized-ingested-but-failed.v1.1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	change(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func exampleClaim(doc map[string]any) map[string]any {
	return doc["evidence_claim_results"].([]any)[0].(map[string]any)
}

func TestSchemaRejectsInvalidDocuments(t *testing.T) {
	tests := []struct {
		name   string
		change func(map[string]any)
		path   string
	}{
		{"invented flat_test_results", func(d map[string]any) { d["flat_test_results"] = []any{} }, "/"},
		{"extra top-level engine field", func(d map[string]any) { d["engine"] = map[string]any{"id": "pagopa-wct"} }, "/"},
		{"wrong result enum", func(d map[string]any) { d["run"].(map[string]any)["overall_result"] = "great" }, "/run/overall_result"},
		{"unknown target property", func(d map[string]any) { d["target"].(map[string]any)["suite"] = "x" }, "/target"},
		{"missing claim artifact ref", func(d map[string]any) {
			delete(exampleClaim(d)["evidence_artifacts"].([]any)[0].(map[string]any), "ref")
		}, "/evidence_claim_results/0/evidence_artifacts/0"},
		{"malformed timestamp", func(d map[string]any) { d["run"].(map[string]any)["started_at"] = "26/10/2026" }, "/run/started_at"},
		{"malformed sha256", func(d map[string]any) { d["hash_manifest"].([]any)[0].(map[string]any)["sha256"] = "abc" }, "/hash_manifest/0/sha256"},
		{"wrong evidence type", func(d map[string]any) { exampleClaim(d)["evidence_type"] = []any{"vibes"} }, "/evidence_claim_results/0/evidence_type/0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDocument(mutateExample(t, tt.change))
			var schemaErr *SchemaError
			if !errors.As(err, &schemaErr) {
				t.Fatalf("expected a SchemaError, got %v", err)
			}
			found := false
			for _, d := range schemaErr.Diagnostics {
				if d.Path == tt.path {
					found = true
				}
			}
			if !found {
				t.Errorf("no diagnostic at %s: %v", tt.path, err)
			}
		})
	}
}

func TestSchemaDiagnosticsAreBounded(t *testing.T) {
	secret := "eyJhbGciOiJFUzI1NiJ9." + strings.Repeat("c2VjcmV0", 300)
	data := mutateExample(t, func(d map[string]any) {
		d["hash_manifest"].([]any)[0].(map[string]any)["sha256"] = secret
		for i := 0; i < 40; i++ {
			d["hash_manifest"] = append(d["hash_manifest"].([]any), map[string]any{"ref": "r", "sha256": "bad"})
		}
	})
	err := ValidateDocument(data)
	var schemaErr *SchemaError
	if !errors.As(err, &schemaErr) {
		t.Fatalf("expected a SchemaError, got %v", err)
	}
	if len(schemaErr.Diagnostics) > maxDiagnostics+1 {
		t.Errorf("%d diagnostics exceed the bound", len(schemaErr.Diagnostics))
	}
	if strings.Contains(err.Error(), secret) {
		t.Error("diagnostics must not carry a whole field value")
	}
}

func TestValidateDocumentRejectsNonJSON(t *testing.T) {
	var schemaErr *SchemaError
	if err := ValidateDocument([]byte("{not json")); !errors.As(err, &schemaErr) {
		t.Fatalf("expected a SchemaError, got %v", err)
	}
	if err := ValidateNormalized(nil); err == nil {
		t.Fatal("a nil document must be rejected")
	}
}

// The public flow for both engines: exact bytes -> Parse -> Normalize (which
// validates) -> explicit ValidateNormalized.
func TestParseNormalizeValidateFlowForBothEngines(t *testing.T) {
	cases := map[string]string{
		"captured/itwct/inconclusive.result.json": "captured/itwct/inconclusive.artifacts",
		"captured/wct/wct-fail.result.json":       "captured/wct/wct-fail.artifacts",
		"captured/wct/wct-pass.result.json":       "captured/wct/wct-pass.artifacts",
		"captured/itwct/timed_out.result.json":    "",
	}
	for file, artifacts := range cases {
		data, err := os.ReadFile(filepath.Join(runnerTestdata, file))
		if err != nil {
			t.Fatal(err)
		}
		result, err := runner.Parse(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		ctx := syntheticContext(file, data)
		if artifacts != "" {
			ctx = withArtifacts(t, ctx, result, artifacts)
		}
		doc, err := Normalize(result, ctx)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if err := ValidateNormalized(doc); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if doc.Run.OverallResult != result.Summary.Result {
			t.Errorf("%s: run result %s, external %s", file, doc.Run.OverallResult, result.Summary.Result)
		}
	}
}

func TestNormalizeRefusesADocumentTheSchemaRejects(t *testing.T) {
	result, data := parseFixture(t, "itwct-pass.json")
	ctx := syntheticContext("itwct-pass.json", data)
	ctx.StartedAt = "not a timestamp"
	var schemaErr *SchemaError
	if _, err := Normalize(result, ctx); !errors.As(err, &schemaErr) {
		t.Fatalf("expected a SchemaError, got %v", err)
	}
}
