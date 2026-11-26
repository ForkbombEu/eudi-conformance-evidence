# Project Specifications

## Selected backend and rationale

Go. EUDI Conformance Evidence is a reusable Go module whose CLI and web
interface are thin adapters over the `pkg/` library; the stack predates this
document and stays Go.

## Language and runtime versions

Declared in `mise.toml`:

| Tool | Version | Purpose |
| --- | --- | --- |
| Go | 1.26.4 | Module, CLI, web server, tests |
| Task | latest | `Taskfile.yml` runner |
| Node.js | latest | `npx @google/design.md lint DESIGN.md` only |

## Module or package identity

`github.com/forkbombeu/eudi-conformance-evidence`

## Frameworks and dependencies

- Standard library `net/http`, `html/template` and `embed` for the web
  interface; no web framework.
- `github.com/santhosh-tekuri/jsonschema/v6` validates normalized conformance
  documents against the embedded Credimi v1.1 schema.
- `go.opentelemetry.io/otel` with the OTLP HTTP trace exporter
  (`pkg/telemetry`).
- Stoplight Elements 9.0.0, loaded from `unpkg.com` by the `/docs` page only.

## Application architecture

- `pkg/` holds the library: `credoffer`, `presentation`, `discovery`, `jwt`,
  `output`, `externaltests` and `telemetry`. It never contacts Credimi.
- `cmd/extractcontext` and `cmd/webui` are adapters over `pkg/`.
  `cmd/internal/credimi` resolves Credimi Hub record URLs to deeplinks for
  those adapters only.
- `main.go` dispatches the CLI subcommands.

## CLI, web, and API interfaces

CLI subcommands: `extract-context`, `web`, `version`.

Web routes served by `eudi-conformance-evidence web`:

| Route | Purpose |
| --- | --- |
| `GET /` | Extractor UI (issuer and verifier tabs) |
| `POST /extract` | HTML form route rendering extraction results; not a JSON API |
| `GET /docs` | Stoplight Elements reference for the OpenAPI document |
| `GET /openapi.yaml` | OpenAPI 3.1 document |
| `GET /healthz` | Liveness check |
| `GET /static/…` | Embedded assets |

## Configuration

- `web --addr` sets the listen address (default `:8080`).
- `OTEL_EXPORTER_OTLP_ENDPOINT` enables trace export when set.
- `extract-context` flags are listed by `eudi-conformance-evidence extract-context -h`.

## Build, format, lint, and test commands

| Command | Runs |
| --- | --- |
| `mise run dev` | Web UI on `:8080` |
| `mise run fmt` | `go fmt ./...`, `go vet ./...` |
| `mise run lint` | `go vet ./...`, `npx --yes @google/design.md lint DESIGN.md` |
| `mise run test` | `go test ./...` |
| `mise run coverage` | Tests with the coverage threshold (`COVERAGE_MIN`, default 80) |
| `mise run build` | `bin/eudi-conformance-evidence` |

## Production and Docker execution

Releases are built by GoReleaser (`.goreleaser.yaml`, `.github/workflows/release.yml`)
as static (`CGO_ENABLED=0`) binaries for Linux, macOS and Windows on amd64 and
arm64. There is no Docker image.

## Important directories

| Path | Contents |
| --- | --- |
| `brand/` | Canonical Credimi Extras design assets, vendored from `ForkbombEu/credimi-extras-template` |
| `cmd/webui/static/` | Embedded runtime copies of the brand assets, plus `app.css`, `app.js`, `openapi.yaml` |
| `cmd/webui/templates/` | Page templates; `chrome.html` holds the shared banner, topbar and footer |
| `docs/credimi-integration/` | Runner contract, result semantics, checkout check |
| `integration/credimi/` | Runner fixtures and the Credimi v1.1 source-of-truth schema and registries |
| `fixtures/` | Pipeline inputs for `extract-context` |
| `HITL/` | Human-supplied briefs and the original brand inputs |

## Generated files

None are committed. `bin/` is a build output.

## Repository-specific constraints

### Design assets

`DESIGN.md` is the design source of truth. `brand/` holds the canonical
assets; the web interface embeds byte-identical runtime copies, enforced by
`TestRuntimeBrandAssetsMatchCanonicalSources`. The browser loads `style.css`
before `app.css`, and `fonts/` stays a sibling of `style.css` so its relative
`url()` resolves.

| Canonical asset | Runtime copy | SHA-256 |
| --- | --- | --- |
| `brand/style.css` | `cmd/webui/static/style.css` | `bcf791121cf504add3f59ab3139aebd226c10a8aa42a718deab4ffcb41c12469` |
| `brand/logos/credimi_logo.svg` | `cmd/webui/static/credimi_logo.svg` | `031885760a9165e9d8d49eab45baca30ba5ed8dd1fbf0b4699fba2de5dc4feac` |
| `brand/logos/credimi_logo_negative.svg` | `cmd/webui/static/credimi_logo_negative.svg` | `32df33f9f5ffa696d452e1f65f5d6738b920415c5114db4b010af1f997a8cb3a` |
| `brand/logos/credimi_logo-transp.svg` | `cmd/webui/static/credimi_logo-transp.svg` | `8407a3ed0beddc137599f71498f1ca8e68766a2684dba80093ea25e17173eef7` |
| `brand/logos/credimi_logo-transp_white.svg` | `cmd/webui/static/credimi_logo-transp_white.svg` | `196017744fca7d3836720995aca8c55c8531908e8dcdafc5ca5e7b48ef8f7e10` |
| `brand/fonts/InterVariable.ttf` | `cmd/webui/static/fonts/InterVariable.ttf` | `29160a80ff49ddcab2c97711247e08b1fab27a484a329ce8b813d820dc559031` |
| `brand/fonts/SourceCodeProVariable.ttf` | `cmd/webui/static/fonts/SourceCodeProVariable.ttf` | `b400fc584e10aff25d0e775ce181b4fc1c5ea1b5dc37b81aeb2084375b945790` |

Logo use: the dark wordmark identifies the topbar; the mark is the favicon
(`/static/credimi_logo.svg`) and the top Credimi Extras strip's glyph; the
negative mark sits in the indigo footer; the white wordmark closes the footer's
Credimi Extras strip. To move the brand, replace the `brand/` asset and its
runtime copy together.

### Web security

Pages are served with a restrictive Content-Security-Policy; only `/docs` allows
`unsafe-inline` styles, which Stoplight Elements needs. Outbound fetches from the
web interface go through a client that refuses non-public addresses.
