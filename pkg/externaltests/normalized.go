// Package externaltests normalizes imported external conformance results into
// the Credimi conformance result document (schema v1.1), offline.
package externaltests

// Credimi v1.1 identifiers used for imported external results. The profile and
// claim already exist in the v1.1 evidence registry.
const (
	SchemaVersion      = "1.1"
	RegistryVersion    = "1.1"
	SourceScopeVersion = "1.1"
	ProfileSlug        = "third-party-conformance-ingestion"
	ProfileTitle       = "PagoPA external test result ingestion"
	ClaimSlug          = "external-conformance-test-result-ingested"
	ClaimTitle         = "External conformance test result is ingested and mapped"
	ClaimInternalID    = "credimi.evidence.external.conformance_result_ingested_and_mapped.v1"
	EvidenceTypeThird  = "third_party_result"
	ArtifactTypeResult = "external_test_result"
	MappingUnmapped    = "unmapped"
)

// ConformanceResult is the subset of the v1.1 document an import produces.
// Field names and nesting follow credimi-conformance-result.schema.v1.1.json.
type ConformanceResult struct {
	SchemaVersion        string                `json:"schema_version"`
	RegistryVersion      string                `json:"registry_version"`
	ProfileSlug          string                `json:"profile_slug"`
	ProfileTitle         string                `json:"profile_title"`
	OverallResult        string                `json:"overall_result"`
	Run                  Run                   `json:"run"`
	Target               Target                `json:"target"`
	Environment          Environment           `json:"environment"`
	EvidenceClaimResults []EvidenceClaimResult `json:"evidence_claim_results"`
	RawEvidenceRefs      RawEvidenceRefs       `json:"raw_evidence_refs"`
	HashManifest         []HashEntry           `json:"hash_manifest"`
	Limitations          []string              `json:"limitations,omitempty"`
	SourceScopeVersion   string                `json:"source_scope_version"`
}

// Run identifies the Credimi workflow run that imported the result.
type Run struct {
	WorkflowID      string `json:"workflow_id"`
	WorkflowRunID   string `json:"workflow_run_id"`
	PipelineID      string `json:"pipeline_id,omitempty"`
	PipelineVersion string `json:"pipeline_version,omitempty"`
	PipelineName    string `json:"pipeline_name,omitempty"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
	OverallResult   string `json:"overall_result"`
}

// Target is the actual system under test; the external suite is identified
// separately by ExternalSuiteName/Version.
type Target struct {
	ActorType            string `json:"actor_type"`
	ActorName            string `json:"actor_name"`
	ActorVersion         string `json:"actor_version,omitempty"`
	IssuerIdentifier     string `json:"issuer_identifier,omitempty"`
	ExternalSuiteName    string `json:"external_suite_name"`
	ExternalSuiteVersion string `json:"external_suite_version"`
}

// Environment carries engine provenance in metadata (an open object in v1.1).
type Environment struct {
	Metadata EngineMetadata `json:"metadata"`
}

// EngineMetadata records which external engine build produced the result.
type EngineMetadata struct {
	ExternalEngineID       string `json:"external_engine_id"`
	ExternalEngineVersion  string `json:"external_engine_version"`
	ExternalEngineRevision string `json:"external_engine_revision"`
	UpstreamRepository     string `json:"upstream_repository"`
	ForkRepository         string `json:"fork_repository"`
	ExternalRunID          string `json:"external_run_id"`
	NativeSuiteStatus      string `json:"native_suite_status"`
	CoverageMode           string `json:"coverage_mode"`
}

// EvidenceClaimResult is one v1.1 evidence claim result.
type EvidenceClaimResult struct {
	Slug              string             `json:"slug"`
	Title             string             `json:"title"`
	InternalID        string             `json:"internal_id"`
	Result            string             `json:"result"`
	EvidenceType      []string           `json:"evidence_type"`
	EvidenceArtifacts []EvidenceArtifact `json:"evidence_artifacts"`
	ObservedOutputs   *ObservedOutputs   `json:"observed_outputs,omitempty"`
	Limitations       []string           `json:"limitations,omitempty"`
}

// EvidenceArtifact references stored evidence bytes.
type EvidenceArtifact struct {
	Type        string `json:"type"`
	Ref         string `json:"ref"`
	SHA256      string `json:"sha256"`
	MimeType    string `json:"mime_type"`
	Description string `json:"description,omitempty"`
}

// RawEvidenceRefs lists stored raw evidence references.
type RawEvidenceRefs struct {
	ExternalTestOutputRefs []string `json:"external_test_output_refs"`
}

// HashEntry binds a stored reference to the hash of its exact bytes.
type HashEntry struct {
	Ref         string `json:"ref"`
	SHA256      string `json:"sha256"`
	Algorithm   string `json:"algorithm"`
	MimeType    string `json:"mime_type,omitempty"`
	Description string `json:"description,omitempty"`
}

// ObservedOutputs is the imported external result as observed (an open
// object in v1.1). It keeps every native assertion and scenario outcome.
type ObservedOutputs struct {
	ExternalSuite              string              `json:"external_suite"`
	ExternalRunID              string              `json:"external_run_id"`
	ProfileID                  string              `json:"profile_id"`
	Category                   string              `json:"category"`
	SelectionIDs               []string            `json:"selection_ids"`
	ExecutionStatus            string              `json:"execution_status"`
	ExecutionError             string              `json:"execution_error,omitempty"`
	ExitCode                   *int                `json:"exit_code,omitempty"`
	ExternalResult             string              `json:"external_result"`
	NativeSuiteStatus          string              `json:"native_suite_status"`
	CoverageMode               string              `json:"coverage_mode"`
	AssertionCount             int                 `json:"assertion_count"`
	NativeFailedAssertionCount int                 `json:"native_failed_assertion_count"`
	NativeStatusCounts         NativeStatusCounts  `json:"native_status_counts"`
	MappingStatus              string              `json:"mapping_status"`
	UnmappedRequirementIDs     []string            `json:"unmapped_requirement_ids"`
	Sessions                   []ObservedSession   `json:"sessions"`
	ScenarioOutcomes           []ObservedScenario  `json:"scenario_outcomes"`
	Assertions                 []ObservedAssertion `json:"assertions"`
	RunnerLimitations          []string            `json:"runner_limitations"`
}

// NativeStatusCounts counts assertions per native status.
type NativeStatusCounts struct {
	Pass       int `json:"PASS"`
	Fail       int `json:"FAIL"`
	NotReached int `json:"NOT_REACHED"`
}

// ObservedSession is a native session as reported.
type ObservedSession struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ObservedScenario is a native scenario outcome as reported.
type ObservedScenario struct {
	ScenarioID    string `json:"scenario_id"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Verdict       string `json:"verdict"`
}

// ObservedAssertion is one native assertion occurrence as reported.
type ObservedAssertion struct {
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
}
