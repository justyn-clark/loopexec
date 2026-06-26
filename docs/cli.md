# loopexec CLI

This document defines the command surface and machine contract for loopexec.

## Commands

- `loopexec init`
  - Initialize local loopexec workspace metadata.
- `loopexec run`
  - Run a bounded `check_fixpoint` loop until the check passes, a bound trips, or the work command fails.
  - Flags: `--check "<cmd>"` (required external oracle; exit 0 = converged), `--exec "<cmd>"` (work step run each iteration), `--max-iterations N` (fuse, default 10), `--run-id`, `--workdir`, `--budget-usd`.
  - Optional set-based progress (section 3.2): `--failures-cmd "<cmd>"` prints the current open failures (one identity per line) and enables the no-regression ratchet; `--no-progress-k N` halts after N iterations with no new best failing-set size.
  - Optional metric-integrity gate (section 6): `--integrity-cmd "<cmd>"` prints the test-determining surface (one identity per line); its `t0` set MUST NOT lose a member. Evaluated before the green check (guards dominate success); a violation halts `metric_integrity_violation`.
  - Optional receipt pinning (section 8): `--model-provider/--model-id/--model-version`, `--temperature/--seed/--max-tokens`, `--context-file <path>` (repeatable; recorded with sha256), `--cost-usd`. A check fingerprint (exit code + output sha256) is always recorded for `replay`.
  - Optional comprehension gate (section 9): `--comprehension-every N` halts `comprehension_debt_exceeded` after N merged-but-unread iterations (cleared by `loopexec ack`). Each iteration also updates a heartbeat for `loopexec watch`.
  - Halt reasons are **computed** from observed state (`success_condition_met` / `max_iterations_reached` / `execution_failure` / `workspace_invalid`, plus `no_progress_detected` / `oscillation_detected` / `same_test_regressed` when `--failures-cmd` is set), never forced by a flag.
  - Writes a typed JSONL receipt to `.loopexec/run-<id>.jsonl` and atomic state to `.loopexec/state.json`.
- `loopexec probe-check`
  - Measure check determinism as a confidence bound (SPEC O2): no check, no loop.
  - Flags: `--check "<cmd>"` (required), `--runs N` (default derived), `--max-flake-rate R` (derives run count via the rule of three, `runs >= 3/R`), `--workdir`.
  - Reports the achieved 95% upper bound on the flake rate; halts `check_flaky` (exit 14) if the verdict varies across runs.
- `loopexec doctor`
  - Gate loop preconditions. Enforces determinism (via the probe) and an isolation preflight; reports hermeticity and adequacy as planned (SPEC O3-O5, section 7).
  - Flags: `--check "<cmd>"`, `--runs N`, `--max-flake-rate R`, `--workdir`, plus isolation preflight `--bind-claude-home` (a `$HOME/.claude` credential mount fails closed -> `credential_scope_invalid`, 13) and `--exec-network <policy>` (must be `none` -> else `isolation_unsatisfiable`, 30).
  - Exit 0 on a green doctor; `check_flaky` (14), `credential_scope_invalid` (13), `isolation_unsatisfiable` (30), or `workspace_invalid` (30) otherwise.
- `loopexec explain-halt`
  - Render why the recorded run halted, distinguishing raise-the-limit (the failing set was still shrinking) from do-not-retry (stalled, regressed, oscillating, or infeasible). Reads `.loopexec/state.json`.
- `loopexec replay`
  - VERIFY a recorded receipt: re-run the recorded check against the current end-state and confirm the fingerprint matches. Agent-free and budget-free; never re-runs the agent. Exit 0 on a match; `objective_unverified` (13) on a mismatch. (`reexecute`, the live re-run, is Planned.)
- `loopexec attest`
  - HMAC-sign the receipt (over the model pin, sampling, context manifest, cost, and fingerprint) so provenance is checkable; `--verify` checks the stored signature. Key from `--key`, else `$LOOPEXEC_ATTEST_KEY`, else a dev default.
- `loopexec reexecute`
  - Live re-run of the recorded loop config `--samples N` times in isolated copies; reports the halt-reason distribution and convergence rate (a statistical match, not byte identity). `--confirm` required (it burns budget).
- `loopexec escalate`
  - Emit a structured escalation packet (`--channel file|stdout`; github/slack Planned) and mark the run `paged`. Cleared by `ack`.
- `loopexec watch`
  - Read `.loopexec/heartbeat` and report `alive` or `heartbeat_stale` (19) when its age exceeds `--stall-timeout`. The kill-the-wedged-PID actuator is Planned.
- `loopexec ack`
  - Clear comprehension debt and any paged escalation, recording `--reviewer` (a forcing/visibility gate, not proof of comprehension).
- `loopexec status`
  - Show loop status.
- `loopexec check`
  - Validate invariants (state hygiene, not the application oracle).
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
