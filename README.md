# loopexec

[![CI](https://github.com/justyn-clark/loopexec/actions/workflows/ci.yml/badge.svg)](https://github.com/justyn-clark/loopexec/actions/workflows/ci.yml)
[![Latest tag](https://img.shields.io/github/v/tag/justyn-clark/loopexec?sort=semver)](https://github.com/justyn-clark/loopexec/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.26.0-00ADD8?logo=go&logoColor=white)](https://go.dev)

`loopexec` is a deterministic runtime for loop engineering: it drives bounded, stateless, auditable execution loops that progress through an external check and stop on a computed reason, never on an agent's say-so. It is the runtime companion to SMALL Protocol: SMALL describes state, constraints, plans, progress, and handoff; loopexec drives one bounded loop against that state and reports machine-readable outcomes.

The normative contract is `SPEC.md`. As of v0.2.0 the binary implements the engine through Slice 7 - 18 commands covering the real loop, determinism probing, the metric-integrity gate, the no-regression ratchet, pinned and offline-verifiable receipts, and two-zone isolation. Every capability in `SPEC.md` section 11 now has a Shipped core; what remains is named, inline sub-parts. See `SPEC.md` section 11 for the per-capability status.

## Current status

Implemented now (v0.2.0):

- Real bounded `run` loop: iterates `--exec` then `--check` until the check passes (`success_condition_met`), a bound trips (`max_iterations_reached`), or work fails (`execution_failure`); "no check, no loop"; computed halt reasons mapped to a stable exit-code class; typed JSONL receipts (`.loopexec/run-<id>.jsonl`) and atomic durable state.
- `probe-check` - determinism as a 95% confidence bound (rule of three); `doctor` - precondition gate (determinism + isolation preflight).
- Set-based progress + no-regression ratchet (oscillation / no-progress / regression halts) + `explain-halt` (raise-the-limit vs do-not-retry).
- Metric-integrity gate (`run --integrity-cmd`): guards dominate success (collected-set monotonicity).
- Receipt pinning (model tuple + sampling + context manifest + cost + check fingerprint); `replay` (verify a receipt offline, agent-free) and `attest` (HMAC-sign + verify).
- `reexecute` (live re-run distribution), `escalate` / `watch` (heartbeat + structured packet), comprehension `ack` gate.
- `build-context` - budgeted, workdir-confined, symlink-safe relevant-file slice.
- `isolate` - two-zone orchestration: detached-clone sandbox + per-run minted/revoked credential + rendered/launched exec/agent zones.
- Global `--json` output, explicit exit-code contract, 63 contract/unit tests, GitHub Actions CI (`gofmt`, `go vet`, `go test ./...`).

Not implemented yet (named sub-parts; see `SPEC.md` section 11):

- The operator-provided infra `isolate` composes with: the container engine, the auditing egress proxy, and the provider key API (`--runtime` / `--egress-proxy` / `--mint-cmd` + `--revoke-cmd`).
- Live/auto cost metering (parsing provider usage) and in-loop budget enforcement during `run` -- the cost analysis itself ships as `inspect-cost` (run-total cap -> `budget_exceeded`, sigma spike -> `cost_anomaly`) over a supplied ledger; the deeper metric-integrity layers (assertion-count / manifest-hash / coverage-floor); `probe-check` adversarial perturbation + the in-loop sequential monitor.
- The `import_closure` / `dep_graph` context-relevance tiers; github/slack escalation channels; the kill-the-PID watchdog actuator; git revert-to-best for the ratchet.
- Full SMALL-driven `task_list` loop execution and `small` CLI integration; container, Nix, or remote substrates and multi-worker orchestration.

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

- `loopexec init` / `run` / `status` / `check` / `step`
- `loopexec probe-check` / `doctor` / `explain-halt`
- `loopexec replay` / `attest` / `report` / `reexecute`
- `loopexec escalate` / `watch` / `ack`
- `loopexec build-context` / `isolate` / `inspect-cost`

See `docs/cli.md` for flags and exit semantics.

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
  "version": "0.2.0",
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

The `halt_reason` string is the stable contract; the exit code is its coarse class (SPEC section 5). Every class `13`-`19` emits today alongside the base `0/10/12/20/30/40/50` (class `18`, `budget_exceeded` / `cost_anomaly`, via `inspect-cost`; class `15`, `check_inadequate`, via the `doctor --mutate-cmd` adequacy canary); only class `11`'s task-list reasons (`no_actionable_tasks` / `human_required`) remain reserved, and those belong to the `task_list` loop topology. A few individual reasons inside active classes, the `doctor` coverage-delta and hermeticity tiers, and in-loop budget enforcement during `run`, are still Planned (see SPEC section 11).

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
