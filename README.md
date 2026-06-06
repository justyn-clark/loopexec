# loopexec

[![CI](https://github.com/justyn-clark/loopexec/actions/workflows/ci.yml/badge.svg)](https://github.com/justyn-clark/loopexec/actions/workflows/ci.yml)
[![Latest tag](https://img.shields.io/github/v/tag/justyn-clark/loopexec?sort=semver)](https://github.com/justyn-clark/loopexec/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.25.3-00ADD8?logo=go&logoColor=white)](https://go.dev)

`loopexec` is a contract-first CLI for bounded execution loops. It is the runtime companion to SMALL Protocol: SMALL describes state, constraints, plans, progress, and handoff; loopexec drives one bounded execution loop against that state and reports machine-readable outcomes.

The current release is intentionally small. It locks down the command names, JSON shape, and exit-code contract that downstream tools can depend on while the full SMALL-driven loop engine continues to develop.

## Current status

Implemented now:

- Go CLI for `loopexec` in `cmd/loopexec`
- Commands: `init`, `run`, `status`, `check`, `step`
- Global `--json` output mode
- Explicit exit-code contract for success, halt, invariant, workspace, execution, and internal failures
- Contract tests for machine-readable output
- GitHub Actions CI running `gofmt`, `go vet`, and `go test ./...`

Not implemented yet:

- Full SMALL-driven loop execution
- Direct `small` CLI integration inside `cmd/loopexec`
- Real task selection, checkpointing, and handoff orchestration
- Container, Nix, or remote execution substrates
- Multi-worker orchestration

## Install

```bash
go install github.com/justyn-clark/loopexec/cmd/loopexec@latest
```

## Build

From the repo root:

```bash
go build ./cmd/loopexec
```

## Test

```bash
go test ./...
```

CI enforces:

- `gofmt`
- `go vet ./...`
- `go test ./...`

## CLI

The implemented CLI returns deterministic human or JSON output.

### Commands

- `loopexec init`
- `loopexec run`
- `loopexec status`
- `loopexec check`
- `loopexec step`

### Global flag

- `--json` - emit exactly one JSON object to stdout

### Example

```bash
loopexec init
loopexec run --json
loopexec status --run-id local --iteration 1
loopexec check
```

Example JSON response:

```json
{
  "tool": "loopexec",
  "version": "0.1.0-rc1",
  "status": "ok",
  "run_id": "local",
  "iteration": 1,
  "errors": []
}
```

### Exit codes

- `0` success
- `10` halted: success condition met
- `11` halted: blocked
- `12` halted: max iterations reached
- `20` invariant failed
- `30` workspace invalid or missing
- `40` execution failure
- `50` internal error

## Repository layout

```text
cmd/loopexec/             loopexec CLI
docs/architecture.md      planned loop architecture
docs/cli.md               current loopexec command contract
docs/integrations.md      integration guidance for the implemented CLI
docs/loop-contract.md     normative target contract/spec
.github/workflows/ci.yml  fmt/vet/test CI gate
scripts/feeltest.sh       deterministic shell feel test helper
```

## SMALL relationship

This repo is SMALL-governed at the repository level, but the implemented `loopexec` binary does not yet execute the full SMALL loop described in the design docs.

Read the docs in this order:

1. `docs/cli.md` - what the current CLI actually does
2. `docs/integrations.md` - how to integrate with the current implementation
3. `docs/architecture.md` - target architecture and planned boundaries
4. `docs/loop-contract.md` - normative target contract

## Links

- Docs site: https://loopexec.dev
- SMALL Protocol: https://smallprotocol.dev

## License

MIT. See `LICENSE`.
