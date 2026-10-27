# Evidence importer: offline runner-result import for Credimi

Credimi calls the importer as a Go module. It is not an HTTP service, and the `/extract` HTML route is unchanged.

## Flow

1. **Credimi collects** (outside this module). A Credimi activity downloads the runner result and any manifest artifacts from the runner API. It stores the exact bytes and records each stored reference with the SHA-256 of those bytes.
2. **Parse.** `runner.Parse(reader)` validates the envelope offline: contract 1.0, engine `pagopa-itwct` or `pagopa-wct`, 16 MiB limit, required identity, selection, statuses and counts, and no `pass` that contradicts the evidence. It keeps the exact bytes in `result.Raw`. A valid cancelled, timed-out or error envelope parses successfully; `result.Incomplete()` reports it.
3. **Normalize.** `externaltests.Normalize(result, ctx)` builds the Credimi conformance result (schema v1.1) from the parsed result and the caller's real provenance. It validates the document against the embedded schema before returning it.
4. **Validate stored documents.** `externaltests.ValidateNormalized(doc)` (or `ValidateDocument(bytes)` for stored JSON) re-checks a document against the embedded schema. Schema validity proves shape only, not truth or certification.

The module performs no network or process I/O. It never fetches artifact URLs, `request_uri` values or the public schema `$id`, and it never calls Credimi, the runner or the system under test.

## Ingestion claim vs external result

These are two different results, and both are kept:

- **The ingestion claim.** `evidence_claim_results[0]` is the existing `external-conformance-test-result-ingested` claim (`credimi.evidence.external.conformance_result_ingested_and_mapped.v1`) of the existing `third-party-conformance-ingestion` profile. It is `pass` when the external result was parsed and imported. It says nothing about whether the system under test conforms.
- **The external result.** `run.overall_result` and the top-level `overall_result` carry the external suite's own outcome, unchanged: `pass`, `fail`, `inconclusive`, `partial` or `not_tested`. An ingested failed suite is therefore *claim pass plus run fail*.

**Where the native data goes.** Native assertions (including duplicate and unknown requirement IDs), scenario outcomes, sessions, counts, selection and coverage mode are in the claim's `observed_outputs`. `mapping_status` is `unmapped`, and no `official_references` are added: no reviewed per-assertion mapping exists yet. Engine provenance (engine ID, version, revision, upstream and fork repositories, native suite status, coverage mode) is in `environment.metadata`. The target is the actual wallet, issuer or verifier; `external_suite_name`/`external_suite_version` identify the PagoPA engine.

**Limitations and references.** The profile title reuses the existing third-party ingestion profile. Results are identified as PagoPA engine results, which are not OpenID Foundation certification tests or official IPZS validations. Generic legacy runs (`coverage_mode` `interoperability`) carry a limitation stating that no Italian assertion matrix was applied.

## Go example

```go
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/externaltests"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/externaltests/runner"
)

// stored holds bytes a Credimi activity already collected and stored.
func importRunnerResult(resultBytes, nativeBytes []byte, workflowID, workflowRunID string) (*externaltests.ConformanceResult, error) {
	result, err := runner.Parse(bytes.NewReader(resultBytes))
	if err != nil {
		return nil, err // malformed or inconsistent envelope; keep the raw bytes for review
	}

	nativeSum := sha256.Sum256(nativeBytes)
	ctx := externaltests.NormalizationContext{
		WorkflowID:    workflowID,    // real Temporal workflow ID
		WorkflowRunID: workflowRunID, // real Temporal run ID
		RawResult: externaltests.StoredRef{
			Ref:    "artifact:" + workflowRunID + "/runner-result.json", // reference returned by storage
			SHA256: result.SHA256(), // equals the hash of the stored bytes
		},
		Artifacts: []externaltests.StoredArtifact{{
			ArtifactID: result.Artifacts[0].ArtifactID, // from the runner manifest
			StoredRef: externaltests.StoredRef{
				Ref:    "artifact:" + workflowRunID + "/native-results.json",
				SHA256: hex.EncodeToString(nativeSum[:]), // must equal the manifest hash
			},
		}},
	}
	return externaltests.Normalize(result, ctx) // validated against schema v1.1
}
```

`Normalize` refuses missing workflow identity or raw reference, a raw hash that does not match the parsed bytes, an artifact that is not in the runner manifest, an artifact hash that differs from the manifest, and any document the schema rejects.

## Schema

`pkg/externaltests/schemas/credimi-conformance-result.schema.v1.1.json` is the supplied source-of-truth schema, copied unchanged and embedded with `go:embed`. A test pins its SHA-256 (`813b9c9a…bfbf55`) and checks it is byte-identical to `integration/credimi/source-of-truth/`.

It is compiled with `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3:
- under the local ID `embedded:///credimi-conformance-result.schema.v1.1.json`;
- with `AssertFormat()` enabled, so date-time formats are checked;
- with a loader that refuses every external reference.

Violations are returned as a `SchemaError` with at most 20 bounded path diagnostics.

## Pinning in Credimi

Credimi pins this module through its own `go.mod`, using the reviewed revision once these commits are reviewed and pushed. Use `go get github.com/forkbombeu/eudi-conformance-evidence@<reviewed commit>`, then `go mod tidy` in the Credimi checkout. No tag or pseudo-version is published by this work; take the exact commit from the reviewed main branch.

## Test evidence

- **Pack fixtures** are synthetic: `pkg/externaltests/runner/testdata/*.json`.
- **Captured envelopes** come from the compiled runners on local harmless fixtures: `pkg/externaltests/runner/testdata/captured/{wct,itwct}`. Each set has a `manifest.json` recording the runner commit, Node version, command, request and per-file SHA-256.
  - The legacy WCT captures cover pass, fail, a passing check with a failing teardown (`partial`), a collection error, cancellation and a deadline.
  - The ITWCT captures cover WP_RP_HAPPY without a wallet (`inconclusive` with 9 failed strict-PASS matchers), cancellation, a deadline, a native port conflict and an immediate cancellation.
  - They are fixture evidence; no live wallet or issuer run (live SUT) is included.
- **Golden v1.1 documents** are in `pkg/externaltests/testdata/golden/`.
