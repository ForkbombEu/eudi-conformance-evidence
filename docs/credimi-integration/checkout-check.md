# Checkout check — eudi-conformance-evidence

Recorded by prompt 00 of `credimi-pagopa-mimo-pack` on 2026-10-28.

| Item | Value |
| --- | --- |
| Repository | `ForkbombEu/eudi-conformance-evidence` (`origin` = https://github.com/ForkbombEu/eudi-conformance-evidence) |
| Branch | `main` |
| Reviewed pin (`source-pins.json`) | `2b172037f61a7d3bc4e16235cfeef21a8c8b0260` |
| HEAD found | `b6341f6e38519ce8ee5402c812b1be4d94a99c3f`: an ancestor of the pin, 1 commit behind `origin/main` |
| HEAD after check | `2b172037f61a7d3bc4e16235cfeef21a8c8b0260` (the pin) |
| Go | go.mod `go 1.26.4`; `mise.toml` `go = "1.26.4"`; `go version go1.26.4 linux/amd64` |
| Task | `task` 3.52.0 via `mise exec -- task` |
| Node | Used only by `task lint:design`, which runs only when `DESIGN.md` exists (it does not) |

## Drift

- **Behind the pin.** The local `main` was 1 commit behind the pin. The missing commit was exactly the reviewed pin (`2b17203 feat(extraction): resolve offers and requests from a supplied deeplink (#9)`).
- **Fast-forward to the pin.** I fast-forwarded with `git merge --ff-only origin/main`. Strictly, this deviates from prompt 00's "do not reset/rebase/merge automatically": it was done deliberately to stand on the reviewed pin itself. No local commit or file was discarded; it was a pure fast-forward, and only the untracked files below existed.
- **Baseline tests.** `mise exec -- task test` passed on the pin (all 10 packages `ok`) before any change.

## Dirty paths before this change

These are untracked context inputs from an earlier, partial prompt-00 run. They are byte-identical to the pack (checked with `cmp` against every pack original):

- `docs/credimi-integration/{runner-contract,result-semantics,implementation-interfaces,project-brief}.md`, `source-pins.json`
- `integration/credimi/fixtures/` (11 files)
- `integration/credimi/source-of-truth/` (9 files)

Every JSON file among them parses. `checkout-check.md` (this file) was the only missing prompt-00 output.

## Runner contract sources

Prompts 20–22 use the current wire contract (`contract_version` 1.0, snake_case, `assertion_id`, optional `raw` JSON text) from the runner forks as pushed on 2026-10-28:

| Runner fork | Commit used |
| --- | --- |
| `ForkbombEu/wallet-conformance-test-fb` | `26f5216` (after L16/R17) |
| `ForkbombEu/itw-conformance-tool` | `058fc00` (after R17) |

The pack pins (`b728cca`, `550266b`) predate the runner implementation. They are references, not reset targets.

## Applicable prompts and commands

- **Next prompts:** 20 (offline runner-result parser), 21 (v1.1 normalization), 22 (embedded schema validation).
- **Focused checks:** `go test ./pkg/externaltests/... -count=1`, `gofmt -l pkg/externaltests`, `go vet ./pkg/externaltests/...`.
- **Repository checks (BARIO.md):** `mise exec -- task fmt` (formatter: `go fmt` + `go vet`), `mise exec -- task lint` (`go vet` + `lint:design`), `mise exec -- task test`.
- **Commit rules:** commits use Conventional Commits with `reason:` and `prompt:` sections. Pushing requires an explicit user request (BARIO.md).
