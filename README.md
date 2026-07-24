# EUDI Conformance Evidence

## Intro

EUDI Conformance Evidence extracts durable protocol context from credential offers and presentation requests. It addresses short-lived wallet, issuer, and verifier flows by resolving their metadata and request chains into structured evidence suitable for inspection and downstream conformance work.

## Technical specs

This is a reusable Go module with CLI and embedded web adapters. It resolves credential offers, issuer and authorization-server metadata, presentation requests, and JWT/JWS payloads while using bounded HTTP access for the web interface.

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
