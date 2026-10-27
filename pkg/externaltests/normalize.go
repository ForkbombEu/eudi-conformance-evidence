package externaltests

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/externaltests/runner"
)

const jsonMimeType = "application/json"

var sha256Hex = regexp.MustCompile(`^[a-f0-9]{64}$`)

// StoredRef is a reference to bytes the caller already stored, with the
// SHA-256 the caller computed from those exact bytes.
type StoredRef struct {
	Ref      string
	SHA256   string
	MimeType string
}

// StoredArtifact is a runner artifact the caller collected and stored.
type StoredArtifact struct {
	// ArtifactID is the artifact_id from the runner result manifest.
	ArtifactID string
	StoredRef
}

// NormalizationContext is the caller's real provenance for one import.
// Normalize never invents any of it.
type NormalizationContext struct {
	WorkflowID      string
	WorkflowRunID   string
	PipelineID      string
	PipelineVersion string
	PipelineName    string
	// StartedAt/FinishedAt override the run timestamps; by default the
	// runner's recorded execution timestamps are used.
	StartedAt  string
	FinishedAt string
	// RawResult references the stored runner result bytes parsed by
	// runner.Parse; its SHA256 must match those bytes.
	RawResult StoredRef
	// Artifacts are already collected runner artifacts (optional).
	Artifacts []StoredArtifact
	// Limitations are caller statements added to the document.
	Limitations []string
}

// ContextError reports missing or inconsistent caller provenance.
type ContextError struct {
	Field   string
	Message string
}

func (e *ContextError) Error() string {
	return "externaltests: " + e.Field + ": " + e.Message
}

// Normalize builds the v1.1 document for a parsed runner result and validates
// it against the embedded schema before returning it. It performs no I/O. A successful import sets the ingestion claim to pass; the external
// suite outcome is reported separately in run.overall_result and
// overall_result and is never upgraded.
func Normalize(result *runner.RunnerResult, ctx NormalizationContext) (*ConformanceResult, error) {
	if result == nil || len(result.Raw) == 0 {
		return nil, &ContextError{Field: "result", Message: "a parsed runner result is required"}
	}
	if err := checkContext(result, ctx); err != nil {
		return nil, err
	}

	refs, artifacts, hashes := evidenceRefs(result, ctx)
	external := result.Summary.Result
	doc := &ConformanceResult{
		SchemaVersion:   SchemaVersion,
		RegistryVersion: RegistryVersion,
		ProfileSlug:     ProfileSlug,
		ProfileTitle:    ProfileTitle,
		OverallResult:   external,
		Run: Run{
			WorkflowID:      ctx.WorkflowID,
			WorkflowRunID:   ctx.WorkflowRunID,
			PipelineID:      ctx.PipelineID,
			PipelineVersion: ctx.PipelineVersion,
			PipelineName:    ctx.PipelineName,
			StartedAt:       firstNonEmpty(ctx.StartedAt, result.Execution.StartedAt),
			FinishedAt:      firstNonEmpty(ctx.FinishedAt, result.Execution.FinishedAt),
			OverallResult:   external,
		},
		Target: Target{
			ActorType:            result.Target.ActorType,
			ActorName:            result.Target.ActorName,
			ActorVersion:         result.Target.ActorVersion,
			IssuerIdentifier:     result.Target.IssuerIdentifier,
			ExternalSuiteName:    result.Engine.ID,
			ExternalSuiteVersion: result.Engine.Version,
		},
		Environment: Environment{Metadata: EngineMetadata{
			ExternalEngineID:       result.Engine.ID,
			ExternalEngineVersion:  result.Engine.Version,
			ExternalEngineRevision: result.Engine.Revision,
			UpstreamRepository:     result.Engine.UpstreamRepository,
			ForkRepository:         result.Engine.ForkRepository,
			ExternalRunID:          result.RunID,
			NativeSuiteStatus:      result.Native.NativeSuiteStatus,
			CoverageMode:           result.Native.CoverageMode,
		}},
		EvidenceClaimResults: []EvidenceClaimResult{{
			Slug:              ClaimSlug,
			Title:             ClaimTitle,
			InternalID:        ClaimInternalID,
			Result:            "pass",
			EvidenceType:      []string{EvidenceTypeThird},
			EvidenceArtifacts: artifacts,
			ObservedOutputs:   observe(result),
			Limitations:       claimLimitations(result),
		}},
		RawEvidenceRefs:    RawEvidenceRefs{ExternalTestOutputRefs: refs},
		HashManifest:       hashes,
		Limitations:        append([]string(nil), ctx.Limitations...),
		SourceScopeVersion: SourceScopeVersion,
	}
	// Never return a purported v1.1 document the schema rejects.
	if err := ValidateNormalized(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func checkContext(result *runner.RunnerResult, ctx NormalizationContext) error {
	required := []struct{ field, value string }{
		{"workflow_id", ctx.WorkflowID},
		{"workflow_run_id", ctx.WorkflowRunID},
		{"raw_result.ref", ctx.RawResult.Ref},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return &ContextError{Field: r.field, Message: "is required"}
		}
	}
	if ctx.RawResult.SHA256 != result.SHA256() {
		return &ContextError{Field: "raw_result.sha256", Message: "does not match the parsed runner result bytes"}
	}

	manifest := make(map[string]runner.Artifact, len(result.Artifacts))
	for _, a := range result.Artifacts {
		manifest[a.ArtifactID] = a
	}
	refs := map[string]bool{ctx.RawResult.Ref: true}
	for i, stored := range ctx.Artifacts {
		field := fmt.Sprintf("artifacts[%d]", i)
		entry, ok := manifest[stored.ArtifactID]
		if !ok {
			return &ContextError{Field: field + ".artifact_id", Message: "is not in the runner result manifest"}
		}
		if strings.TrimSpace(stored.Ref) == "" {
			return &ContextError{Field: field + ".ref", Message: "is required"}
		}
		if refs[stored.Ref] {
			return &ContextError{Field: field + ".ref", Message: "is used more than once"}
		}
		refs[stored.Ref] = true
		if !sha256Hex.MatchString(stored.SHA256) || stored.SHA256 != entry.SHA256 {
			return &ContextError{Field: field + ".sha256", Message: "does not match the runner manifest hash"}
		}
	}
	return nil
}

// evidenceRefs lists the stored raw result first, then collected artifacts
// in the caller's order.
func evidenceRefs(result *runner.RunnerResult, ctx NormalizationContext) ([]string, []EvidenceArtifact, []HashEntry) {
	type stored struct {
		ref, sha, mime, description string
	}
	all := []stored{{
		ref:         ctx.RawResult.Ref,
		sha:         ctx.RawResult.SHA256,
		mime:        firstNonEmpty(ctx.RawResult.MimeType, jsonMimeType),
		description: "Runner result envelope (contract " + result.ContractVersion + ")",
	}}
	for _, a := range ctx.Artifacts {
		mime := a.MimeType
		for _, entry := range result.Artifacts {
			if entry.ArtifactID == a.ArtifactID && mime == "" {
				mime = entry.MimeType
			}
		}
		all = append(all, stored{
			ref:         a.Ref,
			sha:         a.SHA256,
			mime:        mime,
			description: "Native runner artifact " + a.ArtifactID,
		})
	}

	refs := make([]string, 0, len(all))
	artifacts := make([]EvidenceArtifact, 0, len(all))
	hashes := make([]HashEntry, 0, len(all))
	for _, s := range all {
		refs = append(refs, s.ref)
		artifacts = append(artifacts, EvidenceArtifact{
			Type: ArtifactTypeResult, Ref: s.ref, SHA256: s.sha, MimeType: s.mime, Description: s.description,
		})
		hashes = append(hashes, HashEntry{
			Ref: s.ref, SHA256: s.sha, Algorithm: "sha256", MimeType: s.mime, Description: s.description,
		})
	}
	return refs, artifacts, hashes
}

func observe(result *runner.RunnerResult) *ObservedOutputs {
	out := &ObservedOutputs{
		ExternalSuite:              result.Engine.ID,
		ExternalRunID:              result.RunID,
		ProfileID:                  result.ProfileID,
		Category:                   result.Category,
		SelectionIDs:               append([]string{}, result.Selection.IDs...),
		ExecutionStatus:            result.Execution.Status,
		ExecutionError:             result.Execution.Error,
		ExitCode:                   result.Execution.ExitCode,
		ExternalResult:             result.Summary.Result,
		NativeSuiteStatus:          result.Native.NativeSuiteStatus,
		CoverageMode:               result.Native.CoverageMode,
		AssertionCount:             result.Summary.AssertionCount,
		NativeFailedAssertionCount: result.Summary.NativeFailedAssertionCount,
		MappingStatus:              MappingUnmapped,
		UnmappedRequirementIDs:     []string{},
		Sessions:                   []ObservedSession{},
		ScenarioOutcomes:           []ObservedScenario{},
		Assertions:                 []ObservedAssertion{},
		RunnerLimitations:          append([]string{}, result.Limitations...),
	}
	for _, s := range result.Native.Sessions {
		out.Sessions = append(out.Sessions, ObservedSession{ID: s.ID, Status: s.Status})
	}
	for _, s := range result.Native.ScenarioOutcomes {
		out.ScenarioOutcomes = append(out.ScenarioOutcomes, ObservedScenario{
			ScenarioID: s.ScenarioID, CorrelationID: s.CorrelationID, Verdict: s.Verdict,
		})
	}
	seen := map[string]bool{}
	for _, a := range result.Native.Assertions {
		switch a.NativeStatus {
		case "PASS":
			out.NativeStatusCounts.Pass++
		case "FAIL":
			out.NativeStatusCounts.Fail++
		case "NOT_REACHED":
			out.NativeStatusCounts.NotReached++
		}
		if a.RequirementID != "" && !seen[a.RequirementID] {
			seen[a.RequirementID] = true
			out.UnmappedRequirementIDs = append(out.UnmappedRequirementIDs, a.RequirementID)
		}
		out.Assertions = append(out.Assertions, ObservedAssertion{
			AssertionID:               a.AssertionID,
			RequirementID:             a.RequirementID,
			FullTitle:                 a.FullTitle,
			AncestorTitles:            append([]string{}, a.AncestorTitles...),
			File:                      a.File,
			Phase:                     a.Phase,
			CredentialConfigurationID: a.CredentialConfigurationID,
			NativeStatus:              a.NativeStatus,
			VitestStatus:              a.VitestStatus,
			ErrorMessage:              a.ErrorMessage,
		})
	}
	return out
}

var resultPhrases = map[string]string{
	"pass":         "passed",
	"fail":         "failed",
	"inconclusive": "was inconclusive",
	"partial":      "was partial",
	"not_tested":   "did not test anything",
}

func claimLimitations(result *runner.RunnerResult) []string {
	limitations := []string{
		"Ingestion succeeded. The external test result " + resultPhrases[result.Summary.Result] +
			". No normative assertion mapping was applied.",
		"PagoPA " + result.Engine.ID + " results are recorded under the existing third-party ingestion profile; " +
			"this is not an OpenID Foundation certification or an official IPZS validation.",
	}
	if result.Native.CoverageMode == "interoperability" {
		limitations = append(limitations,
			"Interoperability coverage: no Italian IT-Wallet assertion matrix was applied.")
	}
	if result.Incomplete() {
		limitations = append(limitations,
			"Execution ended as "+result.Execution.Status+": the external evidence is incomplete.")
	}
	if result.Summary.Result == "inconclusive" && result.Summary.NativeFailedAssertionCount > 0 {
		limitations = append(limitations, fmt.Sprintf(
			"The external result is inconclusive; %d native FAIL assertions are retained as reported and are not counted as an independent conformance failure.",
			result.Summary.NativeFailedAssertionCount))
	}
	return limitations
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
