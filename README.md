# eudi-conformance-evidence

Extract and preserve protocol context from [Credimi](https://credimi.io) EUDI Wallet interoperability pipeline runs — credential offers, presentation requests, issuer metadata, and JWT/JWS request objects — for downstream conformance reporting.

## Why this exists

Credimi runs EUDI Wallet / Issuer / Verifier interop pipelines using StepCI, Maestro, and Temporal. Each pipeline produces deeplinks (credential offers, presentation requests) that are **ephemeral**: presentation requests are single-use, issuer sessions expire, and `.well-known` endpoints change.

This tool captures the protocol context **at extraction time** by requesting fresh deeplinks from Credimi and resolving the full chain — credential offer URIs, issuer metadata, request objects — into structured, versionable JSON. The output feeds into conformance taxonomy matching and evidence registry generation (coming later).

## What it does

```
Temporal pipeline input
         │
         ▼
  Step discovery
  (credential-offer + use-case-verification-deeplink)
         │
         ▼
  Parallel fresh deeplink requests to Credimi
         │
         ▼
  Resolve credential_offer chains (recursive credential_offer_uri)
  Fetch issuer .well-known/openid-credential-issuer
  Decode JWT/JWS metadata (OpenID Federation)
         │
         ▼
  Fetch request_uri (GET + POST strategy auto-negotiation)
  Decode JWT request objects (DCQL, presentation definitions)
         │
         ▼
  Structured per-step output + extraction summary
```

## Installation

### With mise (recommended)

```bash
mise use github:forkbombeu/eudi-conformance-evidence@latest
```

### From GitHub Releases

```bash
# Download the binary for your platform from the latest release:
# https://github.com/forkbombeu/eudi-conformance-evidence/releases/latest

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
# binary at ./bin/eudi-conformance-evidence
```

## CLI usage

### extract-context

```bash
eudi-conformance-evidence extract-context \
  --temporal-input fixtures/EUDI-iss-ver/input.json \
  --credimi-base-url https://credimi.io \
  --out-dir out/eudi-iss-ver
```

### Full flags

| Flag | Default | Description |
|------|---------|-------------|
| `--temporal-input` | *(required)* | Path to pipeline/workflow input JSON |
| `--temporal-output` | — | Path to Temporal output JSON (diagnostic only) |
| `--credimi-base-url` | `https://credimi.io` | Credimi API base URL |
| `--out-dir` | *(required)* | Output directory |
| `--parallelism` | `8` | Max concurrent extractions |
| `--strict` | `false` | Exit non-zero on any extraction error |
| `--id-encoding` | `auto` | ID encoding: `auto`, `url`, `raw` |
| `--credential-offer-uri-max-depth` | `5` | Max resolution depth for nested offer URIs |
| `--request-uri-post-strategy` | `auto` | POST strategy: `auto`, `empty`, `wallet_nonce`, `wallet_metadata_object`, `wallet_metadata_empty_string` |
| `--timeout` | `30s` | HTTP request timeout |

### Output structure

```
out/eudi-iss-ver/
├── discovered-steps.json
├── extraction-summary.json
├── .well-known.json              # convenience alias (single offer)
├── request-uri-output.json       # convenience alias (single request)
├── credential-offers/
│   └── 0000-eudiw-pid-.../
│       ├── source-step.json
│       ├── credential-offer-deeplink.txt
│       ├── credential-offer-resolution-chain.json
│       ├── credential-offer.json
│       ├── well-known.json
│       ├── issuer-metadata-fetch.json
│       └── error.json            # only on failure
└── presentation-requests/
    └── 0000-eudiw-pid-verifier-.../
        ├── source-step.json
        ├── presentation-deeplink.txt
        ├── request-uri-fetch.json
        ├── request-uri-raw.jwt
        ├── request-uri-output.json
        └── error.json            # only on failure
```

## OpenTelemetry

Set `OTEL_EXPORTER_OTLP_ENDPOINT` to export spans to an OTLP collector:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318/v1/traces"

eudi-conformance-evidence extract-context \
  --temporal-input fixtures/EUDI-iss-ver/input.json \
  --out-dir out/
```

Every HTTP call — Credimi deeplink fetches, credential offer URI resolution, issuer metadata, and request URI POST negotiation — produces a span with `http.method`, `http.url`, and `http.status_code` attributes. When the env var is unset, tracing is a zero-overhead no-op.

## Library usage

Packages under `pkg/` are importable by downstream Go modules.

### Step discovery

```go
import "github.com/forkbombeu/eudi-conformance-evidence/pkg/discovery"

input, _ := os.ReadFile("pipeline-input.json")
result, err := discovery.Discover(input)
// result.CredentialOfferSteps — []Step with PipelineOrder, StepID, CredentialID
// result.PresentationRequestSteps — []Step with PipelineOrder, StepID, UseCaseID
```

### Credential offer resolution

```go
import (
    "net/http"
    "github.com/forkbombeu/eudi-conformance-evidence/pkg/credoffer"
)

client := &http.Client{Timeout: 30 * time.Second}
result := credoffer.Resolve(client, "https://credimi.io", "/org/integration/issuer-id", "auto", 5)
// result.CredentialOffer — the final credential offer JSON
// result.ResolutionChain — each step in the resolution chain
// result.DeeplinkURI — the raw deeplink returned by Credimi

// Fetch issuer metadata
meta, fetch, err := credoffer.FetchIssuerMetadata(client, result.CredentialOffer)
// meta — .well-known/openid-credential-issuer (JSON or decoded JWT)
// fetch — metadata about the HTTP request
```

### Presentation request resolution

```go
import "github.com/forkbombeu/eudi-conformance-evidence/pkg/presentation"

result := presentation.Resolve(client, "https://credimi.io", "/org/verifier/use-case", "auto", "auto", 30*time.Second)
// result.RequestURI — the extracted request_uri
// result.RequestURIMethod — "get" or "post"
// result.PostStrategy — which strategy succeeded (for POST)
// result.RequestObject — decoded JWT with Header, Payload, SignaturePresent
// result.RequestURIRaw — raw JWT string
```

### JWT/JWS decoding (no verification)

```go
import "github.com/forkbombeu/eudi-conformance-evidence/pkg/jwt"

token, err := jwt.Decode(rawJWT)
// token.Header — json.RawMessage
// token.Payload — json.RawMessage
// token.SignaturePresent — bool
// token.Raw — preserved original string

if jwt.LooksLikeJWT(someString) {
    // ...
}
```

### Complete extraction pipeline

```go
package main

import (
    "net/http"
    "os"
    "time"

    "github.com/forkbombeu/eudi-conformance-evidence/pkg/credoffer"
    "github.com/forkbombeu/eudi-conformance-evidence/pkg/discovery"
    "github.com/forkbombeu/eudi-conformance-evidence/pkg/output"
    "github.com/forkbombeu/eudi-conformance-evidence/pkg/presentation"
    "github.com/forkbombeu/eudi-conformance-evidence/pkg/telemetry"
)

func main() {
    defer telemetry.Setup()()

    input, _ := os.ReadFile("pipeline-input.json")
    disc, _ := discovery.Discover(input)

    client := &http.Client{Timeout: 30 * time.Second}

    // Resolve credential offers
    var offerResults []*credoffer.Result
    for _, step := range disc.CredentialOfferSteps {
        r := credoffer.Resolve(client, "https://credimi.io", step.CredentialID, "auto", 5)
        r.StepID = step.StepID
        if r.Status == "ok" && r.CredentialOffer != nil {
            meta, fetch, _ := credoffer.FetchIssuerMetadata(client, r.CredentialOffer)
            r.IssuerMetadata = meta
            r.IssuerMetadataFetch = fetch
        }
        offerResults = append(offerResults, r)
    }

    // Resolve presentation requests
    var presResults []*presentation.Result
    for _, step := range disc.PresentationRequestSteps {
        r := presentation.Resolve(client, "https://credimi.io", step.UseCaseID, "auto", "auto", 30*time.Second)
        r.StepID = step.StepID
        presResults = append(presResults, r)
    }

    // Write output
    output.WriteAll("out/", disc, offerResults, presResults,
        time.Now().UTC().Format(time.RFC3339),
        time.Now().UTC().Format(time.RFC3339),
        false,
    )
}
```

## Development

```bash
git clone https://github.com/forkbombeu/eudi-conformance-evidence
cd eudi-conformance-evidence
mise install
task test
task lint
task build
```

### Project conventions

See `PURIA.md` for the full engineering doctrine. Key points:

- Go standard library preferred; add dependencies only with clear justification
- Every package has tests with mocked external dependencies
- Commits follow [Conventional Commits](https://www.conventionalcommits.org/) with `reason` and `prompt` trailers
- `mise.toml` declares all required tools
- `Taskfile.yml` defines `test`, `lint`, `lint:design`, `fmt`, `run`, `build`

### Test fixtures

The `fixtures/` directory contains real (anonymized) Credimi/Temporal pipeline inputs and outputs:

| Fixture | Steps |
|---------|-------|
| `EUDI-iss-ver/` | 1 credential-offer + 1 verification-deeplink |
| `EUDI-iss2/` | 1 credential-offer (nested `with.payload`) |
| `Multipaz/` | 1 credential-offer |
| `Talao-iss-cred13/` | 1 credential-offer |
| `AgeVerification/` | 0 extraction steps (mobile-automation only) |

Tests use these fixtures and local `httptest` servers — they never hit real endpoints.

## Versioning

This project uses [go-semantic-release](https://github.com/go-semantic-release/semantic-release) for automated semver based on conventional commit messages.

```bash
eudi-conformance-evidence version
# dev (from source)
# v1.0.0 (from release binary)
```

## License

Apache 2.0
