# Generated CLI reference

Generated from the Cobra command tree. Do not edit manually.

Source version: `0.4.1`.

## `loopexec`

loopexec - deterministic runtime for loop engineering

Usage: `loopexec [flags]`

- `--help, -h` (`bool`, default `false`): help for loopexec
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--version, -v` (`bool`, default `false`): version for loopexec

## `loopexec ack`

Clear comprehension debt and any paged escalation (records the reviewer)

Usage: `loopexec ack [flags]`

- `--help, -h` (`bool`, default `false`): help for ack
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--reviewer` (`string`, default ``): Reviewer identity recorded in the ack (required)
- `--workdir` (`string`, default ``): Directory containing .loopexec/state.json (default: current directory)

## `loopexec attest`

Sign the receipt (HMAC) so provenance is verifiable; --verify to check a signature

Usage: `loopexec attest [flags]`

- `--help, -h` (`bool`, default `false`): help for attest
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--key` (`string`, default ``): Attestation key (else $LOOPEXEC_ATTEST_KEY, else a dev default)
- `--run-id` (`string`, default ``): Attest a specific recorded run by id (default: the latest run)
- `--verify` (`bool`, default `false`): Verify the stored signature instead of creating one
- `--workdir` (`string`, default ``): Directory containing .loopexec (default: current directory)

## `loopexec budget`

Shared call budgets across independent run IDs

Usage: `loopexec budget [flags]`

- `--help, -h` (`bool`, default `false`): help for budget
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--policy` (`string`, default ``): Pinned scoped budget policy JSON
- `--store-dir` (`string`, default ``): Existing absolute directory shared by all runs in this budget scope

## `loopexec budget reserve`

Consume one model-call slot before launching a child

Usage: `loopexec budget reserve [flags]`

- `--call-id` (`string`, default ``): Unique identity for this logical call; duplicates fail closed
- `--help, -h` (`bool`, default `false`): help for reserve
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--phase` (`string`, default ``): Budget phase, such as builder or critic
- `--policy` (`string`, default ``): Pinned scoped budget policy JSON
- `--store-dir` (`string`, default ``): Existing absolute directory shared by all runs in this budget scope

## `loopexec budget status`

Read remaining scoped model-call allowance

Usage: `loopexec budget status [flags]`

- `--help, -h` (`bool`, default `false`): help for status
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--policy` (`string`, default ``): Pinned scoped budget policy JSON
- `--store-dir` (`string`, default ``): Existing absolute directory shared by all runs in this budget scope

## `loopexec build-context`

Assemble a narrow, budgeted context slice (state + failure + relevant files)

Usage: `loopexec build-context [flags]`

- `--budget-tokens` (`int`, default `8000`): Context token ceiling (code-calibrated ~3.3 chars/token)
- `--diff-base` (`string`, default `HEAD`): Git ref for the last-diff relevance signal
- `--failure` (`string`, default ``): Failure / stack-trace source: a file path, '-' for stdin, or inline text
- `--help, -h` (`bool`, default `false`): help for build-context
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--out` (`string`, default ``): Output path within --workdir (default: <workdir>/.loopexec/context.md)
- `--strategy` (`string`, default `stacktrace_last_diff`): Relevance strategy (import_closure / dep_graph are Planned)
- `--workdir` (`string`, default ``): Directory to resolve files against (default: current directory)

## `loopexec check`

Validate loop invariants (state hygiene, not the application oracle)

Usage: `loopexec check [flags]`

- `--fail-invariant` (`bool`, default `false`): Force invariant failure
- `--help, -h` (`bool`, default `false`): help for check
- `--json` (`bool`, default `false`): Emit machine-readable JSON output

## `loopexec demo`

Prove execute -> external check -> receipt -> replay without credentials

Usage: `loopexec demo [flags]`

- `--help, -h` (`bool`, default `false`): help for demo
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--workdir` (`string`, default ``): Fresh directory for durable demo artifacts (default: create and retain a temporary directory)

## `loopexec doctor`

Gate loop preconditions: determinism + isolation preflight + adequacy canary (--mutate-cmd)

Usage: `loopexec doctor [flags]`

- `--bind-model-home` (`bool`, default `false`): Declare a $HOME/.model-home credential bind-mount (fails the isolation preflight)
- `--check` (`string`, default ``): External check command to validate
- `--exec-network` (`string`, default ``): Declared exec-zone network policy; must be 'none' (SPEC section 7)
- `--help, -h` (`bool`, default `false`): help for doctor
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--max-flake-rate` (`float64`, default `0`): Target max flake rate to certify
- `--mutate-cmd` (`string`, default ``): Operator command that plants a mutation in the changed code; the --check MUST then turn red, else check_inadequate (SPEC O4)
- `--runs` (`int`, default `0`): Determinism probe runs (default: derived, else 10)
- `--workdir` (`string`, default ``): Directory to run the check in (default: current directory)

## `loopexec escalate`

Emit a structured escalation packet and mark the run paged (cleared by `ack`)

Usage: `loopexec escalate [flags]`

- `--channel` (`string`, default `file`): Escalation channel: file | stdout (github/slack are Planned)
- `--help, -h` (`bool`, default `false`): help for escalate
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--workdir` (`string`, default ``): Directory containing .loopexec/state.json (default: current directory)

## `loopexec explain-halt`

Explain why the recorded run halted (raise-the-limit vs do-not-retry)

Usage: `loopexec explain-halt [flags]`

- `--help, -h` (`bool`, default `false`): help for explain-halt
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--run-id` (`string`, default ``): Explain a specific recorded run by id (default: the latest run)
- `--workdir` (`string`, default ``): Directory containing .loopexec (default: current directory)

## `loopexec init`

Initialize loopexec workspace metadata

Usage: `loopexec init [flags]`

- `--help, -h` (`bool`, default `false`): help for init
- `--json` (`bool`, default `false`): Emit machine-readable JSON output

## `loopexec inspect-cost`

Analyze a per-iteration cost ledger against a budget cap and a sigma anomaly bound

Usage: `loopexec inspect-cost [flags]`

- `--budget-usd` (`float64`, default `0`): Run-total hard cap in USD (0 = no cap); over it halts budget_exceeded (18)
- `--cost` (`stringArray`, default `[]`): Per-iteration USD cost (repeatable); an alternative to --ledger
- `--help, -h` (`bool`, default `false`): help for inspect-cost
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--ledger` (`string`, default ``): File of per-iteration USD costs, one per line (the cost ledger)
- `--sigma` (`float64`, default `3`): Anomaly threshold in standard deviations above the rolling mean (cost_anomaly, 18)

## `loopexec isolate`

Two-zone isolation: detached clone + per-run minted key + exec/agent zone launch plan

Usage: `loopexec isolate [flags]`

- `--agent-image` (`string`, default `loopexec/agent`): Agent-zone container image
- `--agent-network` (`string`, default `loopexec-agent-net`): Agent-zone network (operator-provisioned internal net; egress via the proxy)
- `--branch` (`string`, default ``): Branch to check out (default: the repo default)
- `--check` (`string`, default ``): Check command to run in the exec zone
- `--confirm` (`bool`, default `false`): Confirm launching containers (required with --execute)
- `--egress-allow` (`stringArray`, default `[]`): Allowed egress host:port (default api.model.example:443); RECORDED in the receipt, enforced by the operator's allowlist proxy
- `--egress-proxy` (`string`, default `http://egress-proxy:8080`): Auditing forward proxy the agent zone routes through
- `--exec` (`string`, default ``): Agent/work command to run in the agent zone
- `--exec-image` (`string`, default `loopexec/exec`): Exec-zone container image
- `--execute` (`bool`, default `false`): Launch the zones (default: render the plan only)
- `--help, -h` (`bool`, default `false`): help for isolate
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--key-env` (`string`, default `MODEL_API_KEY`): Env var the minted credential is injected as
- `--mint-cmd` (`string`, default ``): Operator hook that prints a per-run scoped, spend-capped key to stdout (requires --revoke-cmd)
- `--repo` (`string`, default ``): Repository to clone into the sandbox (default: --workdir)
- `--revoke-cmd` (`string`, default ``): Operator hook that revokes the key (receives it via the key-env env var)
- `--run-id` (`string`, default ``): Run identifier (slug)
- `--runtime` (`string`, default `docker`): Container runtime used to launch the zones
- `--workdir` (`string`, default ``): Directory holding .loopexec (default: current directory)

## `loopexec probe-check`

Measure check determinism as a confidence bound (no check, no loop)

Usage: `loopexec probe-check [flags]`

- `--check` (`string`, default ``): External check command to probe (required)
- `--help, -h` (`bool`, default `false`): help for probe-check
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--max-flake-rate` (`float64`, default `0`): Target max flake rate to certify (derives runs via rule of three)
- `--runs` (`int`, default `0`): Number of probe runs (default: derived from --max-flake-rate, else 10)
- `--workdir` (`string`, default ``): Directory to run the check in (default: current directory)

## `loopexec reexecute`

Live re-run of the recorded loop config N times; reports a statistical match (--confirm)

Usage: `loopexec reexecute [flags]`

- `--confirm` (`bool`, default `false`): Confirm the budget-burning live re-run
- `--help, -h` (`bool`, default `false`): help for reexecute
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--samples` (`int`, default `3`): Number of live re-run samples
- `--workdir` (`string`, default ``): Directory containing .loopexec/state.json (default: current directory)

## `loopexec replay`

VERIFY a recorded receipt: re-run the check and confirm the fingerprint (agent-free)

Usage: `loopexec replay [flags]`

- `--help, -h` (`bool`, default `false`): help for replay
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--run-id` (`string`, default ``): Verify a specific recorded run by id (default: the latest run)
- `--workdir` (`string`, default ``): Directory containing .loopexec (default: current directory)

## `loopexec report`

Render a recorded receipt (state + JSONL event log); re-runs nothing

Usage: `loopexec report [flags]`

- `--help, -h` (`bool`, default `false`): help for report
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--run-id` (`string`, default ``): Report a specific recorded run by id (default: the latest run)
- `--workdir` (`string`, default ``): Directory containing .loopexec (default: current directory)

## `loopexec run`

Run a bounded check_fixpoint loop until the check passes or a bound trips

Usage: `loopexec run [flags]`

- `--budget-usd` (`float64`, default `0`): Run-total USD allowance; requires workflow strict or observed metering
- `--check` (`string`, default ``): External check command; exit 0 means converged (required)
- `--command-timeout` (`duration`, default `0s`): Deadline for every command and collector (0 = disabled)
- `--comprehension-every` (`int`, default `0`): Halt comprehension_debt_exceeded after N iterations without a `loopexec ack` (0 = off)
- `--context-file` (`stringArray`, default `[]`): File to include in the receipt context manifest (path + sha256); repeatable
- `--cost-usd` (`float64`, default `0`): Legacy recorded cost metadata; workflow actuals are metered separately
- `--exec` (`string`, default ``): Work command run each iteration before the check (e.g. an agent invocation)
- `--failures-cmd` (`string`, default ``): Command printing current open failures (one identity per line); enables set-based progress and the no-regression ratchet
- `--help, -h` (`bool`, default `false`): help for run
- `--integrity-cmd` (`string`, default ``): Command printing the test-determining surface (one identity per line); its t0 set MUST NOT lose a member (metric-integrity gate)
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--max-iterations` (`int`, default `10`): Maximum iterations (fuse)
- `--max-tokens` (`int`, default `0`): Recorded sampling max_tokens
- `--model-id` (`string`, default ``): Record the model id in the receipt (records only; the call lives in --exec); enables the model/sampling pin
- `--model-provider` (`string`, default ``): Record the model provider in the receipt (records only; does NOT select the model - --exec does)
- `--model-version` (`string`, default ``): Record the model version/build in the receipt
- `--no-progress-k` (`int`, default `3`): Halt no_progress_detected after K iterations with no new best failing-set size
- `--once` (`bool`, default `false`): Run exactly one iteration (debug single-step); overrides --max-iterations
- `--resume` (`bool`, default `false`): Resume a workflow run with identical policy and cumulative bounds
- `--run-id` (`string`, default ``): Run identifier
- `--seed` (`int`, default `0`): Recorded sampling seed
- `--temperature` (`float64`, default `0`): Recorded sampling temperature
- `--terminate-grace` (`duration`, default `250ms`): Grace between process-group TERM and KILL
- `--timeout` (`duration`, default `0s`): Total run deadline (0 = disabled)
- `--workdir` (`string`, default ``): Directory to run commands in (default: current directory)
- `--workflow` (`string`, default ``): Pinned workflow JSON (argv hooks, metering, candidates, numeric policy)

## `loopexec status`

Show loop status

Usage: `loopexec status [flags]`

- `--halt-reason` (`string`, default ``): Current halt reason
- `--help, -h` (`bool`, default `false`): help for status
- `--iteration` (`int`, default `0`): Current iteration
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--run-id` (`string`, default ``): Run identifier

## `loopexec step`

Execute a single loop step

Usage: `loopexec step [flags]`

- `--help, -h` (`bool`, default `false`): help for step
- `--json` (`bool`, default `false`): Emit machine-readable JSON output

## `loopexec version`

Print the CLI identity

Usage: `loopexec version [flags]`

- `--help, -h` (`bool`, default `false`): help for version
- `--json` (`bool`, default `false`): Emit machine-readable JSON output

## `loopexec watch`

Read the heartbeat and report alive vs heartbeat_stale

Usage: `loopexec watch [flags]`

- `--help, -h` (`bool`, default `false`): help for watch
- `--json` (`bool`, default `false`): Emit machine-readable JSON output
- `--stall-timeout` (`int`, default `600`): Seconds before the heartbeat is considered stale
- `--workdir` (`string`, default ``): Directory containing .loopexec/heartbeat (default: current directory)

## JSON field contract

Types and JSON tags are generated from the runtime structs. Value semantics and exit behavior are described in cli.md and SPEC.md.

### `response`

- `failure_cause,omitempty`: `string`
- `best_candidate,omitempty`: `string`
- `tool`: `string`
- `version`: `string`
- `status`: `string`
- `run_id,omitempty`: `string`
- `iteration,omitempty`: `int`
- `halt_reason,omitempty`: `string`
- `check_exit,omitempty`: `*int`
- `receipt,omitempty`: `string`
- `probe,omitempty`: `*main.probeReport`
- `doctor,omitempty`: `*main.doctorReport`
- `verdict,omitempty`: `string`
- `why,omitempty`: `string`
- `verified,omitempty`: `*bool`
- `signature,omitempty`: `string`
- `ops,omitempty`: `*main.opsReport`
- `context,omitempty`: `*main.contextReport`
- `isolation,omitempty`: `*main.isolationReport`
- `report,omitempty`: `*main.reportSummary`
- `cost,omitempty`: `*main.costReport`
- `budget_scope,omitempty`: `*main.scopedBudgetReport`
- `errors`: `[]string`

### `receiptEvent`

- `output_truncated,omitempty`: `bool`
- `phase,omitempty`: `string`
- `duration_ms`: `int64`
- `cause,omitempty`: `string`
- `ts`: `int64`
- `run_id`: `string`
- `iteration`: `int`
- `event`: `string`
- `detail,omitempty`: `string`
- `exit_code,omitempty`: `*int`

### `probeReport`

- `runs`: `int`
- `passes`: `int`
- `fails`: `int`
- `flake_count`: `int`
- `stable`: `bool`
- `flake_upper_bound`: `float64`
- `confidence_pct`: `int`
- `distinct_exit_codes`: `[]int`
- `max_flake_rate,omitempty`: `float64`
- `certified`: `bool`

### `doctorReport`

- `checks`: `[]main.doctorCheck`
- `probe,omitempty`: `*main.probeReport`

### `opsReport`

- `samples,omitempty`: `int`
- `converged,omitempty`: `int`
- `convergence_rate,omitempty`: `float64`
- `distribution,omitempty`: `map[string]int`
- `packet,omitempty`: `string`
- `heartbeat_age_s,omitempty`: `*int`
- `stale,omitempty`: `*bool`

### `contextReport`

- `path`: `string`
- `tokens_estimated`: `int`
- `budget_tokens`: `int`
- `files_included`: `[]string`
- `files_dropped`: `[]string`

### `isolationReport`

- `sandbox`: `string`
- `clone`: `string`
- `agent_image`: `string`
- `exec_image`: `string`
- `exec_zone_cmd`: `string`
- `agent_zone_cmd`: `string`
- `egress_allow`: `[]string`
- `key_env,omitempty`: `string`
- `hardened`: `bool`
- `minted`: `bool`
- `revoked`: `bool`
- `executed`: `bool`
- `exec_zone_exit,omitempty`: `*int`
- `agent_zone_exit,omitempty`: `*int`

### `reportSummary`

- `budget,omitempty`: `*main.budgetBook`
- `numeric,omitempty`: `*main.numericState`
- `candidate,omitempty`: `*main.candidateState`
- `best_candidate,omitempty`: `string`
- `phase`: `string`
- `exit_code`: `int`
- `iterations`: `int`
- `check,omitempty`: `string`
- `workdir,omitempty`: `string`
- `cost_usd,omitempty`: `float64`
- `model,omitempty`: `*main.modelPin`
- `sampling,omitempty`: `*main.samplingPin`
- `context_files,omitempty`: `int`
- `fingerprint,omitempty`: `*main.checkFingerprint`
- `events`: `int`
- `receipt,omitempty`: `string`
- `receipt_status`: `string`
- `warnings`: `[]string`
- `timeline`: `[]main.receiptEvent`
- `score_target,omitempty`: `*float64`
- `attestation_status`: `string`
- `attested`: `bool`

### `costReport`

- `iterations`: `int`
- `total_usd`: `float64`
- `mean_usd`: `float64`
- `stddev_usd`: `float64`
- `max_usd`: `float64`
- `budget_usd,omitempty`: `float64`
- `sigma`: `float64`
- `over_budget`: `bool`
- `anomaly_at,omitempty`: `[]int`

### `scopedBudgetReport`

- `scope`: `string`
- `max_calls`: `int`
- `used`: `int`
- `remaining`: `int`
- `phase_remaining`: `map[string]int`
- `reservation,omitempty`: `string`

### `doctorCheck`

- `name`: `string`
- `status`: `string`
- `detail,omitempty`: `string`

### `budgetBook`

- `mode`: `string`
- `microusd`: `int64`
- `monetary_known`: `bool`
- `calls`: `int64`
- `tokens`: `int64`
- `entries`: `map[string]workflow.Call`
- `pending,omitempty`: `*workflow.Preflight`
- `phase_costs_usd,omitempty`: `[]float64`

### `numericState`

- `best_accepted,omitempty`: `*float64`
- `patience_anchor,omitempty`: `*float64`
- `reviewed`: `int`
- `reviews_since_improvement`: `int`

### `candidateState`

- `best_iteration,omitempty`: `int`
- `numeric,omitempty`: `*main.numericState`
- `attempted_id,omitempty`: `string`
- `accepted_id,omitempty`: `string`
- `restored_id,omitempty`: `string`
- `best_id,omitempty`: `string`
- `initial_id,omitempty`: `string`
- `status`: `string`
- `best_exposed`: `bool`
- `protected`: `[]main.manifestEntry`
- `best_fingerprint,omitempty`: `*main.checkFingerprint`

### `modelPin`

- `provider,omitempty`: `string`
- `id`: `string`
- `version,omitempty`: `string`

### `samplingPin`

- `temperature`: `float64`
- `seed`: `int`
- `max_tokens,omitempty`: `int`

### `checkFingerprint`

- `exit_code`: `int`
- `output_sha256`: `string`

### `manifestEntry`

- `path`: `string`
- `sha256`: `string`
