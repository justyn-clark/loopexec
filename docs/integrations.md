# Integrations

This document describes integration patterns for the code that is implemented in this repository today.

> Note: `docs/architecture.md` and `docs/loop-contract.md` describe the target loop design. This file stays scoped to the current CLI surface.

---

## loopexec integration surface

The current `loopexec` binary exposes these commands:

- `loopexec init`
- `loopexec run`
- `loopexec status`
- `loopexec check`
- `loopexec step`

There is no `emit` command in the current implementation.

### Machine-readable output

All implemented commands support the global `--json` flag.

Example:

```bash
loopexec run --json
```

Example response:

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

### JSON fields

All JSON responses use this shape:

- `tool` (string)
- `version` (string)
- `status` (string)
- `run_id` (string, optional)
- `iteration` (integer, optional)
- `halt_reason` (string, optional)
- `errors` (array of strings)

Human-readable output goes to stdout. Error lines are printed to stderr.

---

## Exit codes

`loopexec` currently uses these exit codes:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 10 | Halted: success condition met |
| 11 | Halted: blocked |
| 12 | Halted: max iterations reached |
| 20 | Invariant failed |
| 30 | Workspace invalid or missing |
| 40 | Execution failure |
| 50 | Internal error |

Example shell handling:

```bash
loopexec run --json
case $? in
  0)  echo "success" ;;
  10) echo "halted: success condition met" ;;
  11) echo "halted: blocked" ;;
  12) echo "halted: max iterations reached" ;;
  20) echo "invariant failed" ;;
  30) echo "workspace invalid or missing" ;;
  40) echo "execution failure" ;;
  50) echo "internal error" ;;
  *)  echo "unexpected exit code" ;;
esac
```

---

## Bash usage

### Initialize local metadata

```bash
loopexec init
```

This creates `.loopexec/` in the current working directory.

### Run a stub iteration

```bash
loopexec run --run-id local --max-iterations 1 --json
```

### Force known halt paths for wrappers and tests

```bash
loopexec run --json --halt-reason success
loopexec run --json --halt-reason blocked
loopexec run --json --halt-reason max-iterations
loopexec run --json --halt-reason exec-fail
```

### Read status

```bash
loopexec status --run-id local --iteration 1 --json
```

### Force an invariant failure

```bash
loopexec check --json --fail-invariant
```

---

## CI integration

This repository's own CI lives in `.github/workflows/ci.yml` and currently checks:

- `gofmt`
- `go vet ./...`
- `go test ./...`

A minimal external CI integration for the current repo can use the same pattern:

```yaml
name: loopexec CI
on: [push, pull_request]

jobs:
  go-ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Format check
        run: |
          files=$(find . -type f -name '*.go' -not -path './vendor/*')
          out=$(gofmt -l $files)
          test -z "$out"

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test ./...
```

---

## jcn-worker integration surface

`jcn-worker` is a separate local-worker prototype in this repo.

Implemented commands:

- `jcn-worker version`
- `jcn-worker list`
- `jcn-worker status`
- `jcn-worker run <taskPath> [--policy <path>] [--registry <path>]`

### Default paths

If the task file does not override them, `jcn-worker run` uses:

- router policy: `docs/jcn-agent-stack/router-policy.example.json`
- model registry: `docs/jcn-agent-stack/model-registry.example.json`
- LM Studio base URL: `http://localhost:1234`

You can override the base URL with:

```bash
export JCN_LMSTUDIO_BASE_URL=http://localhost:1234
```

### Example run

```bash
go run ./cmd/jcn-worker run docs/jcn-agent-stack/worker-task.example.json
```

Successful output includes:

- `selected_model_id: ...`
- `selected_machine_target: ...`
- model response text

Successful and failed runs both attempt to write replay artifacts under:

- `docs/jcn-agent-stack/runs/<runId>.json`
- `docs/jcn-agent-stack/runs/<runId>.txt`

The JSON run record includes task, policy, and registry hashes plus timestamps and selected route data.

---

## Deferred integrations

The following are still design-stage in this repo and should not be treated as implemented behavior:

- direct SMALL-driven loop execution from `cmd/loopexec`
- `loopexec emit`
- automatic task selection from `.small/`
- checkpoint and handoff execution by `loopexec`
- container/Nix/remote execution substrates
- multi-worker orchestration
