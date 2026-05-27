package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverEUDIIssVer(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "EUDI-iss-ver", "input.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	result, err := Discover(data)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.CredentialOfferSteps) != 1 {
		t.Errorf("expected 1 credential-offer step, got %d", len(result.CredentialOfferSteps))
	}
	if len(result.PresentationRequestSteps) != 1 {
		t.Errorf("expected 1 presentation-request step, got %d", len(result.PresentationRequestSteps))
	}

	co := result.CredentialOfferSteps[0]
	if co.Use != "credential-offer" {
		t.Errorf("expected use credential-offer, got %s", co.Use)
	}
	if co.StepID != "eudiw-pid-pid-vc-sd-jwt-haip-vci-0002" {
		t.Errorf("unexpected step ID: %s", co.StepID)
	}
	if co.CredentialID != "/forkbomb-bv-andrea/misc-issuer-integration-demo/eudiw-pid-pid-vc-sd-jwt-haip-vci" {
		t.Errorf("unexpected credential ID: %s", co.CredentialID)
	}

	pr := result.PresentationRequestSteps[0]
	if pr.Use != "use-case-verification-deeplink" {
		t.Errorf("expected use use-case-verification-deeplink, got %s", pr.Use)
	}
	if pr.StepID != "eudiw-pid-verifier-dc-sd-jwt-0004" {
		t.Errorf("unexpected step ID: %s", pr.StepID)
	}
}

func TestDiscoverEUDIIss2NestedWith(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "EUDI-iss2", "input.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	result, err := Discover(data)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.CredentialOfferSteps) != 1 {
		t.Errorf("expected 1 credential-offer step, got %d", len(result.CredentialOfferSteps))
	}

	co := result.CredentialOfferSteps[0]
	if co.CredentialID != "forkbomb-bv-andrea/misc-issuer-integration-demo/eudiw-pid-sd-jwt-vc-issuer-backend" {
		t.Errorf("unexpected credential ID: %s", co.CredentialID)
	}
}

func TestDiscoverAgeVerificationNoSteps(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "AgeVerification", "input.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	result, err := Discover(data)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.CredentialOfferSteps) != 0 {
		t.Errorf("expected 0 credential-offer steps, got %d", len(result.CredentialOfferSteps))
	}
}

func TestDiscoverTalao(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "Talao-iss-cred13", "input.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	result, err := Discover(data)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.CredentialOfferSteps) != 1 {
		t.Errorf("expected 1 credential-offer step, got %d", len(result.CredentialOfferSteps))
	}
}

func TestDiscoverMultipaz(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "Multipaz", "input.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	result, err := Discover(data)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.CredentialOfferSteps) != 1 {
		t.Errorf("expected 1 credential-offer step, got %d", len(result.CredentialOfferSteps))
	}
}

func TestDiscoverNoDeduplication(t *testing.T) {
	// Pipeline with the same credential_id appearing twice — must not deduplicate
	input := `{
		"workflow_definition": {
			"steps": [
				{"id": "step-001", "use": "credential-offer", "with": {"credential_id": "same-id"}},
				{"id": "step-002", "use": "credential-offer", "with": {"credential_id": "same-id"}}
			]
		}
	}`

	result, err := Discover([]byte(input))
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.CredentialOfferSteps) != 2 {
		t.Errorf("expected 2 steps (no deduplication), got %d", len(result.CredentialOfferSteps))
	}
}

func TestDiscoverInvalidJSON(t *testing.T) {
	_, err := Discover([]byte(`{invalid}`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDiscoverRecursiveScan(t *testing.T) {
	// Pipeline in an unusual nested shape
	input := `{
		"custom_wrapper": {
			"nested": {
				"steps": [
					{"id": "co-001", "use": "credential-offer", "with": {"credential_id": "test-id"}},
					{"id": "vp-001", "use": "use-case-verification-deeplink", "with": {"use_case_id": "test-case"}}
				]
			}
		}
	}`

	result, err := Discover([]byte(input))
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.CredentialOfferSteps) != 1 {
		t.Errorf("expected 1 credential-offer step, got %d", len(result.CredentialOfferSteps))
	}
	if len(result.PresentationRequestSteps) != 1 {
		t.Errorf("expected 1 presentation-request step, got %d", len(result.PresentationRequestSteps))
	}
}

func TestPipelineOrderPreservation(t *testing.T) {
	input := `{
		"workflow_definition": {
			"steps": [
				{"id": "step-a", "use": "credential-offer", "with": {"credential_id": "id-a"}},
				{"id": "step-b", "use": "credential-offer", "with": {"credential_id": "id-b"}},
				{"id": "step-c", "use": "credential-offer", "with": {"credential_id": "id-c"}}
			]
		}
	}`

	result, err := Discover([]byte(input))
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.CredentialOfferSteps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.CredentialOfferSteps))
	}
	if result.CredentialOfferSteps[0].PipelineOrder != 0 {
		t.Errorf("step 0 order: %d", result.CredentialOfferSteps[0].PipelineOrder)
	}
	if result.CredentialOfferSteps[2].PipelineOrder != 2 {
		t.Errorf("step 2 order: %d", result.CredentialOfferSteps[2].PipelineOrder)
	}
}

func TestDiscoveryResultJSON(t *testing.T) {
	input := `{
		"workflow_definition": {
			"steps": [
				{"id": "co-001", "use": "credential-offer", "with": {"credential_id": "test-id"}},
				{"id": "vp-001", "use": "use-case-verification-deeplink", "with": {"use_case_id": "test-case"}}
			]
		}
	}`

	result, err := Discover([]byte(input))
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if !json.Valid(out) {
		t.Error("output is not valid JSON")
	}
}
