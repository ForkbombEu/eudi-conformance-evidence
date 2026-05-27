package extractcontext

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/internal/discovery"
)

func TestRunWithMockCredimi(t *testing.T) {
	// Create a temporary input file
	inputDir := t.TempDir()
	inputPath := filepath.Join(inputDir, "input.json")
	input := `{
		"workflow_definition": {
			"steps": [
				{
					"id": "co-001",
					"use": "credential-offer",
					"with": {"credential_id": "test-cred-id"}
				},
				{
					"id": "vp-001",
					"use": "use-case-verification-deeplink",
					"with": {"use_case_id": "test-case-id"}
				}
			]
		}
	}`
	if err := os.WriteFile(inputPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	// Mock Credimi server
	var credimiServer *httptest.Server
	credimiServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/credential/deeplink":
			_, _ = w.Write([]byte(`openid-credential-offer://?credential_offer=%7B%22credential_issuer%22%3A%22https%3A%2F%2Fissuer.example%22%7D`))
		case "/api/verification/deeplink":
			_, _ = w.Write([]byte(`haip-vp://?request_uri=` + url.QueryEscape(credimiServer.URL+"/request.jwt") + `&request_uri_method=get`))
		case "/request.jwt":
			w.Header().Set("Content-Type", "application/jwt")
			_, _ = w.Write([]byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.c2ln"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer credimiServer.Close()

	outDir := t.TempDir()

	args := []string{
		"--temporal-input", inputPath,
		"--credimi-base-url", credimiServer.URL,
		"--out-dir", outDir,
		"--parallelism", "1",
		"--timeout", "5s",
	}

	if err := Run(args); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify output structure
	checkFile(t, filepath.Join(outDir, "discovered-steps.json"))
	checkFile(t, filepath.Join(outDir, "extraction-summary.json"))

	// Check discovered steps
	data, err := os.ReadFile(filepath.Join(outDir, "discovered-steps.json"))
	if err != nil {
		t.Fatalf("read discovered-steps: %v", err)
	}
	var disc struct {
		CredentialOfferSteps     []any `json:"credential_offer_steps"`
		PresentationRequestSteps []any `json:"presentation_request_steps"`
	}
	if err := json.Unmarshal(data, &disc); err != nil {
		t.Fatalf("unmarshal discovered-steps: %v", err)
	}
	if len(disc.CredentialOfferSteps) != 1 {
		t.Errorf("expected 1 credential-offer step, got %d", len(disc.CredentialOfferSteps))
	}
	if len(disc.PresentationRequestSteps) != 1 {
		t.Errorf("expected 1 presentation-request step, got %d", len(disc.PresentationRequestSteps))
	}

	// Check credential-offer output
	coDirs, _ := filepath.Glob(filepath.Join(outDir, "credential-offers", "*"))
	if len(coDirs) != 1 {
		t.Errorf("expected 1 credential-offer directory, got %d", len(coDirs))
	}
	if len(coDirs) > 0 {
		checkFile(t, filepath.Join(coDirs[0], "source-step.json"))
		checkFile(t, filepath.Join(coDirs[0], "credential-offer-resolution-chain.json"))
		checkFile(t, filepath.Join(coDirs[0], "credential-offer-deeplink.txt"))
		checkFile(t, filepath.Join(coDirs[0], "credential-offer.json"))
	}

	// Check presentation-request output
	presDirs, _ := filepath.Glob(filepath.Join(outDir, "presentation-requests", "*"))
	if len(presDirs) != 1 {
		t.Errorf("expected 1 presentation-request directory, got %d", len(presDirs))
	}
	if len(presDirs) > 0 {
		checkFile(t, filepath.Join(presDirs[0], "source-step.json"))
		checkFile(t, filepath.Join(presDirs[0], "presentation-deeplink.txt"))
	}
}

func TestRunMissingInput(t *testing.T) {
	err := Run([]string{"--temporal-input", "/nonexistent/file.json", "--out-dir", "/tmp"})
	if err == nil {
		t.Error("expected error for missing input file")
	}
}

func TestRunInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	_ = os.WriteFile(inputPath, []byte(`not valid json`), 0644)

	err := Run([]string{"--temporal-input", inputPath, "--out-dir", dir})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRunMissingOutDir(t *testing.T) {
	err := Run([]string{"--temporal-input", "/some/file.json"})
	if err == nil {
		t.Error("expected error for missing --out-dir")
	}
}

func TestRunWithStrict(t *testing.T) {
	inputDir := t.TempDir()
	inputPath := filepath.Join(inputDir, "input.json")
	input := `{"workflow_definition":{"steps":[{"id":"co-001","use":"credential-offer","with":{"credential_id":"test-id"}}]}}`
	_ = os.WriteFile(inputPath, []byte(input), 0644)

	// Server that always fails
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer failServer.Close()

	outDir := t.TempDir()

	err := Run([]string{
		"--temporal-input", inputPath,
		"--credimi-base-url", failServer.URL,
		"--out-dir", outDir,
		"--strict",
		"--timeout", "5s",
	})
	if err == nil {
		t.Error("expected error in strict mode with failed extraction")
	}
}

func TestRunExtractionBytes(t *testing.T) {
	// Mock Credimi that returns a direct credential offer
	var server *httptest.Server //nolint:staticcheck
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/credential/deeplink":
			_, _ = w.Write([]byte(`openid-credential-offer://?credential_offer=%7B%22credential_issuer%22%3A%22https%3A%2F%2Fissuer.example%22%7D`))
		case "/api/verification/deeplink":
			_, _ = w.Write([]byte(`haip-vp://?request_uri=` + url.QueryEscape(server.URL+"/request.jwt") + `&request_uri_method=get`))
		case "/request.jwt":
			w.Header().Set("Content-Type", "application/jwt")
			_, _ = w.Write([]byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.c2ln"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	input := `{"workflow_definition":{"steps":[
		{"id":"co-001","use":"credential-offer","with":{"credential_id":"test-id"}},
		{"id":"vp-001","use":"use-case-verification-deeplink","with":{"use_case_id":"test-case"}}
	]}}`

	client := &http.Client{Timeout: 5 * time.Second}
	collected, err := RunExtractionBytes([]byte(input), client, Options{
		CredimiBaseURL: server.URL,
		Parallelism:    1,
		IDEncoding:     "raw",
		MaxDepth:       5,
		PostStrategy:   "auto",
		Timeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("RunExtractionBytes failed: %v", err)
	}
	if collected.Summary.CredentialOfferCount != 1 {
		t.Errorf("expected 1 offer, got %d", collected.Summary.CredentialOfferCount)
	}
	if collected.Summary.PresentationRequestCount != 1 {
		t.Errorf("expected 1 request, got %d", collected.Summary.PresentationRequestCount)
	}
}

func TestRunExtractionSteps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/credential/deeplink":
			_, _ = w.Write([]byte(`openid-credential-offer://?credential_offer=%7B%22credential_issuer%22%3A%22https%3A%2F%2Fissuer.example%22%7D`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	steps := []discovery.Step{
		{PipelineOrder: 0, StepID: "co-001", Use: "credential-offer", CredentialID: "test-id"},
	}

	client := &http.Client{Timeout: 5 * time.Second}
	collected, err := RunExtractionSteps(steps, client, Options{
		CredimiBaseURL: server.URL,
		Parallelism:    1,
		IDEncoding:     "raw",
		MaxDepth:       5,
		Timeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("RunExtractionSteps failed: %v", err)
	}
	if collected.Summary.CredentialOfferCount != 1 {
		t.Errorf("expected 1 offer, got %d", collected.Summary.CredentialOfferCount)
	}
}

func checkFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", path)
	}
}
