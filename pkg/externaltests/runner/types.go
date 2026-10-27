// Package runner parses PagoPA runner result envelopes (contract 1.0) offline.
//
// It reads bytes the caller already collected. It never fetches artifact URLs,
// request_uri values or any other network resource, and it never decides that
// a system under test conforms: it only validates and types the envelope.
package runner

import (
	"crypto/sha256"
	"encoding/hex"
)

// ContractVersion is the only runner contract version accepted.
const ContractVersion = "1.0"

// MaxResultBytes bounds a runner result read by Parse.
const MaxResultBytes = 16 << 20

// Engine IDs accepted in engine.id.
const (
	EngineITWCT = "pagopa-itwct"
	EngineWCT   = "pagopa-wct"
)

// RunnerResult is a validated runner result envelope.
//
// Raw holds the exact bytes that were parsed, so unmodelled native diagnostics
// stay available and the source can be hashed. Raw is never serialised.
type RunnerResult struct {
	ContractVersion string     `json:"contract_version"`
	RunID           string     `json:"run_id"`
	ProfileID       string     `json:"profile_id"`
	Category        string     `json:"category"`
	Engine          Engine     `json:"engine"`
	Execution       Execution  `json:"execution"`
	Selection       Selection  `json:"selection"`
	Target          Target     `json:"target"`
	Native          Native     `json:"native"`
	Summary         Summary    `json:"summary"`
	Artifacts       []Artifact `json:"artifacts"`
	Limitations     []string   `json:"limitations"`

	Raw []byte `json:"-"`
}

// Engine identifies the external suite and the build that produced the result.
type Engine struct {
	ID                 string `json:"id"`
	UpstreamRepository string `json:"upstream_repository"`
	ForkRepository     string `json:"fork_repository"`
	Version            string `json:"version"`
	Revision           string `json:"revision"`
}

// Execution is the runner lifecycle outcome, distinct from the suite outcome.
type Execution struct {
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Selection lists the scenario or catalogue entry IDs the run executed.
type Selection struct {
	IDs []string `json:"ids"`
}

// Target is the actual system under test, as recorded by the runner.
type Target struct {
	ActorType        string `json:"actor_type"`
	ActorName        string `json:"actor_name"`
	ActorVersion     string `json:"actor_version,omitempty"`
	IssuerIdentifier string `json:"issuer_identifier,omitempty"`
}

// Native carries the engine's own sessions, assertions and scenario outcomes.
type Native struct {
	Sessions          []Session         `json:"sessions"`
	Assertions        []Assertion       `json:"assertions"`
	ScenarioOutcomes  []ScenarioOutcome `json:"scenario_outcomes"`
	NativeSuiteStatus string            `json:"native_suite_status"`
	CoverageMode      string            `json:"coverage_mode"`
}

// Session is one native test session.
type Session struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Assertion is one native assertion occurrence. Requirement IDs may repeat
// and may be unknown to Credimi; both are preserved as they are.
type Assertion struct {
	AssertionID               string   `json:"assertion_id"`
	RequirementID             string   `json:"requirement_id,omitempty"`
	FullTitle                 string   `json:"full_title"`
	AncestorTitles            []string `json:"ancestor_titles"`
	File                      string   `json:"file"`
	Phase                     string   `json:"phase"`
	CredentialConfigurationID string   `json:"credential_configuration_id,omitempty"`
	NativeStatus              string   `json:"native_status"`
	VitestStatus              string   `json:"vitest_status,omitempty"`
	ErrorMessage              string   `json:"error_message,omitempty"`
	// NativeRecord is the engine's original producer record as JSON text.
	NativeRecord string `json:"raw,omitempty"`
}

// ScenarioOutcome is a native scenario verdict (PASS, FAIL or INCONCLUSIVE).
type ScenarioOutcome struct {
	ScenarioID    string `json:"scenario_id"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Verdict       string `json:"verdict"`
}

// Summary is the runner's conservative aggregate, checked against the evidence.
type Summary struct {
	Result                     string `json:"result"`
	AssertionCount             int    `json:"assertion_count"`
	NativeFailedAssertionCount int    `json:"native_failed_assertion_count"`
}

// Artifact is a manifest entry. Its URL and hash are unverified metadata until
// the caller supplies the collected bytes.
type Artifact struct {
	ArtifactID  string `json:"artifact_id"`
	Filename    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"download_url"`
}

// SHA256 returns the lowercase hex SHA-256 of the exact parsed bytes.
func (r *RunnerResult) SHA256() string {
	sum := sha256.Sum256(r.Raw)
	return hex.EncodeToString(sum[:])
}

// Incomplete reports whether execution did not complete (cancelled, timed
// out or error). Such an envelope is valid, incomplete evidence.
func (r *RunnerResult) Incomplete() bool {
	return r.Execution.Status != "completed"
}
