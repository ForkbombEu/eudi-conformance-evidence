// Package output writes extraction results to the filesystem.
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/forkbombeu/eudi-conformance-evidence/internal/credoffer"
	"github.com/forkbombeu/eudi-conformance-evidence/internal/discovery"
	"github.com/forkbombeu/eudi-conformance-evidence/internal/presentation"
)

// ExtractionSummary is written to extraction-summary.json.
type ExtractionSummary struct {
	Status                          string   `json:"status"`
	StartedAt                       string   `json:"started_at"`
	FinishedAt                      string   `json:"finished_at"`
	CredentialOfferCount            int      `json:"credential_offer_count"`
	CredentialOfferSuccessCount     int      `json:"credential_offer_success_count"`
	PresentationRequestCount        int      `json:"presentation_request_count"`
	PresentationRequestSuccessCount int      `json:"presentation_request_success_count"`
	Warnings                        []string `json:"warnings"`
	Errors                          []string `json:"errors"`
}

// WriteAll writes all extraction outputs to the output directory.
func WriteAll(outDir string, disc *discovery.Result, offerResults []*credoffer.Result, presResults []*presentation.Result,
	startedAt, finishedAt string, strict bool) error {

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Write discovered-steps.json
	if err := writeJSON(filepath.Join(outDir, "discovered-steps.json"), disc); err != nil {
		return err
	}

	// Write credential offers
	offerDir := filepath.Join(outDir, "credential-offers")
	offerSuccess := 0
	for i, r := range offerResults {
		subDir := fmt.Sprintf("%04d-%s", i, sanitizeDirName(r.StepID))
		if err := writeCredentialOffer(filepath.Join(offerDir, subDir), disc.CredentialOfferSteps[i], r); err != nil {
			return err
		}
		if r.Status == "ok" {
			offerSuccess++
		}
	}

	// Write presentation requests
	presDir := filepath.Join(outDir, "presentation-requests")
	presSuccess := 0
	for i, r := range presResults {
		subDir := fmt.Sprintf("%04d-%s", i, sanitizeDirName(r.StepID))
		if err := writePresentationRequest(filepath.Join(presDir, subDir), disc.PresentationRequestSteps[i], r); err != nil {
			return err
		}
		if r.Status == "ok" {
			presSuccess++
		}
	}

	// Write convenience top-level aliases
	if offerSuccess == 1 {
		for _, r := range offerResults {
			if r.Status == "ok" && r.IssuerMetadata != nil {
				if err := writeRaw(filepath.Join(outDir, ".well-known.json"), r.IssuerMetadata); err != nil {
					return err
				}
				break
			}
		}
	}

	if presSuccess == 1 {
		for _, r := range presResults {
			if r.Status == "ok" && r.RequestObject != nil {
				tokenOutput := buildRequestObjectOutput(r)
				if err := writeJSON(filepath.Join(outDir, "request-uri-output.json"), tokenOutput); err != nil {
					return err
				}
				break
			}
		}
	}

	// Write extraction-summary.json
	status := "ok"
	var errors []string
	if offerSuccess < len(offerResults) || presSuccess < len(presResults) {
		if offerSuccess == 0 && presSuccess == 0 && (len(offerResults)+len(presResults) > 0) {
			status = "error"
		} else if offerSuccess+presSuccess < len(offerResults)+len(presResults) {
			status = "partial"
		}
	}
	for _, r := range offerResults {
		if r.Error != nil {
			errors = append(errors, r.Error.Error.Message)
		}
	}
	for _, r := range presResults {
		if r.Error != nil {
			errors = append(errors, r.Error.Error.Message)
		}
	}

	summary := ExtractionSummary{
		Status:                          status,
		StartedAt:                       startedAt,
		FinishedAt:                      finishedAt,
		CredentialOfferCount:            len(offerResults),
		CredentialOfferSuccessCount:     offerSuccess,
		PresentationRequestCount:        len(presResults),
		PresentationRequestSuccessCount: presSuccess,
		Errors:                          errors,
	}
	return writeJSON(filepath.Join(outDir, "extraction-summary.json"), summary)
}

func writeCredentialOffer(dir string, step discovery.Step, r *credoffer.Result) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	writeJSON(filepath.Join(dir, "source-step.json"), step)
	writeJSON(filepath.Join(dir, "credential-offer-resolution-chain.json"), r)

	if r.Error != nil {
		writeJSON(filepath.Join(dir, "error.json"), r.Error)
		return nil
	}

	if r.DeeplinkURI != "" {
		os.WriteFile(filepath.Join(dir, "credential-offer-deeplink.txt"), []byte(r.DeeplinkURI), 0644)
	}
	if r.CredentialOffer != nil {
		writeRaw(filepath.Join(dir, "credential-offer.json"), r.CredentialOffer)
	}
	if r.IssuerMetadata != nil {
		writeRaw(filepath.Join(dir, "well-known.json"), r.IssuerMetadata)
	}
	if r.IssuerMetadataFetch != nil {
		writeJSON(filepath.Join(dir, "issuer-metadata-fetch.json"), r.IssuerMetadataFetch)
	}

	return nil
}

func writePresentationRequest(dir string, step discovery.Step, r *presentation.Result) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	writeJSON(filepath.Join(dir, "source-step.json"), step)

	if r.Error != nil {
		writeJSON(filepath.Join(dir, "error.json"), r.Error)
		return nil
	}

	if r.DeeplinkURI != "" {
		os.WriteFile(filepath.Join(dir, "presentation-deeplink.txt"), []byte(r.DeeplinkURI), 0644)
	}
	if r.RequestURIFetch != nil {
		writeJSON(filepath.Join(dir, "request-uri-fetch.json"), r.RequestURIFetch)
	}
	if r.RequestURIRaw != "" {
		os.WriteFile(filepath.Join(dir, "request-uri-raw.jwt"), []byte(r.RequestURIRaw), 0644)
	}
	if r.RequestObject != nil {
		output := buildRequestObjectOutput(r)
		writeJSON(filepath.Join(dir, "request-uri-output.json"), output)
	}

	return nil
}

func buildRequestObjectOutput(r *presentation.Result) map[string]any {
	output := map[string]any{
		"source_request_uri":     r.RequestURI,
		"request_uri_method":     r.RequestURIMethod,
		"post_strategy_selected": r.PostStrategy,
		"format":                 "jwt",
		"raw":                    r.RequestURIRaw,
	}
	if r.RequestObject != nil {
		output["header"] = json.RawMessage(r.RequestObject.Header)
		output["payload"] = json.RawMessage(r.RequestObject.Payload)
		output["signature_present"] = r.RequestObject.SignaturePresent
	}
	return output
}

func sanitizeDirName(s string) string {
	result := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			result = append(result, c)
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func writeRaw(path string, data json.RawMessage) error {
	// Pretty-print if it's valid JSON
	var pretty any
	if err := json.Unmarshal(data, &pretty); err == nil {
		formatted, err := json.MarshalIndent(pretty, "", "  ")
		if err == nil {
			formatted = append(formatted, '\n')
			return os.WriteFile(path, formatted, 0644)
		}
	}
	return os.WriteFile(path, data, 0644)
}
