# loopexec CLI

This document defines the command surface and machine contract for loopexec.

## Commands

- `loopexec init`
  - Initialize local loopexec workspace metadata.
- `loopexec run`
  - Run a bounded `check_fixpoint` loop until the check passes, a bound trips, or the work command fails.
  - Flags: `--check "<cmd>"` (required external oracle; exit 0 = converged), `--exec "<cmd>"` (work step run each iteration), `--max-iterations N` (fuse, default 10), `--run-id`, `--workdir`, `--budget-usd`.
  - Halt reasons are **computed** from observed state (`success_condition_met` / `max_iterations_reached` / `execution_failure` / `workspace_invalid`), never forced by a flag.
  - Writes a typed JSONL receipt to `.loopexec/run-<id>.jsonl` and atomic state to `.loopexec/state.json`.
- `loopexec status`
  - Show loop status.
- `loopexec check`
  - Validate invariants.
- `loopexec step`
  - Execute a single step.

## Global flags

- `--json`
  - Emit exactly one JSON object to stdout.
  - Human logs and error text go to stderr.

## JSON output schema

All commands in `--json` mode emit one object with these fields:

- `tool` (string)
- `version` (string)
- `status` (string)
- `run_id` (string, optional)
- `iteration` (integer, optional)
- `halt_reason` (string, optional)
- `errors` (array of strings)

Example:

```json
{
  "tool": "loopexec",
  "version": "0.2.0-rc1",
  "status": "ok",
  "run_id": "local",
  "iteration": 1,
  "errors": []
}
```

## Exit codes

The `halt_reason` string is the stable contract; the exit code is its coarse class (see `SPEC.md` section 5). Codes `13`-`19` are reserved for halt reasons emitted by later slices.

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
- `40` execution failure (timeout or command failure)
- `50` internal error

## Full example

Human mode:

```sh
loopexec init
loopexec run
loopexec status --run-id local --iteration 1
```

JSON mode:

```sh
loopexec run --json
```
