# loopexec CLI

This document defines the command surface and machine contract for loopexec.

## Commands

- `loopexec init`
  - Initialize local loopexec workspace metadata.
- `loopexec run`
  - Run a bounded `check_fixpoint` loop until the check passes, a bound trips, or the work command fails.
  - Flags: `--check "<cmd>"` (required external oracle; exit 0 = converged), `--exec "<cmd>"` (work step run each iteration), `--max-iterations N` (fuse, default 10), `--once` (run exactly one iteration for debugging; overrides `--max-iterations`; converges or halts `max_iterations_reached`), `--run-id`, `--workdir`, `--budget-usd`.
  - Optional set-based progress (section 3.2): `--failures-cmd "<cmd>"` prints the current open failures (one identity per line) and enables the no-regression ratchet; `--no-progress-k N` halts after N iterations with no new best failing-set size.
  - Optional metric-integrity gate (section 6): `--integrity-cmd "<cmd>"` prints the test-determining surface (one identity per line); its `t0` set MUST NOT lose a member. Evaluated before the green check (guards dominate success); a violation halts `metric_integrity_violation`.
  - Optional receipt pinning (section 8): `--model-provider/--model-id/--model-version`, `--temperature/--seed/--max-tokens`, `--context-file <path>` (repeatable; recorded with sha256), `--cost-usd`. A check fingerprint (exit code + output sha256) is always recorded for `replay`. The `--model-*` flags **record only** -- they do not select or constrain the model (the model is chosen and invoked inside `--exec`), and the pin is not verified against the actual call. See the Models and Agents guide for wiring a specific model.
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
  - Render why the recorded run halted, distinguishing raise-the-limit (the failing set was still shrinking) from do-not-retry (stalled, regressed, oscillating, or infeasible). Reads the latest run by default; `--run-id <id>` explains a specific recorded run.
- `loopexec replay`
  - VERIFY a recorded receipt: re-run the recorded check against the current end-state and confirm the fingerprint matches. Agent-free and budget-free; never re-runs the agent. Exit 0 on a match; `objective_unverified` (13) on a mismatch. Verifies the latest run by default; `--run-id <id>` verifies a specific recorded run. (`reexecute`, the live re-run, is Planned.)
- `loopexec attest`
  - HMAC-sign the receipt (over the model pin, sampling, context manifest, cost, and fingerprint) so provenance is checkable; `--verify` checks the stored signature. Signs the latest run by default; `--run-id <id>` targets a specific recorded run. Key from `--key`, else `$LOOPEXEC_ATTEST_KEY`, else a dev default.
- `loopexec report`
  - Render a recorded receipt as a digest: the run's outcome (phase, halt reason, exit class), its pins (check, fingerprint, model, sampling, cost, context manifest size), whether it has been attested, and the per-iteration timeline parsed from `.loopexec/run-<id>.jsonl`. Re-runs nothing and exits `0` even for a failed run (it reports, it does not re-decide). Reports the latest run by default; `--run-id <id>` targets a specific recorded run.

`run` writes a per-run state snapshot (`.loopexec/run-<id>.state.json`) next to its receipt, so `replay` / `explain-halt` / `attest` / `report` can address any recorded run by `--run-id` even after later runs advance the default `.loopexec/state.json` pointer.
- `loopexec reexecute`
  - Live re-run of the recorded loop config `--samples N` times in isolated copies; reports the halt-reason distribution and convergence rate (a statistical match, not byte identity). `--confirm` required (it burns budget).
- `loopexec escalate`
  - Emit a structured escalation packet (`--channel file|stdout`; github/slack Planned) and mark the run `paged`. Cleared by `ack`.
- `loopexec watch`
  - Read `.loopexec/heartbeat` and report `alive` or `heartbeat_stale` (19) when its age exceeds `--stall-timeout`. The kill-the-wedged-PID actuator is Planned.
- `loopexec ack`
  - Clear comprehension debt and any paged escalation, recording `--reviewer` (a forcing/visibility gate, not proof of comprehension).
- `loopexec build-context`
  - Assemble a narrow, budgeted context slice (state + the open failure + relevant files) under a code-calibrated token ceiling. Relevance = files named in the `--failure` stack trace + the last git diff (`--diff-base`) + untracked files. Flags: `--failure <file|-|text>`, `--budget-tokens N` (default 8000), `--diff-base`, `--workdir`, `--out` (confined to `--workdir`). Never fatal on no files; `context_budget_unsatisfiable` (19) only if the mandatory state+failure slice cannot fit. File resolution is workdir-confined and symlink-safe (untrusted failure text cannot read outside the workdir), reads are size-bounded, and untrusted content is fence-escaped.
- `loopexec isolate`
  - Orchestrate two-zone isolation (SPEC section 7): a hardened **detached-clone** sandbox (no origin, no host hooks, no inherited credentials), a **per-run minted/revoked credential** (injected via a `0600 --env-file`, never on the argv / in the receipt; `--mint-cmd` requires `--revoke-cmd`), and a rendered exec-zone (`--network none`) + agent-zone (egress-allowlist via `--egress-proxy`) launch plan. `--execute --confirm` launches via `--runtime` (default `docker`); otherwise the plan is rendered only. Image/run-id inputs are validated and a `--` separator stops docker flag parsing (no argument-injection). A failed zone surfaces as `execution_failure` (40). The container engine, the auditing egress proxy, and the provider key API are operator-provided hooks.
- `loopexec inspect-cost`
  - Analyze a per-iteration cost ledger against a run-total cap and a sigma anomaly bound. loopexec does not meter live token cost (the model call lives in `--exec`); it owns the math over costs you supply. Inputs: `--ledger <file>` (one USD per line) and/or `--cost <usd>` (repeatable). `--budget-usd` is the run-total hard cap (over it halts `budget_exceeded`, 18); `--sigma N` (default 3) flags any iteration exceeding the rolling mean + N standard deviations of the iterations before it (`cost_anomaly`, 18). A flat ledger has no variance, so it raises no anomaly; negative costs and an empty ledger are rejected (`invariant_failed`, 20). Auto-parsing provider usage and in-loop enforcement during `run` are Planned.
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
  "version": "0.2.0",
  "status": "ok",
  "run_id": "local",
  "iteration": 1,
  "errors": []
}
```

## Exit codes

The `halt_reason` string is the stable contract; the exit code is its coarse class (see `SPEC.md` section 5). Classes `13`, `14`, `16`, `17`, `18`, and `19` emit today alongside the base `0/10/12/20/30/40/50` (class `18`, `budget_exceeded` / `cost_anomaly`, via `inspect-cost`); only class `15` (`check_inadequate`) and class `11`'s task-list reasons stay reserved. A few individual reasons inside active classes, and in-loop budget enforcement during `run`, are still Planned (see `SPEC.md` section 11).

A computed halt is an outcome, not a crash: the command emits its result object (JSON or the human summary) to stdout and exits with the class code, and prints nothing to stderr. A converged run exits `10` cleanly. Only genuine failures (invalid usage, unreadable state, I/O errors) print a message to stderr.

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
