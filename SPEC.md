# loopexec - Canonical Specification

**Status:** normative. This is the single source of truth for the loopexec contract. The binary (`cmd/loopexec`) and the docs site (`loopexec.dev`) MUST conform to this document. Where `docs/loop-contract.md` and this file disagree, this file wins; `loop-contract.md` is the **`task_list` mode profile** of this spec.

**Spec version:** 2.0.0  |  **Conformance keywords:** MUST / MUST NOT / SHOULD / MAY per RFC 2119.

---

## 1. What loopexec is

loopexec is a **deterministic runtime for loop engineering**: it runs bounded, stateless, auditable work loops that progress through an **external check**, durable state, isolated execution, and **explicit stop conditions**.

A loop runs until an external check passes, a guard trips, a budget is spent, or a human is required - **never** because an agent reports it is done. Every halt emits a **computed** reason (never a flag) and a **replayable receipt**.

loopexec is **not** an agent, a model, or a task runner. It composes with those by governing *how* work is executed, recorded, and resumed.

---

## 2. Topology - the two machines, made explicit

loopexec hosts two control topologies. The mode is explicit and recorded in every receipt.

| `loop.mode` | Stop condition | Gate | Profile |
|---|---|---|---|
| `check_fixpoint` | external check returns `success_exit_code` | the application oracle (test/build/lint/score) | this document, section 4 |
| `task_list` | all SMALL plan tasks `completed` | `small check --strict` (state hygiene) **+ a per-task acceptance oracle** | `docs/loop-contract.md` |

**Invariant T1.** `check_fixpoint` is the degenerate one-task plan whose acceptance criterion *is* the external check. `task_list` MUST attach a real per-task acceptance oracle; state-hygiene validation alone is NOT an acceptance oracle (it cannot decide application correctness). A `task_list` loop without a per-task oracle MUST refuse to mark tasks `completed` and MUST halt `check_inadequate`.

**Invariant T2.** The receipt MUST record, per iteration, which oracle was evaluated at which commit. A receipt that cannot answer *"which oracle, at which commit, under which pinned model, having spent how much, with which guards intact"* is not a halt record.

---

## 3. The oracle contract (the check)

**O1 - External.** The check MUST be an external process returning an exit code, independent of the agent. The agent's self-assessment is not a check.

**O2 - Determinism as a maintained confidence bound (not a one-time probe).**
- `loopexec probe-check` MUST report an **achieved confidence bound** on the check's flake rate, not a pass/fail of N runs. The run count is derived from a target `max_flake_rate`, not a magic constant.
- During a loop, each in-loop check invocation is treated as a free Bernoulli sample; the runtime maintains a sequential (Wilson) lower bound on stability and MUST halt `check_not_deterministic` when it drops below `confidence_target`.
- Probing MUST be adversarial where configured (test order, seed, clock, concurrency, load) and MUST report which dimension broke.

**O3 - Hermeticity.** A check declared deterministic MUST be hermetic: frozen clock/seed/TZ/locale, OS-assigned (`:0`) ports, ephemeral fixtures, pinned tool versions. A non-hermetic check MUST be rejected by `doctor` (`check_not_hermetic`), not silently looped on.

**O4 - Adequacy != determinism.** A check that returns `0` regardless of the diff is deterministic and useless. `doctor` MUST verify adequacy: every changed/added line is exercised by the check (coverage delta), and a mutation canary injected into changed lines MUST turn the check red. Failure => `check_inadequate`.

**O5 - Determinism != idempotency.** A deterministic but side-effecting check (passes once, fails on rerun in the same workspace) is distinct from a flaky one; it MUST be reported `check_has_side_effects` and cured by reset, not by editing the check.

**O6 - Single capture.** The runtime MUST run the check **once** per iteration and derive *both* the green/red verdict and the failing-test set from that one captured artifact `{exit_code, stdout, stderr}`. It MUST NOT re-invoke the suite to scrape failure text.

---

## 4. Iteration lifecycle - `check_fixpoint`

Each iteration MUST execute as one ordered transaction:

1. **Build context** - a narrow, budgeted slice (`build-context`). Token budget is a true ceiling, measured with a code-calibrated estimator; a slice that cannot fit even after widening tiers MUST emit `context_budget_unsatisfiable`, never silently evict relevant files.
2. **Execute the agent** in the agent zone (section 7).
3. **Run the check once** (O6); capture the artifact; parse the failing-test set `F_i`.
4. **Guards dominate success (G-before-S).** Evaluate guards (section 6) BEFORE any green branch can fire. A check that went green because a guard was violated (e.g., a weakened test) MUST classify as the guard reason, NOT `success_condition_met`.
5. **Acceptance (anti-regression ratchet).** Accept the iteration's diff only if the passing-test set is a **superset** of the last accepted set (primary guarantee). Optionally enforce a numeric `score >= best_so_far` ratchet with an acceptance band. On violation, revert to the last accepted commit and re-prompt.
6. **Progress / feasibility.** Update `F_i`. Halt on no strict decrease in `|F_i|` over K iterations, on a previously-passing test re-entering `F` (oscillation), or on a contradictory pair across >=2 cycles. Distinguish "still improving at cutoff" (raise the limit) from "stalled/infeasible" (do not retry).
7. **Checkpoint the receipt** (section 8). Every iteration MUST append a typed JSONL event and update durable state.
8. **Continue or halt.** Halt only on a computed condition from section 5; otherwise iterate. Before declaring `success_condition_met`, the full check phase MUST pass (a targeted subset MAY drive intermediate iterations).

---

## 5. Halt reasons -> exit codes (canonical map)

The **`halt_reason` string is the stable integration contract**; the **exit code is a coarse class** CI can branch on without parsing JSON. Existing codes `0/10/11/12/20/30/40/50` are preserved; `13-19` are reserved new classes. Every reason MUST be **computed from observed state** - `--halt-reason` is a hidden test fixture only.

| exit | class | `halt_reason` strings | owner |
|---|---|---|---|
| 0 | nominal | *(loop ran, no halt)* | LoopExec |
| 10 | converged | `success_condition_met` | LoopExec |
| 11 | terminal-blocked | `no_actionable_tasks`, `human_required` | human |
| 12 | iteration-cap | `max_iterations_reached` | LoopExec |
| 13 | integrity | `blocked_path_modified`, `reward_hacking_detected`, `metric_integrity_violation`, `credential_scope_invalid`, `objective_unverified` | LoopExec (det.) |
| 14 | oracle-untrusted | `check_flaky`, `check_has_side_effects`, `check_not_hermetic`, `hermeticity_violation` | LoopExec (det.) |
| 15 | check-inadequate | `check_inadequate` | LoopExec (det.) |
| 16 | resumable-judgment | `escalation_pending`, `reviewer_rejected` | Musketeer / human |
| 17 | no-convergence | `no_progress_detected`, `same_failure_repeated`, `oscillation_detected`, `same_test_regressed`, `unsatisfiable_constraints`, `infeasible_suspected` | LoopExec (det.) |
| 18 | budget | `budget_exceeded`, `cost_anomaly` | LoopExec (det.) |
| 19 | liveness/drift | `heartbeat_stale`, `model_drift_detected`, `comprehension_debt_exceeded`, `context_budget_unsatisfiable` | LoopExec (det.) |
| 20 | invariant | `invariant_failed` | LoopExec |
| 30 | workspace | `workspace_invalid`, `isolation_unsatisfiable` | LoopExec |
| 40 | execution | `execution_failure` | LoopExec |
| 50 | internal | `internal_error` | LoopExec |

**Ownership stance.** LoopExec owns everything a number can decide (exit codes, the `F_i` trajectory, a fingerprint hash, a coverage/mutation delta, a manifest hash, a spend ledger, a model-identity tuple, a sequential flake bound). Musketeer owns only what a number cannot express - `reward_hacking_detected`, `reviewer_rejected` - layered **on top of** the deterministic `metric_integrity_violation`, never replacing it. Humans own the irreducible decision and the ack. **Judgment is never the stop oracle:** it can veto a green loop or escalate, but it cannot promote a red one to green.

**Migration.** The legacy bare `"blocked"` string (one word, three meanings) is split: `no_actionable_tasks` (11), `blocked_path_modified` (13), `human_required` (11).

---

## 6. Guards & metric integrity

Guards are evaluated against an **immutable baseline `t0`** captured at loop start, BEFORE any green declaration (G-before-S, section 4.4), covering working-tree + staged + committed + untracked changes.

- **`blocked_paths`** - globs the loop MUST NOT modify (default includes `test/`, `infra/`, `.env`, `.git/`). Violation => `blocked_path_modified`.
- **Metric integrity** (supersedes `git diff --quiet -- test/`): the collected-test-set MUST NOT lose a member; test-count and AST-level assertion-count MUST NOT decrease; a protected-manifest hash over the full test-determining surface (runner/coverage config, CI yaml, test-dep lockfile) MUST be stable; coverage MUST NOT drop below the `t0` floor. Tests SHOULD run from a read-only mount frozen at `t0`. Violation => `metric_integrity_violation`.
- **Judge (advisory).** A cross-model, cross-lab judge MAY veto a green iteration (`reviewer_rejected` / `reward_hacking_detected`). It receives a behavioral oracle (check log, result delta, collected-set delta, coverage/mutation signal), treats the diff as untrusted data, and never replaces the deterministic gate.

---

## 7. Isolation & secrets (two zones)

A single `--network none` container cannot both run a cloud agent and isolate untrusted code. Isolation MUST be two zones sharing only a work volume that is a **detached clone** (a git worktree is ergonomic, NOT a security boundary - it shares the object DB/refs/remotes/hooks/credentials).

- **Agent zone** - reasoning surface. Network: default-deny + **egress allowlist** to the model endpoint only, via an auditing proxy. Credentials: exactly one **per-run minted, short-TTL, spend-capped** key (never a `~/.claude` bind-mount). Filesystem: RW `/work`, RO elsewhere, no `$HOME`.
- **Exec zone** - untrusted code execution (check/build/test). Network: `none`. Hermetic (O3). Ephemeral: reset each run.
- **`doctor` preconditions (fail-closed):** reject `network: none` for a cloud/local-host model (`isolation_unsatisfiable`); reject any `~/.claude` mount (`credential_scope_invalid`); reject `exec_zone.network != none`; require an egress allowlist; warn on `worktree` + credentialed origin.

---

## 8. Receipts, replay, and state

**Receipt (per run).** MUST pin everything that determines output, or fail closed (`workspace_invalid`): the **model-identity tuple** `{provider, model_id, version/build, quantization, runtime, endpoint}`, recorded sampling params `{temperature, top_p, seed, max_tokens}`, a context manifest `[{path, content_sha256}]`, parsed cost actuals `{prompt_tokens, completion_tokens, usd}`, the per-iteration check command + exit code + commit, the computed `halt_reason`, and the achieved determinism confidence. Receipts SHOULD be signed (`attest`).

**Replay vs re-execute (MUST NOT be conflated):**
- **`replay` = VERIFY.** Re-run the deterministic check against the recorded end-state and confirm the fingerprint matches the receipt. Agent-free, budget-free, deterministic. Answers *"does this receipt's verdict hold?"*
- **`reexecute` = RE-RUN.** Run the live agent loop again. Non-deterministic (the LLM samples); reports a **statistical** match, never byte identity. Budget-burning; `--confirm`-gated.

A live-LLM trajectory is not reproducible; only the **verdict** is. Marketing copy MUST say "replayable **verdicts**," not "replayable runs."

**State (durable, resumable).** `.loop_state.json` MUST carry at least: `phase`, `iteration`, `last_green_commit`, `open_failures`, `cumulative_usd`, `baseline` (Step-1 measurement), `determinism_probe` result, `metric_integrity` snapshot, `model_pin`, and `escalation` state. On resume, the runtime MUST revalidate `determinism_probe` and `metric_integrity` before the first agent call and halt on drift.

---

## 9. Cost & liveness

- **Budget** MUST be a **run-total** hard cap (not only per-turn), accumulated from parsed provider usage, with a rolling-sigma anomaly detector (`cost_anomaly`) distinct from the absolute cap (`budget_exceeded`). The cost model includes agent tokens **+ judge cost + per-iteration check-execution cost**; the ceiling is a quantile, not a mean mislabeled "upper bound."
- **Liveness.** The heartbeat MUST be read by an external watchdog (`loopexec watch`) that times out a wedged agent call and emits `heartbeat_stale`. Every exit - including a non-zero agent exit - MUST pass through the typed logger; a `set -e`-style silent death is non-conformant.
- **Comprehension.** `diffs_merged_unread` MUST be tracked; the loop SHOULD halt `comprehension_debt_exceeded` after a configured threshold, cleared by a signed `ack`. (A forcing/visibility gate, not proof of comprehension.)

---

## 10. Command surface

| Command | Purpose | Exit semantics |
|---|---|---|
| `init` | scaffold versioned `.loop_state.json` + `loop.yml` | 0 / `workspace_invalid` |
| `run` | the real iterating loop (section 4); computed halt | per section 5 |
| `run --once` | single iteration (debug); absorbs legacy `step` | per section 5 |
| `probe-check` | determinism as a confidence bound (O2); agent-free | 0 / class 14 |
| `build-context` | bounded relevant slice; budget-unsatisfiable handling | 0 / `context_budget_unsatisfiable` |
| `doctor` | precondition gate: determinism + hermeticity + adequacy + isolation | 0 / class 14|15|30 |
| `check` | SMALL/state-hygiene validation only (NOT the application oracle) | 0 / `invariant_failed` |
| `ratchet` | inspect/advance the monotonic acceptance frontier | 0 |
| `replay` | VERIFY a recorded receipt (agent-free, budget-free) | 0 / class 13 |
| `reexecute` | live re-run; statistical match; `--confirm` | 0 |
| `report` / `status` | render receipt / live tail | 0 |
| `inspect-cost` | composite cost model + caps + anomaly | 0 |
| `explain-halt` | human rationale; separates "raise the limit" from "never retry" | 0 |
| `escalate` / `watch` | structured human handoff + external watchdog | `escalation_pending` / `heartbeat_stale` |
| `attest` / `ack` | sign a receipt / signed comprehension ack | 0 |

**JSON contract.** Every command in `--json` mode emits exactly one JSON object to stdout; human logs and errors go to stderr. The object includes at least `{tool, version, status, errors[]}` and, where applicable, `{run_id, iteration, halt_reason}`. The schema is additive: new fields MAY be added; existing field meanings MUST NOT change within a major spec version.

---

## 11. Capability status (normative source for the docs matrix)

Each capability is **Shipped** (in `cmd/loopexec` with tests), **In progress** (on a feature branch), or **Planned** (specified here, not yet built). The docs site MUST render this distinction and MUST NOT present Planned capability as Shipped.

| Capability | Status |
|---|---|
| CLI contract: `--json`, stable exit codes, deterministic output | Shipped |
| `init` / `status` / `check` / `step` stubs | Shipped |
| `run` as a real iterating loop (section 4) | Shipped |
| Computed halt reasons (section 5) replacing `--halt-reason` | Shipped |
| Typed JSONL receipt + durable state (section 8) | Shipped |
| `probe-check` confidence bound (O2) | Shipped (core; adversarial perturbation + in-loop sequential monitor Planned) |
| `doctor` precondition gate (O3-O5, section 7) | Shipped (determinism + isolation preflight; hermeticity/adequacy reported as planned) |
| Set-based progress + no-regression ratchet: oscillation / no-progress / regression halts (section 3.2) | Shipped (via `--failures-cmd`; git revert-to-best Planned) |
| `explain-halt`: raise-the-limit vs do-not-retry (feasibility) | Shipped |
| Metric-integrity gate: guards dominate success (section 6) | Shipped (collected-set monotonicity via `--integrity-cmd`; assertion-count / manifest-hash / coverage-floor Planned) |
| `doctor` isolation preflight: credential-mount + exec-network fail-closed (section 7) | Shipped |
| Receipt pinning: model-identity tuple + sampling + context manifest (sha256) + cost + check fingerprint (section 8) | Shipped (recorded from flags; live cost metering Planned) |
| `replay`: verify a receipt offline by re-running the check and matching the fingerprint (section 8) | Shipped |
| `attest`: HMAC-sign a receipt and `--verify` it (section 8) | Shipped |
| `reexecute`: live re-run of the recorded config N times, halt-reason distribution (section 8) | Shipped |
| `escalate` / `watch` / `ack` + comprehension gate (section 9) | Shipped (file/stdout channels, heartbeat + staleness detection, comprehension `--comprehension-every`; github/slack channels + kill-the-PID actuator Planned) |
| `build-context`: budgeted relevant-file slice with workdir-confined, symlink-safe file resolution (section 4) | Shipped (stacktrace + last-diff + untracked relevance; import_closure / dep_graph tiers Planned) |
| Two-zone isolation orchestration (`isolate`): detached-clone sandbox + per-run minted/revoked credential (0600 env-file, never on the argv) + rendered/launched exec-zone (`network:none`) and agent-zone (egress-allowlist) (section 7) | Shipped (orchestration; the container engine, the auditing egress proxy, and the provider key API are operator-provided hooks: `--runtime`, `--egress-proxy`, `--mint-cmd`/`--revoke-cmd`) |

This table is the contract between the binary and the site. When a capability moves status, update it here first; the binary tests and the docs matrix both reference this section.

Every row above now has a Shipped core. The remaining work is named, inline sub-parts (the operator-provided infra hooks, live cost metering + `cost_anomaly`, the deeper metric-integrity layers, the `import_closure`/`dep_graph` context tiers, github/slack escalation channels, the kill-the-PID watchdog actuator, and git revert-to-best for the ratchet) -- not whole capabilities.
