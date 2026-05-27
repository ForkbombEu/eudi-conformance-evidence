// Package extractcontext implements the extract-context CLI command.
package extractcontext

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/internal/credoffer"
	"github.com/forkbombeu/eudi-conformance-evidence/internal/discovery"
	"github.com/forkbombeu/eudi-conformance-evidence/internal/output"
	"github.com/forkbombeu/eudi-conformance-evidence/internal/presentation"
)

// Run executes the extract-context command.
func Run(args []string) error {
	fs := flag.NewFlagSet("extract-context", flag.ExitOnError)

	temporalInput := fs.String("temporal-input", "", "Path to the pipeline/workflow input JSON (required)")
	temporalOutput := fs.String("temporal-output", "", "Path to Temporal/pipeline output JSON (optional, diagnostic only)")
	credimiBaseURL := fs.String("credimi-base-url", "https://credimi.io", "Credimi base URL")
	outDir := fs.String("out-dir", "", "Output directory (required)")
	parallelism := fs.Int("parallelism", 8, "Max parallel extractions")
	strict := fs.Bool("strict", false, "Exit non-zero on extraction errors")
	idEncoding := fs.String("id-encoding", "auto", "ID encoding strategy: auto, url, raw")
	maxDepth := fs.Int("credential-offer-uri-max-depth", 5, "Max depth for credential_offer_uri resolution")
	postStrategy := fs.String("request-uri-post-strategy", "auto", "POST strategy for request_uri: auto, empty, wallet_nonce, wallet_metadata_object, wallet_metadata_empty_string")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP request timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Validate required flags
	if *temporalInput == "" {
		return fmt.Errorf("--temporal-input is required")
	}
	if *outDir == "" {
		return fmt.Errorf("--out-dir is required")
	}

	startedAt := time.Now().UTC().Format(time.RFC3339)

	// Read input file
	input, err := os.ReadFile(*temporalInput)
	if err != nil {
		return fmt.Errorf("read temporal input: %w", err)
	}

	if !json.Valid(input) {
		return fmt.Errorf("temporal input is not valid JSON")
	}

	// Discover steps
	disc, err := discovery.Discover(input)
	if err != nil {
		return fmt.Errorf("step discovery: %w", err)
	}

	// Create HTTP client
	client := &http.Client{Timeout: *timeout}

	// Resolve credential offers in parallel
	offerResults := make([]*credoffer.Result, len(disc.CredentialOfferSteps))
	resolveOffers(client, *credimiBaseURL, disc.CredentialOfferSteps, *idEncoding, *maxDepth, *parallelism, offerResults)

	// Resolve presentation requests in parallel
	presResults := make([]*presentation.Result, len(disc.PresentationRequestSteps))
	resolvePresentations(client, *credimiBaseURL, disc.PresentationRequestSteps, *idEncoding, *postStrategy, *timeout, *parallelism, presResults)

	// For successful credential offers, fetch issuer metadata
	for i, r := range offerResults {
		if r.Status == "ok" && r.CredentialOffer != nil {
			meta, fetch, err := credoffer.FetchIssuerMetadata(client, r.CredentialOffer)
			if err != nil {
				// Non-fatal: record but continue
				r.IssuerMetadataFetch = fetch
				if *strict {
					r.Status = "error"
					r.Error = newCredentialError("issuer_metadata_fetch_failed", err.Error(),
						"Could not fetch issuer metadata.", "", 0, true)
				}
			} else {
				r.IssuerMetadata = meta
				r.IssuerMetadataFetch = fetch
			}
		}
		_ = i
	}

	finishedAt := time.Now().UTC().Format(time.RFC3339)

	// Write output
	if err := output.WriteAll(*outDir, disc, offerResults, presResults, startedAt, finishedAt, *strict); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	// Check for strict mode failures
	hasErrors := false
	for _, r := range offerResults {
		if r.Error != nil {
			hasErrors = true
		}
	}
	for _, r := range presResults {
		if r.Error != nil {
			hasErrors = true
		}
	}

	if *strict && hasErrors {
		return fmt.Errorf("extraction errors occurred in strict mode")
	}

	_ = temporalOutput // For future diagnostic use
	return nil
}

func resolveOffers(client *http.Client, baseURL string, steps []discovery.Step, idEncoding string, maxDepth, parallelism int, results []*credoffer.Result) {
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for i, step := range steps {
		wg.Add(1)
		go func(idx int, s discovery.Step) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := credoffer.Resolve(client, baseURL, s.CredentialID, idEncoding, maxDepth)
			r.StepID = s.StepID
			results[idx] = r
		}(i, step)
	}
	wg.Wait()
}

func resolvePresentations(client *http.Client, baseURL string, steps []discovery.Step, idEncoding, postStrategy string, timeout time.Duration, parallelism int, results []*presentation.Result) {
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for i, step := range steps {
		wg.Add(1)
		go func(idx int, s discovery.Step) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := presentation.Resolve(client, baseURL, s.UseCaseID, idEncoding, postStrategy, timeout)
			r.StepID = s.StepID
			results[idx] = r
		}(i, step)
	}
	wg.Wait()
}

func newCredentialError(code, message, humanMessage, url string, httpStatus int, recoverable bool) *credoffer.ExtractionError {
	return credoffer.NewError(code, message, humanMessage, url, httpStatus, recoverable)
}
