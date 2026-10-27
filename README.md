# EUDI Conformance Evidence

## Intro

EUDI Conformance Evidence extracts durable protocol context from credential offers and presentation requests. It addresses short-lived wallet, issuer, and verifier flows by resolving their metadata and request chains into structured evidence suitable for inspection and downstream conformance work.

## Technical specs

This is a reusable Go module with CLI and embedded web adapters. It resolves credential offers, issuer and authorization-server metadata, presentation requests, and JWT/JWS payloads while using bounded HTTP access for the web interface.

As a library, `credoffer.ResolveDeeplink` and `presentation.ResolveDeeplink` resolve a credential or verification deeplink the caller already holds; the library never contacts Credimi. Only the CLI and web UI fetch deeplinks from a Credimi instance (`cmd/internal/credimi`), because they run outside Credimi and receive record IDs or Credimi URLs.

`pkg/externaltests/runner` parses PagoPA runner result envelopes (contract 1.0, engines `pagopa-itwct` and `pagopa-wct`) offline. `runner.Parse` takes bytes the caller already collected, bounded to 16 MiB. It validates identity, selection, statuses and counts, and keeps the exact source bytes so they can be hashed. It performs no network or process I/O, and a valid cancelled or error envelope is incomplete evidence, not a parse error.

`externaltests.Normalize` turns a parsed result plus the caller's real provenance into a Credimi conformance result document (schema v1.1). The caller supplies the workflow IDs, plus stored references and hashes for the raw result and any collected artifacts. The document uses the existing `third-party-conformance-ingestion` profile. Its `external-conformance-test-result-ingested` claim is `pass` when ingestion succeeded, while `run.overall_result` carries the external suite's own outcome, so an ingested failed suite stays `fail`. Native assertions, duplicate and unmapped requirement IDs, and scenario outcomes are kept in `observed_outputs`. No normative references are invented. Every document is validated against the embedded v1.1 schema (`santhosh-tekuri/jsonschema` v6, no remote schema loading) before `Normalize` returns it; `ValidateNormalized` and `ValidateDocument` re-check stored documents. The Credimi call sequence is in `docs/credimi-integration/evidence-importer.md`.

Credimi integration inputs live in `docs/credimi-integration/` (runner contract, result semantics, checkout check) and `integration/credimi/` (runner fixtures, plus the Credimi v1.1 source-of-truth schema and registries). These files are test and documentation inputs, not application configuration.

## HOW to run

```sh
go test ./...
go build -o bin/eudi-conformance-evidence .
./bin/eudi-conformance-evidence web --addr :8080
```

Open `http://127.0.0.1:8080`. The API documentation is at `/docs` and its OpenAPI 3.1 document is `/openapi.yaml`.

## Quick GUI guide

### Issuer metadata

Paste an OpenID credential offer or a Credimi credential URL. The page resolves issuer and authorization-server metadata and provides JSON output for copying.

### Presentation metadata

Paste a presentation request or Credimi verification URL. The page resolves the request and exposes its DCQL metadata and resolution details.

## CLI Examples

| Function | Example |
| --- | --- |
| Extract pipeline context | `./bin/eudi-conformance-evidence extract-context --temporal-input fixtures/EUDI-iss-ver/input.json --credimi-base-url https://credimi.io --out-dir out/eudi-iss-ver` |
| Run the browser UI | `./bin/eudi-conformance-evidence web --addr :8080` |
| Print version | `./bin/eudi-conformance-evidence version` |

## API Examples

| Function | Example |
| --- | --- |
| Health | `curl http://127.0.0.1:8080/healthz` |
| Extract issuer metadata | `curl -X POST http://127.0.0.1:8080/extract --data-urlencode 'kind=issuer-metadata' --data-urlencode 'input=openid-credential-offer://?credential_offer=...'` |
| Extract presentation metadata | `curl -X POST http://127.0.0.1:8080/extract --data-urlencode 'kind=presentation-metadata' --data-urlencode 'input=openid4vp://?request_uri=...'` |

`/extract` is an HTML form route; it is not a JSON API.
