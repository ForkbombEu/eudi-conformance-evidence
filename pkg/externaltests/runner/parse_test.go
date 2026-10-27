package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/build"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Pack fixtures (synthetic) and envelopes captured from the compiled runners
// (local fixture evidence, no live SUT): see testdata/captured/*/manifest.json.
func TestParseFixturesAndCapturedEnvelopes(t *testing.T) {
	tests := []struct {
		file       string
		engine     string
		execution  string
		summary    string
		assertions int
		failed     int
		incomplete bool
	}{
		{"itwct-pass.json", EngineITWCT, "completed", "pass", 1, 0, false},
		{"itwct-assertion-fail.json", EngineITWCT, "completed", "fail", 1, 1, false},
		{"itwct-inconclusive.json", EngineITWCT, "completed", "inconclusive", 1, 1, false},
		{"itwct-cancelled-partial.json", EngineITWCT, "cancelled", "partial", 1, 0, true},
		{"itwct-empty.json", EngineITWCT, "completed", "not_tested", 0, 0, false},
		{"wct-interop-pass.json", EngineWCT, "completed", "pass", 1, 0, false},
		{"wct-interop-fail.json", EngineWCT, "completed", "fail", 1, 1, false},
		{"captured/wct/wct-pass.result.json", EngineWCT, "completed", "pass", 1, 0, false},
		{"captured/wct/wct-fail.result.json", EngineWCT, "completed", "fail", 4, 1, false},
		{"captured/wct/wct-teardown-partial.result.json", EngineWCT, "completed", "partial", 1, 0, false},
		{"captured/wct/wct-collection-error.result.json", EngineWCT, "error", "not_tested", 0, 0, true},
		{"captured/wct/wct-cancelled.result.json", EngineWCT, "cancelled", "partial", 1, 0, true},
		{"captured/wct/wct-timed-out.result.json", EngineWCT, "timed_out", "partial", 1, 0, true},
		{"captured/itwct/inconclusive.result.json", EngineITWCT, "completed", "inconclusive", 11, 9, false},
		{"captured/itwct/cancelled.result.json", EngineITWCT, "cancelled", "not_tested", 0, 0, true},
		{"captured/itwct/timed_out.result.json", EngineITWCT, "timed_out", "not_tested", 0, 0, true},
		{"captured/itwct/port_conflict.result.json", EngineITWCT, "error", "not_tested", 0, 0, true},
		{"captured/itwct/immediate_cancel.result.json", EngineITWCT, "cancelled", "not_tested", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data := readTestdata(t, tt.file)
			result, err := Parse(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if result.Engine.ID != tt.engine || result.Execution.Status != tt.execution || result.Summary.Result != tt.summary {
				t.Errorf("got engine=%s execution=%s summary=%s", result.Engine.ID, result.Execution.Status, result.Summary.Result)
			}
			if len(result.Native.Assertions) != tt.assertions || result.Summary.NativeFailedAssertionCount != tt.failed {
				t.Errorf("got %d assertions, %d failed", len(result.Native.Assertions), result.Summary.NativeFailedAssertionCount)
			}
			if result.Incomplete() != tt.incomplete {
				t.Errorf("Incomplete() = %v, want %v", result.Incomplete(), tt.incomplete)
			}
			if !bytes.Equal(result.Raw, data) {
				t.Error("Raw does not hold the exact bytes read")
			}
			if result.SHA256() != sha256Hex(data) {
				t.Error("SHA256() does not hash the exact bytes read")
			}
		})
	}
}

func TestCapturedArtifactsMatchTheirManifestEntries(t *testing.T) {
	cases := map[string]string{
		"captured/wct/wct-fail.result.json":             "captured/wct/wct-fail.artifacts",
		"captured/itwct/inconclusive.result.json":       "captured/itwct/inconclusive.artifacts",
		"captured/wct/wct-cancelled.result.json":        "captured/wct/wct-cancelled.artifacts",
		"captured/wct/wct-collection-error.result.json": "captured/wct/wct-collection-error.artifacts",
	}
	for resultFile, artifactDir := range cases {
		result, err := Parse(bytes.NewReader(readTestdata(t, resultFile)))
		if err != nil {
			t.Fatalf("%s: %v", resultFile, err)
		}
		if len(result.Artifacts) == 0 {
			t.Fatalf("%s: expected an artifact entry", resultFile)
		}
		for _, entry := range result.Artifacts {
			data := readTestdata(t, filepath.Join(artifactDir, entry.ArtifactID))
			if sha256Hex(data) != entry.SHA256 || int64(len(data)) != entry.SizeBytes {
				t.Errorf("%s: artifact %s does not match its manifest entry", resultFile, entry.ArtifactID)
			}
		}
	}
}

func TestParsePreservesNativeDetail(t *testing.T) {
	result, err := Parse(bytes.NewReader(readTestdata(t, "captured/wct/wct-fail.result.json")))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var duplicates, generic int
	for _, a := range result.Native.Assertions {
		if a.RequirementID == "CI_901" {
			duplicates++
		}
		if a.RequirementID == "" {
			generic++
		}
		if a.NativeRecord == "" || !json.Valid([]byte(a.NativeRecord)) {
			t.Errorf("%s: native record not preserved", a.AssertionID)
		}
	}
	if duplicates != 2 {
		t.Errorf("duplicate requirement ID occurrences = %d, want 2", duplicates)
	}
	if generic != 1 {
		t.Errorf("assertions without a requirement ID = %d, want 1", generic)
	}
	first := result.Native.Assertions[0]
	if first.FullTitle != "Production matrix > Requirement identifiers > [CI_900]: bracketed requirement passes" ||
		!slices.Equal(first.AncestorTitles, []string{"Production matrix", "Requirement identifiers"}) {
		t.Errorf("title/ancestors not preserved: %q %v", first.FullTitle, first.AncestorTitles)
	}

	inconclusive, err := Parse(bytes.NewReader(readTestdata(t, "captured/itwct/inconclusive.result.json")))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	outcomes := inconclusive.Native.ScenarioOutcomes
	if len(outcomes) != 1 || outcomes[0].ScenarioID != "WP_RP_HAPPY" || outcomes[0].Verdict != "INCONCLUSIVE" {
		t.Errorf("scenario outcome not preserved: %+v", outcomes)
	}
	// INCONCLUSIVE stays separate from the native Vitest failure count.
	if inconclusive.Summary.Result != "inconclusive" || inconclusive.Summary.NativeFailedAssertionCount != 9 {
		t.Errorf("summary flattened: %+v", inconclusive.Summary)
	}
}

func TestParsePreservesExactBytesIncludingWhitespace(t *testing.T) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, readTestdata(t, "itwct-pass.json")); err != nil {
		t.Fatal(err)
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact.Bytes(), "", "\t"); err != nil {
		t.Fatal(err)
	}
	indented.WriteString("\n\n")

	a, err := Parse(bytes.NewReader(compact.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse(bytes.NewReader(indented.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Raw, indented.Bytes()) {
		t.Error("whitespace was not preserved")
	}
	if a.SHA256() == b.SHA256() {
		t.Error("different source bytes must hash differently")
	}
	again, err := Parse(bytes.NewReader(indented.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if again.SHA256() != b.SHA256() {
		t.Error("hash is not stable for identical bytes")
	}
}

// mutate decodes a fixture, applies change and re-encodes it.
func mutate(t *testing.T, name string, change func(map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(readTestdata(t, name), &doc); err != nil {
		t.Fatal(err)
	}
	change(doc)
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func field(doc map[string]any, key string) map[string]any {
	return doc[key].(map[string]any)
}

func firstAssertion(doc map[string]any) map[string]any {
	return field(doc, "native")["assertions"].([]any)[0].(map[string]any)
}

func TestParseRejectsInvalidEnvelopes(t *testing.T) {
	pass := "itwct-pass.json"
	tests := []struct {
		name  string
		input func(t *testing.T) []byte
		path  string
	}{
		{"unsupported version", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { d["contract_version"] = "2.0" })
		}, "$.contract_version"},
		{"unsupported engine", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "engine")["id"] = "other-suite" })
		}, "$.engine.id"},
		{"non-terminal execution status", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "execution")["status"] = "running" })
		}, "$.execution.status"},
		{"malformed started_at", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "execution")["started_at"] = "yesterday" })
		}, "$.execution.started_at"},
		{"unknown summary result", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "summary")["result"] = "great" })
		}, "$.summary.result"},
		{"unknown native status", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { firstAssertion(d)["native_status"] = "MAYBE" })
		}, "$.native.assertions[0].native_status"},
		{"unknown scenario verdict", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) {
				field(d, "native")["scenario_outcomes"].([]any)[0].(map[string]any)["verdict"] = "SKIPPED"
			})
		}, "$.native.scenario_outcomes[0].verdict"},
		{"unknown coverage mode", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "native")["coverage_mode"] = "full" })
		}, "$.native.coverage_mode"},
		{"unknown actor type", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "target")["actor_type"] = "robot" })
		}, "$.target.actor_type"},
		{"missing run_id", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { delete(d, "run_id") })
		}, "$.run_id"},
		{"missing sessions", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { delete(field(d, "native"), "sessions") })
		}, "$.native.sessions"},
		{"null assertions", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "native")["assertions"] = nil })
		}, "$.native.assertions"},
		{"missing assertion id", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { delete(firstAssertion(d), "assertion_id") })
		}, "$.native.assertions[0].assertion_id"},
		{"empty selection", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "selection")["ids"] = []any{} })
		}, "$.selection.ids"},
		{"duplicate selection", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "selection")["ids"] = []any{"A", "A"} })
		}, "$.selection.ids[1]"},
		{"inconsistent assertion count", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "summary")["assertion_count"] = 5 })
		}, "$.summary.assertion_count"},
		{"inconsistent failed count", func(t *testing.T) []byte {
			return mutate(t, "itwct-assertion-fail.json", func(d map[string]any) {
				field(d, "summary")["native_failed_assertion_count"] = 0
			})
		}, "$.summary.native_failed_assertion_count"},
		{"pass with a FAIL assertion", func(t *testing.T) []byte {
			return mutate(t, "itwct-assertion-fail.json", func(d map[string]any) { field(d, "summary")["result"] = "pass" })
		}, "$.summary.result"},
		{"pass with zero assertions", func(t *testing.T) []byte {
			return mutate(t, "itwct-empty.json", func(d map[string]any) { field(d, "summary")["result"] = "pass" })
		}, "$.summary.result"},
		{"pass with incomplete execution", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { field(d, "execution")["status"] = "cancelled" })
		}, "$.summary.result"},
		{"pass with an INCONCLUSIVE scenario", func(t *testing.T) []byte {
			return mutate(t, "itwct-inconclusive.json", func(d map[string]any) {
				field(d, "summary")["result"] = "pass"
				firstAssertion(d)["native_status"] = "PASS"
				field(d, "summary")["native_failed_assertion_count"] = 0
			})
		}, "$.summary.result"},
		{"pass with a non-PASSED native suite", func(t *testing.T) []byte {
			return mutate(t, "captured/wct/wct-teardown-partial.result.json", func(d map[string]any) {
				field(d, "summary")["result"] = "pass"
			})
		}, "$.summary.result"},
		{"invalid native record", func(t *testing.T) []byte {
			return mutate(t, "captured/wct/wct-pass.result.json", func(d map[string]any) { firstAssertion(d)["raw"] = "{not json" })
		}, "$.native.assertions[0].raw"},
		{"malformed artifact hash", func(t *testing.T) []byte {
			return mutate(t, "captured/wct/wct-pass.result.json", func(d map[string]any) {
				d["artifacts"].([]any)[0].(map[string]any)["sha256"] = "xyz"
			})
		}, "$.artifacts[0].sha256"},
		{"cross-origin artifact URL", func(t *testing.T) []byte {
			return mutate(t, "captured/wct/wct-pass.result.json", func(d map[string]any) {
				d["artifacts"].([]any)[0].(map[string]any)["download_url"] = "https://evil.example/x"
			})
		}, "$.artifacts[0].download_url"},
		{"unsafe artifact ID", func(t *testing.T) []byte {
			return mutate(t, "captured/wct/wct-pass.result.json", func(d map[string]any) {
				d["artifacts"].([]any)[0].(map[string]any)["artifact_id"] = "../etc/passwd"
			})
		}, "$.artifacts[0].artifact_id"},
		{"unknown top-level field", func(t *testing.T) []byte {
			return mutate(t, pass, func(d map[string]any) { d["flat_test_results"] = []any{} })
		}, "$.flat_test_results"},
		{"trailing JSON", func(t *testing.T) []byte {
			return append(readTestdata(t, pass), []byte(` {"x":1}`)...)
		}, "$"},
		{"truncated input", func(t *testing.T) []byte {
			data := readTestdata(t, pass)
			return data[:len(data)/2]
		}, "$"},
		{"top-level array", func(t *testing.T) []byte { return []byte(`[]`) }, "$"},
		{"top-level null", func(t *testing.T) []byte { return []byte(`null`) }, "$"},
		{"empty input", func(t *testing.T) []byte { return nil }, "$"},
		{"runner API error body", func(t *testing.T) []byte {
			return readTestdata(t, "captured/itwct/api-errors/unsupported_selection.json")
		}, "$.contract_version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(bytes.NewReader(tt.input(t)))
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("expected a ValidationError, got %v", err)
			}
			if validation.Path != tt.path {
				t.Errorf("path = %q, want %q (%v)", validation.Path, tt.path, err)
			}
		})
	}
}

func TestParseAcceptsValidIncompleteEnvelope(t *testing.T) {
	result, err := Parse(bytes.NewReader(readTestdata(t, "captured/itwct/immediate_cancel.result.json")))
	if err != nil {
		t.Fatalf("a structurally valid cancelled envelope is incomplete evidence, not a parse error: %v", err)
	}
	if !result.Incomplete() || len(result.Native.Assertions) != 0 || result.Execution.ExitCode != nil {
		t.Errorf("unexpected incomplete envelope: %+v", result.Execution)
	}
}

func TestParseEnforcesByteLimit(t *testing.T) {
	data := readTestdata(t, "itwct-pass.json")
	padded := append(bytes.Clone(data), bytes.Repeat([]byte(" "), MaxResultBytes+1-len(data))...)
	_, err := Parse(bytes.NewReader(padded))
	var validation *ValidationError
	if !errors.As(err, &validation) || !strings.Contains(validation.Message, "limit") {
		t.Fatalf("expected a byte-limit error, got %v", err)
	}

	atLimit := append(bytes.Clone(data), bytes.Repeat([]byte(" "), MaxResultBytes-len(data))...)
	if _, err := Parse(bytes.NewReader(atLimit)); err != nil {
		t.Fatalf("exactly MaxResultBytes must be accepted: %v", err)
	}
}

type failingReader struct{}

var errMidstream = errors.New("connection dropped")

func (failingReader) Read([]byte) (int, error) { return 0, errMidstream }

func TestParseReportsMidstreamReadFailure(t *testing.T) {
	data := readTestdata(t, "itwct-pass.json")
	reader := io.MultiReader(bytes.NewReader(data[:40]), failingReader{})
	_, err := Parse(reader)
	if !errors.Is(err, errMidstream) {
		t.Fatalf("expected the read error to be wrapped, got %v", err)
	}
}

func TestValidationErrorsAreBounded(t *testing.T) {
	secret := "eyJhbGciOiJFUzI1NiJ9." + strings.Repeat("c2VjcmV0", 200)
	data := mutate(t, "itwct-pass.json", func(d map[string]any) { field(d, "engine")["id"] = secret })
	_, err := Parse(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), secret) || len(err.Error()) > 300 {
		t.Errorf("error is not bounded (%d bytes)", len(err.Error()))
	}
}

func TestPackageHasNoNetworkOrProcessDependencies(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range pkg.Imports {
		if strings.HasPrefix(imp, "net") || imp == "os/exec" || strings.Contains(imp, "database/sql") ||
			strings.HasPrefix(imp, "github.com/forkbombeu/eudi-conformance-evidence/") {
			t.Errorf("forbidden import %s", imp)
		}
	}
}
