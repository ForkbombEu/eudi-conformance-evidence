package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/forkbombeu/eudi-conformance-evidence/internal/credoffer"
	"github.com/forkbombeu/eudi-conformance-evidence/internal/discovery"
	"github.com/forkbombeu/eudi-conformance-evidence/internal/presentation"
)

func TestWriteAll(t *testing.T) {
	outDir := t.TempDir()

	disc := &discovery.Result{
		CredentialOfferSteps: []discovery.Step{
			{PipelineOrder: 0, StepID: "co-001", Use: "credential-offer", CredentialID: "test-cred-id"},
		},
		PresentationRequestSteps: []discovery.Step{
			{PipelineOrder: 1, StepID: "vp-001", Use: "use-case-verification-deeplink", UseCaseID: "test-case-id"},
		},
	}

	offerResults := []*credoffer.Result{
		{
			Status: "ok", StepID: "co-001", CredentialID: "test-cred-id",
			DeeplinkURI:     "openid-credential-offer://?credential_offer=%7B%22iss%22%3A%22test%22%7D",
			CredentialOffer: json.RawMessage(`{"credential_issuer":"https://issuer.example"}`),
			IssuerMetadata:  json.RawMessage(`{"credential_issuer":"https://issuer.example"}`),
			FinalOfferPath:  "credential-offer.json",
		},
	}

	presResults := []*presentation.Result{
		{
			Status: "ok", StepID: "vp-001", UseCaseID: "test-case-id",
			DeeplinkURI: "haip-vp://?request_uri=https://example.com/request",
			RequestURI:  "https://example.com/request", RequestURIMethod: "get",
			RequestURIRaw: "raw-jwt-body",
		},
	}

	err := WriteAll(outDir, disc, offerResults, presResults, "2024-01-01T00:00:00Z", "2024-01-01T00:00:01Z", false)
	if err != nil {
		t.Fatalf("WriteAll failed: %v", err)
	}

	// Check discovered-steps.json
	if _, err := os.Stat(filepath.Join(outDir, "discovered-steps.json")); os.IsNotExist(err) {
		t.Error("discovered-steps.json not created")
	}
	// Check extraction-summary.json
	if _, err := os.Stat(filepath.Join(outDir, "extraction-summary.json")); os.IsNotExist(err) {
		t.Error("extraction-summary.json not created")
	}

	// Check credential-offer directory
	coDir := filepath.Join(outDir, "credential-offers")
	if _, err := os.Stat(coDir); os.IsNotExist(err) {
		t.Error("credential-offers directory not created")
	}

	// Check convenience aliases (exactly one successful offer/request)
	if _, err := os.Stat(filepath.Join(outDir, ".well-known.json")); os.IsNotExist(err) {
		t.Error(".well-known.json alias not created")
	}

	// Check presentation-request directory
	presDir := filepath.Join(outDir, "presentation-requests")
	if _, err := os.Stat(presDir); os.IsNotExist(err) {
		t.Error("presentation-requests directory not created")
	}
}

func TestWriteAllWithError(t *testing.T) {
	outDir := t.TempDir()

	disc := &discovery.Result{
		CredentialOfferSteps: []discovery.Step{
			{PipelineOrder: 0, StepID: "co-001", Use: "credential-offer", CredentialID: "test-cred-id"},
		},
	}

	offerResults := []*credoffer.Result{
		{
			Status: "error", StepID: "co-001", CredentialID: "test-cred-id",
			Error: credoffer.NewError("test_error", "Test error", "A human readable error", "http://example.com", 500, true),
		},
	}

	err := WriteAll(outDir, disc, offerResults, nil, "start", "end", false)
	if err != nil {
		t.Fatalf("WriteAll failed: %v", err)
	}

	// Check error.json was written
	matches, _ := filepath.Glob(filepath.Join(outDir, "credential-offers", "*", "error.json"))
	if len(matches) != 1 {
		t.Error("error.json not created for failed extraction")
	}

	// Check summary status
	summaryPath := filepath.Join(outDir, "extraction-summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var summary ExtractionSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if summary.Status != "error" {
		t.Errorf("expected status error, got %s", summary.Status)
	}
}

func TestWriteAllNoConvenienceAliases(t *testing.T) {
	outDir := t.TempDir()

	disc := &discovery.Result{
		CredentialOfferSteps: []discovery.Step{
			{PipelineOrder: 0, StepID: "co-001", Use: "credential-offer", CredentialID: "id-1"},
			{PipelineOrder: 1, StepID: "co-002", Use: "credential-offer", CredentialID: "id-2"},
		},
	}

	offerResults := []*credoffer.Result{
		{Status: "ok", StepID: "co-001", CredentialID: "id-1", CredentialOffer: json.RawMessage(`{"iss":"1"}`), IssuerMetadata: json.RawMessage(`{"iss":"1"}`)},
		{Status: "ok", StepID: "co-002", CredentialID: "id-2", CredentialOffer: json.RawMessage(`{"iss":"2"}`), IssuerMetadata: json.RawMessage(`{"iss":"2"}`)},
	}

	err := WriteAll(outDir, disc, offerResults, nil, "start", "end", false)
	if err != nil {
		t.Fatalf("WriteAll failed: %v", err)
	}

	// No convenience alias when more than one successful offer
	if _, err := os.Stat(filepath.Join(outDir, ".well-known.json")); !os.IsNotExist(err) {
		t.Error(".well-known.json should not exist when multiple offers succeed")
	}
}
