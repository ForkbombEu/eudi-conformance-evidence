package externaltests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/externaltests/runner"
)

var update = flag.Bool("update", false, "rewrite golden normalized documents")

// Runner envelopes come from the parser package's testdata (pack fixtures
// and envelopes captured from the compiled runners; no live SUT).
const runnerTestdata = "runner/testdata"

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func parseFixture(t *testing.T, name string) (*runner.RunnerResult, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runnerTestdata, name))
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return result, data
}

// syntheticContext is explicitly synthetic caller provenance for unit tests.
func syntheticContext(name string, data []byte) NormalizationContext {
	return NormalizationContext{
		WorkflowID:    "synthetic-workflow",
		WorkflowRunID: "synthetic-temporal-run",
		RawResult: StoredRef{
			Ref:    "artifact:synthetic/" + filepath.Base(name),
			SHA256: hexSHA256(data),
		},
	}
}

// withArtifacts adds the collected runner artifacts stored next to a capture.
func withArtifacts(t *testing.T, ctx NormalizationContext, result *runner.RunnerResult, dir string) NormalizationContext {
	t.Helper()
	for _, entry := range result.Artifacts {
		data, err := os.ReadFile(filepath.Join(runnerTestdata, dir, entry.ArtifactID))
		if err != nil {
			t.Fatal(err)
		}
		ctx.Artifacts = append(ctx.Artifacts, StoredArtifact{
			ArtifactID: entry.ArtifactID,
			StoredRef:  StoredRef{Ref: "artifact:synthetic/" + entry.ArtifactID, SHA256: hexSHA256(data)},
		})
	}
	return ctx
}

func marshal(t *testing.T, doc *ConformanceResult) []byte {
	t.Helper()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

type goldenCase struct {
	name      string
	fixture   string
	artifacts string
	external  string
}

var goldenCases = []goldenCase{
	{"pass", "itwct-pass.json", "", "pass"},
	{"fail", "itwct-assertion-fail.json", "", "fail"},
	{"inconclusive-with-native-failures", "captured/itwct/inconclusive.result.json", "captured/itwct/inconclusive.artifacts", "inconclusive"},
	{"partial-cancelled", "captured/wct/wct-cancelled.result.json", "captured/wct/wct-cancelled.artifacts", "partial"},
	{"zero-tests", "itwct-empty.json", "", "not_tested"},
	{"wct-fail", "captured/wct/wct-fail.result.json", "captured/wct/wct-fail.artifacts", "fail"},
}

func normalizeCase(t *testing.T, c goldenCase) (*ConformanceResult, *runner.RunnerResult, []byte) {
	t.Helper()
	result, data := parseFixture(t, c.fixture)
	ctx := syntheticContext(c.fixture, data)
	if c.artifacts != "" {
		ctx = withArtifacts(t, ctx, result, c.artifacts)
	}
	doc, err := Normalize(result, ctx)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	return doc, result, data
}

func TestNormalizeGolden(t *testing.T) {
	for _, c := range goldenCases {
		t.Run(c.name, func(t *testing.T) {
			doc, result, data := normalizeCase(t, c)
			got := marshal(t, doc)
			path := filepath.Join("testdata", "golden", c.name+".v1.1.json")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("normalized document differs from %s", path)
			}

			claim := doc.EvidenceClaimResults[0]
			if claim.Result != "pass" || doc.Run.OverallResult != c.external || doc.OverallResult != c.external {
				t.Errorf("ingestion claim %s, run %s, overall %s; want pass/%s/%s",
					claim.Result, doc.Run.OverallResult, doc.OverallResult, c.external, c.external)
			}
			if doc.Target.ActorName != result.Target.ActorName || doc.Target.ActorType != result.Target.ActorType {
				t.Errorf("target actor not retained: %+v", doc.Target)
			}
			if doc.Target.ExternalSuiteName != result.Engine.ID || doc.Environment.Metadata.ExternalEngineRevision != result.Engine.Revision ||
				doc.Environment.Metadata.ForkRepository != result.Engine.ForkRepository {
				t.Errorf("engine provenance missing: %+v", doc.Environment.Metadata)
			}
			if doc.HashManifest[0].SHA256 != hexSHA256(data) || doc.HashManifest[0].SHA256 != result.SHA256() {
				t.Error("raw source hash does not bind the exact parsed bytes")
			}
			if len(doc.EvidenceClaimResults) != 1 {
				t.Errorf("unknown requirement IDs must not generate claims; got %d claims", len(doc.EvidenceClaimResults))
			}
			if strings.Contains(string(got), "official_references") || strings.Contains(string(got), "flat_test_results") {
				t.Error("no normative references or flat_test_results may appear")
			}
			if len(claim.ObservedOutputs.Assertions) != len(result.Native.Assertions) {
				t.Error("native assertions not all preserved")
			}
		})
	}
}

// The pack's ingested-but-failed example was hand-written from
// itwct-assertion-fail.json. Every field it states must match the faithful
// import, except its evidence refs: it binds a synthetic native file that is
// not in that fixture's runner manifest.
func TestNormalizeMatchesIngestedButFailedExample(t *testing.T) {
	example, err := os.ReadFile(filepath.Join("testdata", "normalized-ingested-but-failed.v1.1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(example, &want); err != nil {
		t.Fatal(err)
	}
	// The example's hash binds the synthetic native export, not the envelope.
	synthetic, err := os.ReadFile(filepath.Join("testdata", "native-results.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	exampleHash := want["hash_manifest"].([]any)[0].(map[string]any)["sha256"]
	if exampleHash != hexSHA256(synthetic) {
		t.Fatalf("example hash %v does not bind native-results.synthetic.json", exampleHash)
	}
	delete(want, "raw_evidence_refs")
	delete(want, "hash_manifest")
	claim := want["evidence_claim_results"].([]any)[0].(map[string]any)
	delete(claim, "evidence_artifacts")

	result, data := parseFixture(t, "itwct-assertion-fail.json")
	ctx := syntheticContext("itwct-assertion-fail.json", data)
	ctx.Limitations = []string{"Synthetic example only. This document is not test evidence or certification."}
	doc, err := Normalize(result, ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(marshal(t, doc), &got); err != nil {
		t.Fatal(err)
	}
	if path, ok := subset(want, got, "$"); !ok {
		t.Errorf("normalized document does not contain the example's value at %s", path)
	}
	if doc.EvidenceClaimResults[0].Result != "pass" || doc.Run.OverallResult != "fail" {
		t.Error("ingested-but-failed must be ingestion claim pass plus run fail")
	}
	refs := doc.RawEvidenceRefs.ExternalTestOutputRefs
	if len(refs) != 1 || refs[0] != doc.HashManifest[0].Ref || doc.EvidenceClaimResults[0].EvidenceArtifacts[0].Ref != refs[0] {
		t.Errorf("raw ref is not bound consistently: %v", refs)
	}
}

// subset reports whether every value in want appears in got. Array elements
// of want must each match some element of got.
func subset(want, got any, path string) (string, bool) {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return path, false
		}
		for key, value := range w {
			if p, ok := subset(value, g[key], path+"."+key); !ok {
				return p, false
			}
		}
		return "", true
	case []any:
		g, ok := got.([]any)
		if !ok {
			return path, false
		}
		for i, item := range w {
			found := false
			for _, candidate := range g {
				if _, ok := subset(item, candidate, path); ok {
					found = true
					break
				}
			}
			if !found {
				return path + "[" + strconv.Itoa(i) + "]", false
			}
		}
		return "", true
	default:
		return path, reflect.DeepEqual(want, got)
	}
}

func TestNormalizePreservesDuplicateAndUnknownRequirementIDs(t *testing.T) {
	doc, _, _ := normalizeCase(t, goldenCases[5])
	observed := doc.EvidenceClaimResults[0].ObservedOutputs
	duplicates := 0
	for _, a := range observed.Assertions {
		if a.RequirementID == "CI_901" {
			duplicates++
		}
	}
	if duplicates != 2 {
		t.Errorf("duplicate requirement IDs = %d, want 2", duplicates)
	}
	if observed.MappingStatus != MappingUnmapped || !reflect.DeepEqual(observed.UnmappedRequirementIDs, []string{"CI_900", "CI_901"}) {
		t.Errorf("unmapped IDs = %v (%s)", observed.UnmappedRequirementIDs, observed.MappingStatus)
	}
	limitations := strings.Join(doc.EvidenceClaimResults[0].Limitations, " ")
	if !strings.Contains(limitations, "no Italian IT-Wallet assertion matrix") {
		t.Error("interoperability coverage must be stated as a limitation")
	}
}

func TestNormalizeKeepsInconclusiveSeparateFromNativeFailures(t *testing.T) {
	doc, _, _ := normalizeCase(t, goldenCases[2])
	observed := doc.EvidenceClaimResults[0].ObservedOutputs
	if doc.Run.OverallResult != "inconclusive" || observed.NativeFailedAssertionCount != 9 || observed.NativeStatusCounts.Fail != 9 {
		t.Errorf("run %s with %d native failures", doc.Run.OverallResult, observed.NativeFailedAssertionCount)
	}
	if len(observed.ScenarioOutcomes) != 1 || observed.ScenarioOutcomes[0].Verdict != "INCONCLUSIVE" {
		t.Errorf("scenario outcome not preserved: %+v", observed.ScenarioOutcomes)
	}
}

func TestNormalizeIsDeterministic(t *testing.T) {
	for _, c := range goldenCases {
		a, _, _ := normalizeCase(t, c)
		b, _, _ := normalizeCase(t, c)
		if !bytes.Equal(marshal(t, a), marshal(t, b)) {
			t.Errorf("%s: output differs for identical input and context", c.name)
		}
	}
	for _, file := range []string{"normalize.go", "normalized.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(source, []byte("time.Now")) {
			t.Errorf("%s must not read the clock", file)
		}
	}
}

func TestNormalizeRequiresCallerProvenance(t *testing.T) {
	result, data := parseFixture(t, "captured/wct/wct-fail.result.json")
	base := withArtifacts(t, syntheticContext("wct-fail", data), result, "captured/wct/wct-fail.artifacts")
	tests := []struct {
		name   string
		change func(*NormalizationContext)
		field  string
	}{
		{"missing workflow_id", func(c *NormalizationContext) { c.WorkflowID = "" }, "workflow_id"},
		{"missing workflow_run_id", func(c *NormalizationContext) { c.WorkflowRunID = " " }, "workflow_run_id"},
		{"missing raw ref", func(c *NormalizationContext) { c.RawResult.Ref = "" }, "raw_result.ref"},
		{"raw hash mismatch", func(c *NormalizationContext) { c.RawResult.SHA256 = strings.Repeat("0", 64) }, "raw_result.sha256"},
		{"unknown artifact", func(c *NormalizationContext) { c.Artifacts[0].ArtifactID = "other.json" }, "artifacts[0].artifact_id"},
		{"artifact hash mismatch", func(c *NormalizationContext) { c.Artifacts[0].SHA256 = strings.Repeat("a", 64) }, "artifacts[0].sha256"},
		{"artifact ref reused", func(c *NormalizationContext) { c.Artifacts[0].Ref = c.RawResult.Ref }, "artifacts[0].ref"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := base
			ctx.Artifacts = append([]StoredArtifact(nil), base.Artifacts...)
			tt.change(&ctx)
			_, err := Normalize(result, ctx)
			var contextErr *ContextError
			if !errors.As(err, &contextErr) || contextErr.Field != tt.field {
				t.Fatalf("expected ContextError on %s, got %v", tt.field, err)
			}
		})
	}
	if _, err := Normalize(nil, base); err == nil {
		t.Error("a nil result must be rejected")
	}
}
