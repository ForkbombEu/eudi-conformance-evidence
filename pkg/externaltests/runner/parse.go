package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"time"
)

const (
	maxSelectionIDs   = 32
	maxMessageLength  = 200
	maxQuotedValueLen = 64
)

var (
	categories       = []string{"issuance", "presentation"}
	engineIDs        = []string{EngineITWCT, EngineWCT}
	terminalStatuses = []string{"completed", "cancelled", "timed_out", "error"}
	actorTypes       = []string{"wallet", "issuer", "verifier", "relying_party"}
	coverageModes    = []string{"it_wallet_profile", "interoperability"}
	nativeStatuses   = []string{"PASS", "FAIL", "NOT_REACHED"}
	scenarioVerdicts = []string{"PASS", "FAIL", "INCONCLUSIVE"}
	summaryResults   = []string{"pass", "fail", "not_tested", "inconclusive", "partial"}
	safeArtifactID   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	lowercaseSHA256  = regexp.MustCompile(`^[a-f0-9]{64}$`)
	topLevelKeys     = []string{"contract_version", "run_id", "profile_id", "category", "engine", "execution", "selection", "target", "native", "summary", "artifacts", "limitations"}
	engineKeys       = []string{"id", "upstream_repository", "fork_repository", "version", "revision"}
	executionKeys    = []string{"status", "started_at"}
	nativeKeys       = []string{"sessions", "assertions", "scenario_outcomes", "native_suite_status", "coverage_mode"}
	summaryKeys      = []string{"result", "assertion_count", "native_failed_assertion_count"}
	assertionKeys    = []string{"assertion_id", "full_title", "ancestor_titles", "file", "phase", "native_status"}
	scenarioKeys     = []string{"scenario_id", "verdict"}
	sessionKeys      = []string{"id", "status"}
	artifactKeys     = []string{"artifact_id", "filename", "mime_type", "size_bytes", "sha256", "download_url"}
	targetKeys       = []string{"actor_type", "actor_name"}
	selectionKeys    = []string{"ids"}
)

// ValidationError reports why an envelope was rejected. Path is a bounded
// JSON path; Message never contains a whole field value or document.
type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	return "runner result: " + e.Path + ": " + e.Message
}

func invalid(path, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if len(message) > maxMessageLength {
		message = message[:maxMessageLength] + "..."
	}
	return &ValidationError{Path: path, Message: message}
}

// quote renders a bounded diagnostic copy of a short enum-like value.
func quote(value string) string {
	if len(value) > maxQuotedValueLen {
		value = value[:maxQuotedValueLen] + "..."
	}
	return strconv.Quote(value)
}

// Parse reads one runner result envelope (at most MaxResultBytes), validates
// it and returns it typed, owning the exact bytes read in Raw.
//
// A structurally valid cancelled, timed-out or error envelope with empty
// evidence is returned without error; see RunnerResult.Incomplete.
func Parse(r io.Reader) (*RunnerResult, error) {
	raw, err := io.ReadAll(io.LimitReader(r, MaxResultBytes+1))
	if err != nil {
		return nil, fmt.Errorf("runner result: read: %w", err)
	}
	if len(raw) > MaxResultBytes {
		return nil, invalid("$", "exceeds the %d-byte limit", MaxResultBytes)
	}
	if err := checkStructure(raw); err != nil {
		return nil, err
	}

	var result RunnerResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, invalid("$", "malformed envelope: %v", err)
	}
	if err := validate(&result); err != nil {
		return nil, err
	}
	result.Raw = raw
	return &result, nil
}

// checkStructure accepts exactly one JSON object and checks that every
// required key is present and not null, before typed decoding hides absence.
func checkStructure(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var top json.RawMessage
	if err := decoder.Decode(&top); err != nil {
		return invalid("$", "malformed JSON: %v", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return invalid("$", "trailing data after the JSON object")
	}

	fields, err := object("$", top, topLevelKeys...)
	if err != nil {
		return err
	}
	for key := range fields {
		if !slices.Contains(topLevelKeys, key) {
			return invalid("$."+key, "unknown top-level field")
		}
	}
	if err := checkNested(fields); err != nil {
		return err
	}
	return nil
}

func checkNested(fields map[string]json.RawMessage) error {
	nested := []struct {
		key  string
		keys []string
	}{
		{"engine", engineKeys},
		{"execution", executionKeys},
		{"selection", selectionKeys},
		{"target", targetKeys},
		{"summary", summaryKeys},
	}
	for _, n := range nested {
		if _, err := object("$."+n.key, fields[n.key], n.keys...); err != nil {
			return err
		}
	}
	native, err := object("$.native", fields["native"], nativeKeys...)
	if err != nil {
		return err
	}
	lists := []struct {
		path string
		raw  json.RawMessage
		keys []string
	}{
		{"$.native.sessions", native["sessions"], sessionKeys},
		{"$.native.assertions", native["assertions"], assertionKeys},
		{"$.native.scenario_outcomes", native["scenario_outcomes"], scenarioKeys},
		{"$.artifacts", fields["artifacts"], artifactKeys},
	}
	for _, l := range lists {
		if err := objectArray(l.path, l.raw, l.keys...); err != nil {
			return err
		}
	}
	return nil
}

func object(path string, raw json.RawMessage, required ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &fields) != nil {
		return nil, invalid(path, "must be a JSON object")
	}
	for _, key := range required {
		value, ok := fields[key]
		if !ok || string(value) == "null" {
			return nil, invalid(path+"."+key, "required field is missing")
		}
	}
	return fields, nil
}

func objectArray(path string, raw json.RawMessage, required ...string) error {
	var items []json.RawMessage
	if len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &items) != nil {
		return invalid(path, "must be a JSON array")
	}
	for i, item := range items {
		if _, err := object(fmt.Sprintf("%s[%d]", path, i), item, required...); err != nil {
			return err
		}
	}
	return nil
}

func validate(r *RunnerResult) error {
	checks := []func(*RunnerResult) error{
		validateIdentity,
		validateExecution,
		validateSelection,
		validateTarget,
		validateNative,
		validateSummary,
		validateArtifacts,
	}
	for _, check := range checks {
		if err := check(r); err != nil {
			return err
		}
	}
	return nil
}

func oneOf(path, value string, allowed []string) error {
	if !slices.Contains(allowed, value) {
		return invalid(path, "unsupported value %s", quote(value))
	}
	return nil
}

func nonEmpty(path, value string) error {
	if value == "" {
		return invalid(path, "must not be empty")
	}
	return nil
}

func validateIdentity(r *RunnerResult) error {
	if r.ContractVersion != ContractVersion {
		return invalid("$.contract_version", "unsupported contract version %s", quote(r.ContractVersion))
	}
	if err := oneOf("$.engine.id", r.Engine.ID, engineIDs); err != nil {
		return err
	}
	for path, value := range map[string]string{
		"$.engine.upstream_repository": r.Engine.UpstreamRepository,
		"$.engine.fork_repository":     r.Engine.ForkRepository,
		"$.engine.version":             r.Engine.Version,
		"$.engine.revision":            r.Engine.Revision,
		"$.run_id":                     r.RunID,
		"$.profile_id":                 r.ProfileID,
	} {
		if err := nonEmpty(path, value); err != nil {
			return err
		}
	}
	return oneOf("$.category", r.Category, categories)
}

func validateExecution(r *RunnerResult) error {
	if err := oneOf("$.execution.status", r.Execution.Status, terminalStatuses); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, r.Execution.StartedAt); err != nil {
		return invalid("$.execution.started_at", "must be an RFC 3339 date-time")
	}
	if r.Execution.FinishedAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, r.Execution.FinishedAt); err != nil {
			return invalid("$.execution.finished_at", "must be an RFC 3339 date-time")
		}
	}
	return nil
}

func validateSelection(r *RunnerResult) error {
	ids := r.Selection.IDs
	if len(ids) == 0 || len(ids) > maxSelectionIDs {
		return invalid("$.selection.ids", "must contain 1 to %d IDs", maxSelectionIDs)
	}
	for i, id := range ids {
		path := fmt.Sprintf("$.selection.ids[%d]", i)
		if err := nonEmpty(path, id); err != nil {
			return err
		}
		if slices.Contains(ids[:i], id) {
			return invalid(path, "duplicate selection ID %s", quote(id))
		}
	}
	return nil
}

func validateTarget(r *RunnerResult) error {
	if err := oneOf("$.target.actor_type", r.Target.ActorType, actorTypes); err != nil {
		return err
	}
	return nonEmpty("$.target.actor_name", r.Target.ActorName)
}

func validateNative(r *RunnerResult) error {
	n := r.Native
	if err := oneOf("$.native.coverage_mode", n.CoverageMode, coverageModes); err != nil {
		return err
	}
	if err := nonEmpty("$.native.native_suite_status", n.NativeSuiteStatus); err != nil {
		return err
	}
	for i, s := range n.Sessions {
		path := fmt.Sprintf("$.native.sessions[%d]", i)
		if err := nonEmpty(path+".id", s.ID); err != nil {
			return err
		}
		if err := nonEmpty(path+".status", s.Status); err != nil {
			return err
		}
	}
	for i, a := range n.Assertions {
		path := fmt.Sprintf("$.native.assertions[%d]", i)
		if err := nonEmpty(path+".assertion_id", a.AssertionID); err != nil {
			return err
		}
		if err := oneOf(path+".native_status", a.NativeStatus, nativeStatuses); err != nil {
			return err
		}
		if a.NativeRecord != "" && !json.Valid([]byte(a.NativeRecord)) {
			return invalid(path+".raw", "must be JSON text")
		}
	}
	for i, s := range n.ScenarioOutcomes {
		path := fmt.Sprintf("$.native.scenario_outcomes[%d]", i)
		if err := nonEmpty(path+".scenario_id", s.ScenarioID); err != nil {
			return err
		}
		if err := oneOf(path+".verdict", s.Verdict, scenarioVerdicts); err != nil {
			return err
		}
	}
	return nil
}

// validateSummary checks the declared counts against the assertions and
// refuses a pass the evidence does not support.
func validateSummary(r *RunnerResult) error {
	s := r.Summary
	if err := oneOf("$.summary.result", s.Result, summaryResults); err != nil {
		return err
	}
	failed, notReached := 0, 0
	for _, a := range r.Native.Assertions {
		switch a.NativeStatus {
		case "FAIL":
			failed++
		case "NOT_REACHED":
			notReached++
		}
	}
	if s.AssertionCount != len(r.Native.Assertions) {
		return invalid("$.summary.assertion_count", "declares %d but %d assertions are present", s.AssertionCount, len(r.Native.Assertions))
	}
	if s.NativeFailedAssertionCount != failed {
		return invalid("$.summary.native_failed_assertion_count", "declares %d but %d assertions are FAIL", s.NativeFailedAssertionCount, failed)
	}
	if s.Result != "pass" {
		return nil
	}
	scenarioNotPass := slices.ContainsFunc(r.Native.ScenarioOutcomes, func(o ScenarioOutcome) bool {
		return o.Verdict != "PASS"
	})
	switch {
	case len(r.Native.Assertions) == 0:
		return invalid("$.summary.result", "pass conflicts with zero assertions")
	case r.Execution.Status != "completed":
		return invalid("$.summary.result", "pass conflicts with execution status %s", quote(r.Execution.Status))
	case failed > 0 || notReached > 0:
		return invalid("$.summary.result", "pass conflicts with FAIL or NOT_REACHED assertions")
	case scenarioNotPass:
		return invalid("$.summary.result", "pass conflicts with a non-PASS scenario outcome")
	case r.Native.NativeSuiteStatus != "PASSED":
		return invalid("$.summary.result", "pass conflicts with native suite status %s", quote(r.Native.NativeSuiteStatus))
	}
	return nil
}

func validateArtifacts(r *RunnerResult) error {
	prefix := "/v1/runs/" + r.RunID + "/"
	seen := make(map[string]bool, len(r.Artifacts))
	for i, a := range r.Artifacts {
		path := fmt.Sprintf("$.artifacts[%d]", i)
		if !safeArtifactID.MatchString(a.ArtifactID) || bytes.Contains([]byte(a.ArtifactID), []byte("..")) {
			return invalid(path+".artifact_id", "is not a safe opaque name")
		}
		if seen[a.ArtifactID] {
			return invalid(path+".artifact_id", "duplicate artifact ID %s", quote(a.ArtifactID))
		}
		seen[a.ArtifactID] = true
		if !lowercaseSHA256.MatchString(a.SHA256) {
			return invalid(path+".sha256", "must be 64 lowercase hex characters")
		}
		if a.SizeBytes < 0 {
			return invalid(path+".size_bytes", "must not be negative")
		}
		if len(a.DownloadURL) <= len(prefix) || a.DownloadURL[:len(prefix)] != prefix {
			return invalid(path+".download_url", "must be a same-origin path under this run")
		}
	}
	return nil
}
