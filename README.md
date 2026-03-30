# loopexec

`loopexec` is a Go repo for two related command-line surfaces:

- `loopexec` — a contract-first execution-loop CLI stub with stable command names, JSON output, and explicit exit codes.
- `jcn-worker` — a local JCN worker prototype that deterministically routes a task to a model/machine pair and can execute a local LM Studio chat completion, then write replay artifacts.

This repository is not yet the full production loop engine described in the longer design docs. Today it is a small, test-covered implementation that locks down CLI behavior and captures the first local worker-routing flow.

## Current status

Implemented now:

- Go CLI for `loopexec` in `cmd/loopexec`
- Commands: `init`, `run`, `status`, `check`, `step`
- Global `--json` output mode
- Explicit exit-code contract for success, halt, invariant, workspace, and execution failures
- Contract tests for machine-readable output
- GitHub Actions CI running `gofmt`, `go vet`, and `go test ./...`
- `jcn-worker` prototype in `cmd/jcn-worker`
- Deterministic task routing from:
  - task file
  - router policy JSON
  - model registry JSON
- Local LM Studio chat-completions call via `http://localhost:1234` by default
- Run artifact output under `docs/jcn-agent-stack/runs/`

Not implemented yet:

- Full SMALL-driven loop execution
- Direct `small` CLI integration inside `cmd/loopexec`
- Real task selection/checkpointing/handoff orchestration
- Container/Nix/remote substrates
- Multi-worker orchestration in this repo

## Repository layout

```text
cmd/loopexec/                  loopexec CLI
cmd/jcn-worker/                local worker prototype
docs/architecture.md           planned loop architecture
docs/cli.md                    current loopexec command contract
docs/integrations.md           current integration guidance for the implemented CLI
docs/loop-contract.md          normative target contract/spec
docs/jcn-agent-stack/          JCN worker architecture, examples, and replay artifacts
.github/workflows/ci.yml       fmt/vet/test CI gate
scripts/feeltest.sh            deterministic shell feel test helper
```

## Build

From repo root:

```bash
go build ./cmd/loopexec
go build ./cmd/jcn-worker
```

Or install the main CLI:

```bash
go install ./cmd/loopexec
```

## Test

```bash
go test ./...
```

CI currently enforces:

- `gofmt`
- `go vet ./...`
- `go test ./...`

## loopexec CLI

The implemented `loopexec` binary is a contract stub that returns deterministic human or JSON output.

### Commands

- `loopexec init`
- `loopexec run`
- `loopexec status`
- `loopexec check`
- `loopexec step`

### Global flag

- `--json` — emit exactly one JSON object to stdout

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

## jcn-worker

`jcn-worker` is an experimental local worker runtime for the JCN stack.

Implemented subcommands:

- `jcn-worker version`
- `jcn-worker list`
- `jcn-worker status`
- `jcn-worker run <taskPath> [--policy <path>] [--registry <path>]`

Current built-in worker inventory returned by `jcn-worker list`:

- `code-worker`
- `docs-worker`
- `infra-worker`
- `mindrail-worker`
- `reaper-worker`

### Example run

```bash
JCN_LMSTUDIO_BASE_URL=http://localhost:1234 \
  go run ./cmd/jcn-worker run docs/jcn-agent-stack/worker-task.example.json
```

Successful runs write:

- `docs/jcn-agent-stack/runs/<runId>.json`
- `docs/jcn-agent-stack/runs/<runId>.txt`

These artifacts record hashes, selected model, selected machine target, timestamps, and transcript text.

## SMALL relationship

This repo is SMALL-governed at the repository level, but the implemented `loopexec` binary does not yet execute the full SMALL loop described in the design docs.

Read the docs in this order:

1. `docs/cli.md` — what the current CLI actually does
2. `docs/integrations.md` — how to integrate with the current implementation
3. `docs/architecture.md` — target architecture and planned boundaries
4. `docs/loop-contract.md` — normative target contract
5. `docs/jcn-agent-stack/` — JCN worker architecture and local worker prototype artifacts

## Notes on planned docs

Some documents in this repo intentionally describe the target architecture rather than the current implementation. Where that is the case, they are marked as planned or normative.
