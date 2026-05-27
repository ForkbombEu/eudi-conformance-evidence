package output

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/forkbombeu/eudi-conformance-evidence/internal/credoffer"
	"github.com/forkbombeu/eudi-conformance-evidence/internal/discovery"
	"github.com/forkbombeu/eudi-conformance-evidence/internal/jwt"
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

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	err := WriteJSON(&buf, map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected output")
	}
	if !json.Valid(buf.Bytes()) {
		t.Error("output is not valid JSON")
	}
}

func TestCollect(t *testing.T) {
	disc := &discovery.Result{
		CredentialOfferSteps: []discovery.Step{
			{PipelineOrder: 0, StepID: "co-001", Use: "credential-offer", CredentialID: "id-1"},
		},
	}

	offerResults := []*credoffer.Result{
		{Status: "ok", StepID: "co-001", CredentialID: "id-1", IssuerMetadata: json.RawMessage(`{"iss":"test"}`)},
	}

	c := Collect(disc, offerResults, nil, "start", "end")
	if c.Summary.Status != "ok" {
		t.Errorf("expected ok, got %s", c.Summary.Status)
	}
	if c.Summary.CredentialOfferCount != 1 {
		t.Errorf("expected 1, got %d", c.Summary.CredentialOfferCount)
	}
	if c.WellKnown == nil {
		t.Error("expected well-known convenience alias")
	}
}

func TestCollectPartial(t *testing.T) {
	disc := &discovery.Result{
		CredentialOfferSteps: []discovery.Step{
			{PipelineOrder: 0, StepID: "co-001", Use: "credential-offer", CredentialID: "id-1"},
			{PipelineOrder: 1, StepID: "co-002", Use: "credential-offer", CredentialID: "id-2"},
		},
	}

	offerResults := []*credoffer.Result{
		{Status: "ok", StepID: "co-001", CredentialID: "id-1"},
		{Status: "error", StepID: "co-002", CredentialID: "id-2",
			Error: credoffer.NewError("e", "msg", "human", "url", 500, true)},
	}

	c := Collect(disc, offerResults, nil, "start", "end")
	if c.Summary.Status != "partial" {
		t.Errorf("expected partial, got %s", c.Summary.Status)
	}
}

func TestWriteToDir(t *testing.T) {
	outDir := t.TempDir()

	c := &CollectedResult{
		DiscoveredSteps: &discovery.Result{},
		Summary:         &ExtractionSummary{Status: "ok"},
		WellKnown:       json.RawMessage(`{"iss":"test"}`),
	}

	err := WriteToDir(outDir, c)
	if err != nil {
		t.Fatalf("WriteToDir failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, ".well-known.json")); os.IsNotExist(err) {
		t.Error(".well-known.json not created")
	}
}

func TestWriteToDirWithOfferResult(t *testing.T) {
	outDir := t.TempDir()

	c := &CollectedResult{
		DiscoveredSteps: &discovery.Result{
			CredentialOfferSteps: []discovery.Step{
				{PipelineOrder: 0, StepID: "co-001", Use: "credential-offer", CredentialID: "id-1"},
			},
		},
		Summary: &ExtractionSummary{Status: "ok"},
		OfferResults: []*credoffer.Result{
			{
				Status: "ok", StepID: "co-001", CredentialID: "id-1",
				DeeplinkURI:     "openid://test",
				CredentialOffer: json.RawMessage(`{"test":"ok"}`),
				IssuerMetadata:  json.RawMessage(`{"iss":"test"}`),
				IssuerMetadataFetch: &credoffer.MetadataFetch{
					URL:        "https://example.com",
					HTTPStatus: 200,
					Format:     "json",
				},
			},
		},
	}

	err := WriteToDir(outDir, c)
	if err != nil {
		t.Fatalf("WriteToDir failed: %v", err)
	}

	// Check all expected files
	dirs, _ := filepath.Glob(filepath.Join(outDir, "credential-offers", "*"))
	if len(dirs) != 1 {
		t.Fatalf("expected 1 offer dir, got %d", len(dirs))
	}
	for _, f := range []string{
		"source-step.json",
		"credential-offer-resolution-chain.json",
		"credential-offer-deeplink.txt",
		"credential-offer.json",
		"well-known.json",
		"issuer-metadata-fetch.json",
	} {
		if _, err := os.Stat(filepath.Join(dirs[0], f)); os.IsNotExist(err) {
			t.Errorf("%s not created", f)
		}
	}
}

func TestWriteToDirWithPresResult(t *testing.T) {
	outDir := t.TempDir()

	c := &CollectedResult{
		DiscoveredSteps: &discovery.Result{
			PresentationRequestSteps: []discovery.Step{
				{PipelineOrder: 0, StepID: "vp-001", Use: "use-case-verification-deeplink", UseCaseID: "id-1"},
			},
		},
		Summary: &ExtractionSummary{Status: "ok"},
		PresResults: []*presentation.Result{
			{
				Status: "ok", StepID: "vp-001", UseCaseID: "id-1",
				DeeplinkURI:      "haip-vp://test",
				RequestURI:       "https://example.com/request",
				RequestURIMethod: "get",
				RequestURIRaw:    "raw-jwt",
				RequestURIFetch: &presentation.RequestURIFetch{
					URL:        "https://example.com",
					HTTPStatus: 200,
					Format:     "jwt",
				},
			},
		},
	}

	err := WriteToDir(outDir, c)
	if err != nil {
		t.Fatalf("WriteToDir failed: %v", err)
	}

	dirs, _ := filepath.Glob(filepath.Join(outDir, "presentation-requests", "*"))
	if len(dirs) != 1 {
		t.Fatalf("expected 1 pres dir, got %d", len(dirs))
	}
	for _, f := range []string{
		"source-step.json",
		"presentation-deeplink.txt",
		"request-uri-fetch.json",
		"request-uri-raw.jwt",
	} {
		if _, err := os.Stat(filepath.Join(dirs[0], f)); os.IsNotExist(err) {
			t.Errorf("%s not created", f)
		}
	}
}

func TestWriteToDirWithRequestObject(t *testing.T) {
	outDir := t.TempDir()

	c := &CollectedResult{
		DiscoveredSteps: &discovery.Result{
			PresentationRequestSteps: []discovery.Step{
				{PipelineOrder: 0, StepID: "vp-001", Use: "use-case-verification-deeplink", UseCaseID: "id-1"},
			},
		},
		Summary: &ExtractionSummary{Status: "ok"},
		PresResults: []*presentation.Result{
			{
				Status: "ok", StepID: "vp-001", UseCaseID: "id-1",
				RequestURI: "https://example.com/request", RequestURIMethod: "post",
				PostStrategy: "wallet_nonce", RequestURIRaw: "raw-jwt",
				RequestObject: &jwt.Token{
					Raw:              "raw-jwt",
					Header:           json.RawMessage(`{"alg":"ES256"}`),
					Payload:          json.RawMessage(`{"sub":"test"}`),
					SignaturePresent: true,
				},
			},
		},
	}

	err := WriteToDir(outDir, c)
	if err != nil {
		t.Fatalf("WriteToDir failed: %v", err)
	}

	dirs, _ := filepath.Glob(filepath.Join(outDir, "presentation-requests", "*"))
	if len(dirs) != 1 {
		t.Fatalf("expected 1 pres dir, got %d", len(dirs))
	}
	if _, err := os.Stat(filepath.Join(dirs[0], "request-uri-output.json")); os.IsNotExist(err) {
		t.Error("request-uri-output.json not created")
	}
	// Verify content
	data, _ := os.ReadFile(filepath.Join(dirs[0], "request-uri-output.json"))
	if !json.Valid(data) {
		t.Error("request-uri-output.json is not valid JSON")
	}
}

func TestWriteToDirRequestURIOutput(t *testing.T) {
	outDir := t.TempDir()

	c := &CollectedResult{
		DiscoveredSteps: &discovery.Result{
			PresentationRequestSteps: []discovery.Step{
				{PipelineOrder: 0, StepID: "vp-001", Use: "use-case-verification-deeplink", UseCaseID: "id-1"},
			},
		},
		Summary: &ExtractionSummary{Status: "ok"},
		PresResults: []*presentation.Result{
			{
				Status: "ok", StepID: "vp-001", UseCaseID: "id-1",
				RequestURI: "https://example.com/request", RequestURIMethod: "post",
				PostStrategy: "wallet_nonce", RequestURIRaw: "raw",
			},
		},
		RequestURIOutput: json.RawMessage(`{"test":"ok"}`),
	}

	err := WriteToDir(outDir, c)
	if err != nil {
		t.Fatalf("WriteToDir failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "request-uri-output.json")); os.IsNotExist(err) {
		t.Error("request-uri-output.json not created")
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
