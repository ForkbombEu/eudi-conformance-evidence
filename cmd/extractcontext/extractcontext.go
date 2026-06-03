// Package extractcontext implements the extract-context CLI command.
package extractcontext

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/credoffer"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/discovery"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/output"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/presentation"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/telemetry"
)

// Options configures an extraction run.
type Options struct {
	CredimiBaseURL string
	Parallelism    int
	Strict         bool
	IDEncoding     string
	MaxDepth       int
	PostStrategy   string
	Timeout        time.Duration
}

// Run executes the extract-context command from CLI args.
func Run(args []string) error {
	fs := flag.NewFlagSet("extract-context", flag.ExitOnError)

	temporalInput := fs.String("temporal-input", "", "Path to the pipeline/workflow input JSON (use '-' for stdin)")
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

	shutdownTelemetry := telemetry.Setup()
	defer shutdownTelemetry()

	if *temporalInput == "" {
		return fmt.Errorf("--temporal-input is required")
	}
	if *outDir == "" {
		return fmt.Errorf("--out-dir is required")
	}

	// Open input (file or stdin)
	var reader io.Reader
	if *temporalInput == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(*temporalInput)
		if err != nil {
			return fmt.Errorf("open temporal input: %w", err)
		}
		defer f.Close() //nolint:errcheck
		reader = f
	}

	opts := Options{
		CredimiBaseURL: *credimiBaseURL,
		Parallelism:    *parallelism,
		Strict:         *strict,
		IDEncoding:     *idEncoding,
		MaxDepth:       *maxDepth,
		PostStrategy:   *postStrategy,
		Timeout:        *timeout,
	}

	startedAt := time.Now().UTC().Format(time.RFC3339)

	client := &http.Client{Timeout: opts.Timeout}

	collected, err := RunExtraction(reader, client, opts)
	if err != nil {
		return err
	}

	collected.Summary.StartedAt = startedAt
	collected.Summary.FinishedAt = time.Now().UTC().Format(time.RFC3339)

	if err := output.WriteToDir(*outDir, collected); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	if opts.Strict && collected.Summary.Status != "ok" {
		return fmt.Errorf("extraction errors occurred in strict mode")
	}

	_ = temporalOutput
	return nil
}

// RunExtraction runs the full extraction pipeline from an io.Reader.
// It returns an in-memory CollectedResult without touching the filesystem —
// suitable for embedding in Temporal activities, HTTP handlers, or tests.
func RunExtraction(r io.Reader, client *http.Client, opts Options) (*output.CollectedResult, error) {
	// Peek at the first bytes to check if it looks like a struct or raw JSON
	disc, err := discovery.DiscoverReader(r)
	if err != nil {
		return nil, fmt.Errorf("step discovery: %w", err)
	}

	// Resolve credential offers in parallel
	offerResults := make([]*credoffer.Result, len(disc.CredentialOfferSteps))
	resolveOffers(client, opts.CredimiBaseURL, disc.CredentialOfferSteps, opts.IDEncoding, opts.MaxDepth, opts.Parallelism, offerResults)

	// Resolve presentation requests in parallel
	presResults := make([]*presentation.Result, len(disc.PresentationRequestSteps))
	resolvePresentations(client, opts.CredimiBaseURL, disc.PresentationRequestSteps, opts.IDEncoding, opts.PostStrategy, opts.Timeout, opts.Parallelism, presResults)

	// Fetch issuer metadata for successful credential offers
	for _, r := range offerResults {
		if r.Status == "ok" && r.CredentialOffer != nil {
			meta, fetch, err := credoffer.FetchIssuerMetadata(client, r.CredentialOffer)
			if err != nil {
				r.IssuerMetadataFetch = fetch
				if opts.Strict {
					r.Status = "error"
					r.Error = credoffer.NewError("issuer_metadata_fetch_failed", err.Error(),
						"Could not fetch issuer metadata.", "", 0, true)
				}
			} else {
				r.IssuerMetadata = meta
				r.IssuerMetadataFetch = fetch
			}
		}
	}

	return output.Collect(disc, offerResults, presResults, "", ""), nil
}

// RunExtractionBytes is like RunExtraction but accepts raw JSON bytes or a pre-parsed struct.
// Pass either raw JSON bytes or nil to skip discovery and use the provided steps directly.
func RunExtractionBytes(input []byte, client *http.Client, opts Options) (*output.CollectedResult, error) {
	return RunExtraction(strings.NewReader(string(input)), client, opts)
}

// RunExtractionSteps runs the pipeline from pre-discovered steps and raw JSON input.
// The raw input is used for discovery metadata; steps drive the extraction.
// Use this when you already have parsed step data (e.g. from a Temporal workflow payload).
func RunExtractionSteps(steps []discovery.Step, client *http.Client, opts Options) (*output.CollectedResult, error) {
	disc := &discovery.Result{}
	for _, s := range steps {
		switch s.Use {
		case "credential-offer":
			disc.CredentialOfferSteps = append(disc.CredentialOfferSteps, s)
		case "use-case-verification-deeplink":
			disc.PresentationRequestSteps = append(disc.PresentationRequestSteps, s)
		}
	}

	offerResults := make([]*credoffer.Result, len(disc.CredentialOfferSteps))
	resolveOffers(client, opts.CredimiBaseURL, disc.CredentialOfferSteps, opts.IDEncoding, opts.MaxDepth, opts.Parallelism, offerResults)

	presResults := make([]*presentation.Result, len(disc.PresentationRequestSteps))
	resolvePresentations(client, opts.CredimiBaseURL, disc.PresentationRequestSteps, opts.IDEncoding, opts.PostStrategy, opts.Timeout, opts.Parallelism, presResults)

	for _, r := range offerResults {
		if r.Status == "ok" && r.CredentialOffer != nil {
			meta, fetch, err := credoffer.FetchIssuerMetadata(client, r.CredentialOffer)
			if err != nil {
				r.IssuerMetadataFetch = fetch
				if opts.Strict {
					r.Status = "error"
					r.Error = credoffer.NewError("issuer_metadata_fetch_failed", err.Error(),
						"Could not fetch issuer metadata.", "", 0, true)
				}
			} else {
				r.IssuerMetadata = meta
				r.IssuerMetadataFetch = fetch
			}
		}
	}

	return output.Collect(disc, offerResults, presResults, "", ""), nil
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
