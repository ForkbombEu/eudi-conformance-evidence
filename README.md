# eudi-conformance-evidence

Extract and preserve protocol context from [Credimi](https://credimi.io) EUDI Wallet interoperability pipeline runs: credential offers, presentation requests, issuer metadata, authorization-server metadata, and JWT/JWS request objects.

## Why this exists

Credimi runs EUDI Wallet, Issuer, and Verifier interoperability pipelines using StepCI, Maestro, and Temporal. Those pipelines produce deeplinks and protocol endpoints whose state can be short-lived: presentation requests can be single-use, issuer sessions can expire, and `.well-known` metadata can change over time.

This tool captures the protocol context at extraction time. It requests fresh Credimi deeplinks, resolves credential-offer and presentation-request chains, fetches issuer and authorization-server metadata, decodes JWT/JWS payloads without verifying signatures, and writes structured JSON that can be committed, diffed, and reused by downstream conformance reporting.

## What it does

```text
Temporal pipeline input
         |
         v
  Step discovery
  (credential-offer + use-case-verification-deeplink)
         |
         v
  Parallel fresh deeplink requests to Credimi
         |
         v
  Resolve credential_offer chains
  Fetch issuer .well-known/openid-credential-issuer
  Discover authorization server from the offer or issuer metadata
  Fetch RFC 8414 metadata with OIDC discovery fallback
  Decode JWT/JWS metadata when returned
         |
         v
  Fetch request_uri by GET or negotiated POST strategy
  Decode JWT request objects and preserve JSON request objects
         |
         v
  Structured per-step output + extraction summary
```

The `extract-context` command discovers `credential-offer` and `use-case-verification-deeplink` steps from known Temporal input shapes, with recursive scanning as a fallback. It resolves each discovered step concurrently, writes per-step artifacts, and marks the run summary as `ok`, `partial`, or `error`.

The `web` command serves a browser interface for one-off extraction. It accepts raw credential offers, Credimi Hub credential URLs, raw presentation requests, and Credimi Hub use-case verification URLs. The production web HTTP client rejects loopback, private, link-local, multicast, and unspecified outbound destinations.

## Deployment

### With mise

```bash
mise use github:forkbombeu/eudi-conformance-evidence@latest
```

### From GitHub Releases

Download the binary for your platform from the latest release:

```bash
curl -fsSL https://github.com/forkbombeu/eudi-conformance-evidence/releases/latest/download/eudi-conformance-evidence-Linux-x86_64 -o eudi-conformance-evidence
chmod +x eudi-conformance-evidence
sudo mv eudi-conformance-evidence /usr/local/bin/
```

### From source

```bash
git clone https://github.com/forkbombeu/eudi-conformance-evidence
cd eudi-conformance-evidence
mise install
task build
```

The source build writes the binary to `./bin/eudi-conformance-evidence`.

Run a pipeline extraction with:

```bash
eudi-conformance-evidence extract-context \
  --temporal-input fixtures/EUDI-iss-ver/input.json \
  --credimi-base-url https://credimi.io \
  --out-dir out/eudi-iss-ver
```

Run the web interface with:

```bash
eudi-conformance-evidence web --addr :8080
```

The executable also supports `version` and `--version`.

## Use via Web GUI

Start the web server:

```bash
eudi-conformance-evidence web --addr :8080
```

Open `http://localhost:8080`.

The browser UI exposes two extractors:

- `issuer-metadata`: accepts a raw OpenID credential offer or a Credimi Hub credential URL, then returns credential issuer metadata and discovered authorization-server metadata.
- `presentation-metadata`: accepts a raw presentation request or a Credimi Hub use-case verification URL, then returns the discovered `dcql_query` or `dcql` object.

The web server exposes:

- `GET /`: extraction UI.
- `POST /extract`: form-encoded extraction endpoint with `kind` and `input` fields.
- `GET /healthz`: plain text health check returning `ok`.
- `GET /static/*`: embedded CSS, JavaScript, and image assets.

A hosted instance is available at `https://capture-issuer-verifier.credimi.io`.

## Use via Curl

The hosted `/extract` endpoint accepts form-encoded `POST` requests. It returns the rendered HTML extraction page, not a JSON API response.

Credential-1, using a Credimi Hub credential URL:

```bash
curl --fail-with-body --silent --show-error \
  --request POST https://capture-issuer-verifier.credimi.io/extract \
  --data-urlencode "kind=issuer-metadata" \
  --data-urlencode "input=https://credimi.io/hub/credentials/forkbomb-bv-andrea/misc-issuer-integration-demo/eudiw-pid-pid-vc-sd-jwt-haip-vci"
```

Credential-2, using an OpenID credential offer URI:

```bash
curl --fail-with-body --silent --show-error \
  --request POST https://capture-issuer-verifier.credimi.io/extract \
  --data-urlencode "kind=issuer-metadata" \
  --data-urlencode "input=openid-credential-offer://?credential_offer_uri=https%3A%2F%2Ffunke.animo.id%2Foid4vci%2F188e2459-6da8-4431-9062-2fcdac274f41%2Foffers%2Fbc1a9b68-7730-4ba3-baee-7fb438404531"
```

Verifier-1, using a Credimi Hub use-case verification URL:

```bash
curl --fail-with-body --silent --show-error \
  --request POST https://capture-issuer-verifier.credimi.io/extract \
  --data-urlencode "kind=presentation-metadata" \
  --data-urlencode "input=https://credimi.io/hub/use_cases_verifications/forkbomb-bv-andrea/misc-verifiers-interop/eudiw-pid-verifier-mdoc"
```

Verifier-2, using an OpenID4VP request URI:

```bash
curl --fail-with-body --silent --show-error \
  --request POST https://capture-issuer-verifier.credimi.io/extract \
  --data-urlencode "kind=presentation-metadata" \
  --data-urlencode "input=openid4vp://?client_id=x509_san_dns%3Afunke.animo.id&request_uri=https%3A%2F%2Ffunke.animo.id%2Foid4vp%2F019368ed-3787-7669-b7f4-8c012238e90d%2Fauthorization-requests%2F1b77be0e-e878-44d2-bf81-a574b84d4c7c"
```

For a local web server, replace the host with `http://localhost:8080/extract`.

## Library usage

Packages under `pkg/` and command packages under `cmd/` can be imported by downstream Go modules.

### Full extraction pipeline

```go
package example

import (
	"net/http"
	"os"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/cmd/extractcontext"
)

func run() error {
	input, err := os.Open("pipeline-input.json")
	if err != nil {
		return err
	}
	defer input.Close()

	client := &http.Client{Timeout: 30 * time.Second}
	result, err := extractcontext.RunExtraction(input, client, extractcontext.Options{
		CredimiBaseURL: "https://credimi.io",
		Parallelism:    8,
		IDEncoding:     "auto",
		MaxDepth:       5,
		PostStrategy:   "auto",
		Timeout:        30 * time.Second,
	})
	if err != nil {
		return err
	}

	_ = result.Summary
	_ = result.DiscoveredSteps
	_ = result.OfferResults
	_ = result.PresResults
	return nil
}
```

### Step discovery

```go
import "github.com/forkbombeu/eudi-conformance-evidence/pkg/discovery"

input, _ := os.ReadFile("pipeline-input.json")
result, err := discovery.Discover(input)
// result.CredentialOfferSteps: []Step with PipelineOrder, StepID, CredentialID
// result.PresentationRequestSteps: []Step with PipelineOrder, StepID, UseCaseID
```

### Credential offer resolution

```go
import (
	"net/http"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/credoffer"
)

client := &http.Client{Timeout: 30 * time.Second}
result := credoffer.Resolve(client, "https://credimi.io", "/org/integration/issuer-id", "auto", 5)
if result.Status != "ok" {
	_ = result.Error
	return
}

metadata, fetch, err := credoffer.FetchIssuerMetadata(client, result.CredentialOffer)
if err != nil {
	return
}

authorizationServers, err := credoffer.FetchAuthorizationServerMetadata(client, result.CredentialOffer, metadata)
_ = fetch
_ = authorizationServers
_ = err
```

`FetchAuthorizationServerMetadata` first uses authorization servers selected by the credential offer grants. If no grant selects one, it uses `authorization_servers` from issuer metadata. If neither exists, it falls back to the credential issuer. For each issuer it tries `/.well-known/oauth-authorization-server` and then `/.well-known/openid-configuration`.

### Presentation request resolution

```go
import (
	"net/http"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/presentation"
)

client := &http.Client{Timeout: 30 * time.Second}
result := presentation.Resolve(client, "https://credimi.io", "/org/verifier/use-case", "auto", "auto", 30*time.Second)
if result.Status != "ok" {
	_ = result.Error
	return
}

_ = result.RequestURI
_ = result.RequestURIMethod
_ = result.PostStrategy
_ = result.RequestObject
_ = result.RequestURIRaw
```

`PostStrategy` can be `auto`, `empty`, `wallet_nonce`, `wallet_metadata_object`, or `wallet_metadata_empty_string`. In `auto` mode the resolver tries those concrete strategies in that order.

### JWT/JWS decoding

```go
import "github.com/forkbombeu/eudi-conformance-evidence/pkg/jwt"

token, err := jwt.Decode(rawJWT)
if err != nil {
	return err
}

_ = token.Header
_ = token.Payload
_ = token.SignaturePresent
_ = token.Raw

if jwt.LooksLikeJWT(someString) {
	// Decode can be attempted without signature verification.
}
```

## Full flags

`extract-context` accepts these flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--temporal-input` | required | Path to pipeline/workflow input JSON. Use `-` to read from stdin. |
| `--temporal-output` | empty | Accepted as an optional diagnostic placeholder. The current command does not read it. |
| `--credimi-base-url` | `https://credimi.io` | Credimi API base URL used for deeplink requests. |
| `--out-dir` | required | Directory where JSON, JWT, and text artifacts are written. |
| `--parallelism` | `8` | Maximum concurrent credential-offer and presentation-request extractions. |
| `--strict` | `false` | Exit non-zero when the collected summary status is not `ok`. Metadata fetch failures are promoted to extraction errors in strict mode. |
| `--id-encoding` | `auto` | ID encoding strategy for Credimi deeplink API calls: `auto`, `url`, or `raw`. `auto` currently URL-encodes IDs. |
| `--credential-offer-uri-max-depth` | `5` | Maximum nested `credential_offer_uri` resolution depth. |
| `--request-uri-post-strategy` | `auto` | POST strategy for `request_uri`: `auto`, `empty`, `wallet_nonce`, `wallet_metadata_object`, or `wallet_metadata_empty_string`. |
| `--timeout` | `30s` | HTTP client timeout. |

The top-level executable accepts:

```text
eudi-conformance-evidence extract-context
eudi-conformance-evidence web
eudi-conformance-evidence version
eudi-conformance-evidence --version
```

The `web` command accepts:

| Flag | Default | Description |
|------|---------|-------------|
| `--addr` | `:8080` | HTTP listen address. |

## Output structure

`extract-context` writes this directory shape:

```text
out/eudi-iss-ver/
├── discovered-steps.json
├── extraction-summary.json
├── .well-known.json              # convenience alias when exactly one offer succeeds
├── request-uri-output.json       # convenience alias when exactly one presentation request succeeds with a JWT request object
├── credential-offers/
│   └── 0000-eudiw-pid-.../
│       ├── source-step.json
│       ├── credential-offer-deeplink.txt
│       ├── credential-offer-resolution-chain.json
│       ├── credential-offer.json
│       ├── well-known.json
│       ├── issuer-metadata-fetch.json
│       ├── authorization-servers.json
│       ├── authorization-server-metadata.json
│       ├── authorization-server-metadata-fetch.json
│       └── error.json
└── presentation-requests/
    └── 0000-eudiw-pid-verifier-.../
        ├── source-step.json
        ├── presentation-deeplink.txt
        ├── request-uri-fetch.json
        ├── request-uri-raw.jwt
        ├── request-uri-output.json
        └── error.json
```

Files are written only when the corresponding data exists. For example, `error.json` is written only for failed step extraction, `authorization-server-metadata.json` is written only when exactly one authorization server is present and metadata was fetched, and `request-uri-output.json` is written only when a JWT request object was decoded.

`extraction-summary.json` contains counts for discovered credential offers and presentation requests, successful counts for each kind, run timestamps, a status, warnings, and error messages.

## Open telemetry

Set `OTEL_EXPORTER_OTLP_ENDPOINT` to export spans to an OTLP HTTP collector:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318/v1/traces"

eudi-conformance-evidence extract-context \
  --temporal-input fixtures/EUDI-iss-ver/input.json \
  --out-dir out/eudi-iss-ver
```

When `OTEL_EXPORTER_OTLP_ENDPOINT` is unset, telemetry setup returns a no-op shutdown function and no exporter is configured.

CLI resolver calls and shared package resolver calls are wrapped in spans for Credimi deeplink fetches, credential-offer URI resolution, issuer metadata fetches, authorization-server metadata discovery, presentation deeplink fetches, and presentation `request_uri` fetches. Each span records `http.method`, `http.url`, and `http.status_code` when available; request errors are recorded on the span.
