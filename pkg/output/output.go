// Package output collects and serialises extraction results.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/credoffer"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/discovery"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/presentation"
)

// ExtractionSummary summarises an extraction run.
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

// CollectedResult holds all extraction results in memory without touching the filesystem.
type CollectedResult struct {
	Summary          *ExtractionSummary
	DiscoveredSteps  *discovery.Result
	OfferResults     []*credoffer.Result
	PresResults      []*presentation.Result
	WellKnown        json.RawMessage // convenience alias (single offer)
	RequestURIOutput json.RawMessage // convenience alias (single request)
}

// Collect builds an in-memory CollectedResult from the extraction pipeline output.
// Use this when you want to serialise the result yourself (e.g. Temporal activity, API response).
func Collect(disc *discovery.Result, offerResults []*credoffer.Result, presResults []*presentation.Result,
	startedAt, finishedAt string) *CollectedResult {

	offerSuccess := 0
	for _, r := range offerResults {
		if r.Status == "ok" {
			offerSuccess++
		}
	}
	presSuccess := 0
	for _, r := range presResults {
		if r.Status == "ok" {
			presSuccess++
		}
	}

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

	c := &CollectedResult{
		DiscoveredSteps: disc,
		OfferResults:    offerResults,
		PresResults:     presResults,
		Summary: &ExtractionSummary{
			Status:                          status,
			StartedAt:                       startedAt,
			FinishedAt:                      finishedAt,
			CredentialOfferCount:            len(offerResults),
			CredentialOfferSuccessCount:     offerSuccess,
			PresentationRequestCount:        len(presResults),
			PresentationRequestSuccessCount: presSuccess,
			Errors:                          errors,
		},
	}

	// Convenience aliases for single-success cases
	if offerSuccess == 1 {
		for _, r := range offerResults {
			if r.Status == "ok" && r.IssuerMetadata != nil {
				c.WellKnown = r.IssuerMetadata
				break
			}
		}
	}
	if presSuccess == 1 {
		for _, r := range presResults {
			if r.Status == "ok" && r.RequestObject != nil {
				c.RequestURIOutput, _ = json.Marshal(buildRequestObjectOutput(r))
				break
			}
		}
	}

	return c
}

// WriteJSON writes v as indented JSON to w.
func WriteJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// WriteToDir serialises a CollectedResult to a directory tree.
func WriteToDir(outDir string, c *CollectedResult) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := writeFile(filepath.Join(outDir, "discovered-steps.json"), c.DiscoveredSteps); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(outDir, "extraction-summary.json"), c.Summary); err != nil {
		return err
	}

	offerDir := filepath.Join(outDir, "credential-offers")
	for i, r := range c.OfferResults {
		subDir := fmt.Sprintf("%04d-%s", i, sanitizeDirName(r.StepID))
		if err := writeCredentialOfferDir(filepath.Join(offerDir, subDir), c.DiscoveredSteps.CredentialOfferSteps[i], r); err != nil {
			return err
		}
	}

	presDir := filepath.Join(outDir, "presentation-requests")
	for i, r := range c.PresResults {
		subDir := fmt.Sprintf("%04d-%s", i, sanitizeDirName(r.StepID))
		if err := writePresentationDir(filepath.Join(presDir, subDir), c.DiscoveredSteps.PresentationRequestSteps[i], r); err != nil {
			return err
		}
	}

	if c.WellKnown != nil {
		if err := writeRaw(filepath.Join(outDir, ".well-known.json"), c.WellKnown); err != nil {
			return err
		}
	}
	if c.RequestURIOutput != nil {
		if err := writeRaw(filepath.Join(outDir, "request-uri-output.json"), c.RequestURIOutput); err != nil {
			return err
		}
	}

	return nil
}

// WriteAll is a convenience that collects and then writes to a directory in one call.
func WriteAll(outDir string, disc *discovery.Result, offerResults []*credoffer.Result,
	presResults []*presentation.Result, startedAt, finishedAt string, strict bool) error {
	_ = strict
	c := Collect(disc, offerResults, presResults, startedAt, finishedAt)
	return WriteToDir(outDir, c)
}

func writeCredentialOfferDir(dir string, step discovery.Step, r *credoffer.Result) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "source-step.json"), step); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "credential-offer-resolution-chain.json"), r); err != nil {
		return err
	}
	if r.Error != nil {
		return writeFile(filepath.Join(dir, "error.json"), r.Error)
	}
	if r.DeeplinkURI != "" {
		if err := os.WriteFile(filepath.Join(dir, "credential-offer-deeplink.txt"), []byte(r.DeeplinkURI), 0644); err != nil {
			return err
		}
	}
	if r.CredentialOffer != nil {
		if err := writeRaw(filepath.Join(dir, "credential-offer.json"), r.CredentialOffer); err != nil {
			return err
		}
	}
	if r.IssuerMetadata != nil {
		if err := writeRaw(filepath.Join(dir, "well-known.json"), r.IssuerMetadata); err != nil {
			return err
		}
	}
	if r.IssuerMetadataFetch != nil {
		if err := writeFile(filepath.Join(dir, "issuer-metadata-fetch.json"), r.IssuerMetadataFetch); err != nil {
			return err
		}
	}
	return nil
}

func writePresentationDir(dir string, step discovery.Step, r *presentation.Result) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "source-step.json"), step); err != nil {
		return err
	}
	if r.Error != nil {
		return writeFile(filepath.Join(dir, "error.json"), r.Error)
	}
	if r.DeeplinkURI != "" {
		if err := os.WriteFile(filepath.Join(dir, "presentation-deeplink.txt"), []byte(r.DeeplinkURI), 0644); err != nil {
			return err
		}
	}
	if r.RequestURIFetch != nil {
		if err := writeFile(filepath.Join(dir, "request-uri-fetch.json"), r.RequestURIFetch); err != nil {
			return err
		}
	}
	if r.RequestURIRaw != "" {
		if err := os.WriteFile(filepath.Join(dir, "request-uri-raw.jwt"), []byte(r.RequestURIRaw), 0644); err != nil {
			return err
		}
	}
	if r.RequestObject != nil {
		if err := writeFile(filepath.Join(dir, "request-uri-output.json"), buildRequestObjectOutput(r)); err != nil {
			return err
		}
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

func writeFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func writeRaw(path string, data json.RawMessage) error {
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
