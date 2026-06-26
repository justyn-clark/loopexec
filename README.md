# loopexec

[![CI](https://github.com/justyn-clark/loopexec/actions/workflows/ci.yml/badge.svg)](https://github.com/justyn-clark/loopexec/actions/workflows/ci.yml)
[![Latest tag](https://img.shields.io/github/v/tag/justyn-clark/loopexec?sort=semver)](https://github.com/justyn-clark/loopexec/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.26.0-00ADD8?logo=go&logoColor=white)](https://go.dev)

`loopexec` is a deterministic runtime for loop engineering: it drives bounded, stateless, auditable execution loops that progress through an external check and stop on a computed reason, never on an agent's say-so. It is the runtime companion to SMALL Protocol: SMALL describes state, constraints, plans, progress, and handoff; loopexec drives one bounded loop against that state and reports machine-readable outcomes.

The normative contract is `SPEC.md`. The binary today implements Slice 0 of that contract (a real bounded `check_fixpoint` loop plus the locked-down CLI, JSON, and exit-code surface) while the rest of the engine is built out slice by slice. See `SPEC.md` section 11 for the per-capability Shipped / In progress / Planned status, and `UPDATES/ref-cross-exam.md` for the rationale.

## Current status

Implemented now:

- Go CLI for `loopexec` in `cmd/loopexec`
- Commands: `init`, `run`, `status`, `check`, `step`
- Real bounded `run` loop: iterates `--exec` then `--check` until the check passes (`success_condition_met`), a bound trips (`max_iterations_reached`), or the work command fails (`execution_failure`)
- No check, no loop: `run` without `--check` halts `workspace_invalid` (SPEC O1)
- Computed halt reasons mapped to a stable exit-code class (SPEC section 5), not a forced flag
- Typed JSONL receipts (`.loopexec/run-<id>.jsonl`) and atomic durable state (`.loopexec/state.json`)
- Global `--json` output mode
- Explicit exit-code contract for success, halt, invariant, workspace, execution, and internal failures
- Contract tests for machine-readable output, halt mapping, and receipt validity
- GitHub Actions CI running `gofmt`, `go vet`, and `go test ./...`

Not implemented yet (specified in `SPEC.md`, see section 11):

- `probe-check` (determinism as a confidence bound) and `doctor` (determinism + hermeticity + adequacy + isolation preconditions)
- `build-context`, `ratchet`, `replay` / `reexecute`, `report`, `inspect-cost`, `explain-halt`, `escalate` / `watch`, `attest` / `ack`
- Two-zone isolation + per-run minted credential
- The metric-integrity gate (guards dominate success)
- Full SMALL-driven `task_list` loop execution and `small` CLI integration
- Container, Nix, or remote execution substrates and multi-worker orchestration

## Install

```bash
go install github.com/justyn-clark/loopexec/cmd/loopexec@latest
```

Requires Go 1.26 or newer.

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
# A real bounded loop: run the work command, then the check, until green or a bound trips.
loopexec run --json --check "go test ./..." --exec "make fix" --max-iterations 10
loopexec status --run-id local --iteration 1
loopexec check
```

Example JSON response (a converged run):

```json
{
  "tool": "loopexec",
  "version": "0.2.0-rc1",
  "status": "halted",
  "run_id": "local",
  "iteration": 3,
  "halt_reason": "success_condition_met",
  "check_exit": 0,
  "receipt": ".loopexec/run-local.jsonl",
  "errors": []
}
```

### Exit codes

The `halt_reason` string is the stable contract; the exit code is its coarse class (SPEC section 5). Existing codes are preserved; `13`-`19` are reserved classes emitted as later slices land.

- `0` success (loop ran, no halt)
- `10` converged: `success_condition_met`
- `11` terminal-blocked: `no_actionable_tasks`, `human_required`
- `12` iteration-cap: `max_iterations_reached`
- `13` integrity: `blocked_path_modified`, `reward_hacking_detected`, `metric_integrity_violation`
- `14` oracle-untrusted: `check_flaky`, `check_not_hermetic`, `hermeticity_violation`
- `15` check-inadequate: `check_inadequate`
- `16` resumable-judgment: `escalation_pending`, `reviewer_rejected`
- `17` no-convergence: `no_progress_detected`, `oscillation_detected`, `infeasible_suspected`
- `18` budget: `budget_exceeded`, `cost_anomaly`
- `19` liveness/drift: `heartbeat_stale`, `model_drift_detected`, `comprehension_debt_exceeded`
- `20` invariant failed
- `30` workspace invalid or missing
- `40` execution failure
- `50` internal error

## Repository layout

```text
SPEC.md                   canonical, normative contract (source of truth)
cmd/loopexec/             loopexec CLI
docs/architecture.md      planned loop architecture
docs/cli.md               current loopexec command contract
docs/integrations.md      integration guidance for the implemented CLI
docs/loop-contract.md     task_list mode profile of SPEC.md
UPDATES/ref-cross-exam.md the cross-examination behind the contract
.github/workflows/ci.yml  fmt/vet/test CI gate
scripts/feeltest.sh       deterministic shell feel test helper
```

## SMALL relationship

This repo is SMALL-governed at the repository level. The implemented `loopexec` binary runs the `check_fixpoint` loop standalone; the full SMALL-driven `task_list` loop is the profile described in `docs/loop-contract.md`.

Read the docs in this order:

0. `SPEC.md` - the canonical, normative contract (source of truth for the binary and the docs site)
1. `docs/cli.md` - what the current CLI actually does
2. `docs/integrations.md` - how to integrate with the current implementation
3. `docs/architecture.md` - target architecture and planned boundaries
4. `docs/loop-contract.md` - the `task_list` mode profile

## Links

- Docs site: https://loopexec.dev
- SMALL Protocol: https://smallprotocol.dev

## License

MIT. See `LICENSE`.
