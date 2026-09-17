# Bounded unattended workflows

The Go governor remains the only retry controller. The opt-in `run --workflow`
contract composes an argv adapter with exact accounting, candidate snapshots,
and optional numeric progress. It needs no model SDK or engine dependency.

## Run controls

`--timeout 30s` bounds the total run, including collectors and hooks.
`--command-timeout 3s` bounds each subprocess.
`--terminate-grace 100ms` bounds TERM-to-KILL escalation.
Deadlines default to zero (disabled); termination grace defaults to 250ms.
The unattended example explicitly enables all three.

On macOS and Linux each governor command owns a process group. Cancellation
(SIGINT/SIGTERM or a deadline) sends TERM, then KILL after grace, waits for the
direct child, and closes pipes. Surviving group members are also killed on normal
parent exit. Adapter children join that group. Descendants deliberately escaping
the group require container/cgroup or equivalent OS containment; process groups
are not an adversarial sandbox. Other platforms build with direct-child
termination only, not a process-tree guarantee. Their exclusive lock file needs
operator removal after an uncatchable crash; Unix locks release automatically.

Command output has a 1 MiB capture ceiling. Commands retain separate 16 KiB
stdout/stderr diagnostic tails in `.loopexec/<run-id>/diagnostics/` (0600 files).
Raw output never goes in receipts or terminal responses. No full logs are
collected by default. Treat private diagnostic tails as potentially sensitive;
do not upload them indiscriminately. Credentials belong in isolated provider
wrappers or private input channels, never command strings or argv recorded in
config/state. Legacy `--exec`/`--check` strings remain recorded for replay.

Every command emits start and end, timeout, or cancellation evidence with phase,
duration, exit, and cause. Final state is atomic on controlled termination.
Candidate cleanup gets a separate bounded grace (at least 250ms); deadlines may
therefore finish after the configured wall time by termination/cleanup grace.
SIGKILL, power loss, and storage failures cannot promise a final receipt.

## Workflow JSON and hook protocol

The generated example's `workflow.json` is an executable reference for schema 1.
The schema types live in `internal/workflow/contract.go`. Unknown or duplicate JSON
keys, trailing JSON, excessive nesting, nonregular evidence files, and evidence
over 1 MiB fail closed.

All hooks are argv arrays. The governor appends one absolute request-file path:

```json
{
  "schema_version": 1,
  "run_id": "creative",
  "iteration": 1,
  "phase": "exec",
  "config_hash": "<sha256>",
  "reservations": [
    {
      "id": "creative/000001/exec/builder",
      "phase": "builder",
      "max_microusd": 400000,
      "max_tokens": 40
    }
  ]
}
```

Check and score requests additionally carry `candidate_id`. With candidate
management this is the SHA256 of the sorted artifact manifest; otherwise it is
the run/iteration identity. Hooks must read their request, emit just their
documented stdout result, and use exit 0 for successful collection. Collector
stderr, nonzero exit, unreadable/oversized output, or malformed JSON is failure.
Legacy identity collectors accept newline-separated IDs or `{"ids":[]}`.
Empty stdout with no stderr is a valid empty set.

`exec` is the bounded work attempt. `check` and `score.command` are local,
read-only verification of saved evidence. `metering.reserve` and
`metering.usage` are also local collectors: they must not themselves spend.
Put all priced builder, critic, engine/test, and tool work inside `exec` and
declare a separate stable call ID for each. There is one check invocation per
attempt. No verification hook should recapture, rerender, or invoke a model.

A nonzero exec is infrastructure failure, exit 40. Ordinary rejection must
return exec 0 and check nonzero. Check 0 can converge only after every configured
integrity, usage, numeric, and candidate guard passes. Protocol failures use
existing classes: `objective_unverified` / `metric_integrity_violation` (13),
`cost_anomaly` / `budget_exceeded` (18), or `execution_failure` (40).
The additive `failure_cause` explains the failed phase. No new exit class.

## Spending and restart guarantees

Money is integer micro-USD: one dollar is 1,000,000 units. `--budget-usd` accepts
up to six decimals; accumulation and reservation comparison use checked integers.
Model/sampling flags and `--cost-usd` remain legacy metadata, not invocation
controls or a substitute for actuals.

| metering.mode | Contract |
| --- | --- |
| strict | Requires a positive budget, reserve hook, usage hook, and `enforced:true` per preflight. Preventive guarantee is conditional on the trusted adapter/provider enforcing every upper bound. |
| observed | Usage is reconciled after work. The governor stops further work at exhaustion, but cannot prevent the completed call from overspending. Never a strict cap. |
| unmetered | Local/subscription monetary usage can be `unknown`; no dollar budget is allowed. Time/attempt bounds still apply, plus call/token bounds when configured. |

The preflight response has `schema_version, run_id, iteration, phase, enforced,
calls[]`. Each call declares `id, phase, max_microusd, max_tokens`.
IDs have the prefix `<run>/<six-digit iteration>/<request phase>/`.
Required call/token limits also require preflight even in unmetered mode.

Every workflow requires a `metering.usage` argv hook, including `unmetered`
local workflows. Unmetered adapters report executed work with `status:unknown`
when monetary usage is unavailable; this does not require a reserve hook unless
call or token limits are configured. An empty call list is valid only when there
was no reportable work. Omitting the usage hook is a configuration error.

The usage response has `schema_version, run_id, iteration, phase, calls[]`.
Each call has `id, phase, status` and optional integer `microusd, tokens`.
Status is `actual` (money required), `unknown` (unmetered only, money absent),
or `skipped` (no charge/tokens). Every reserved call must appear, including a
skipped critic after technical failure. Token counts are required for executed
calls when a token cap is configured. Negative, conflicting, missing, stale, or
over-reservation usage is a typed failure. Known overspending is retained in
the ledger; an unresolved/unknown charge is never silently replaced by zero.

The governor persists reservations before starting work and reconciles the entire
usage batch atomically afterward. Duplicate identical actuals are idempotent;
conflicting duplicates fail. The budget book records cumulative actuals, pending
reservations, calls/tokens, and monetary-known status. Consumers must use
`budget.monetary_known`; the legacy `cumulative_usd` number alone cannot express
unknown spending. Rolling cost anomalies reuse inspect-cost's prior-mean/sigma
logic (three prior observations; flat prior costs do not trigger an anomaly).

Use a fresh run ID for a fresh workflow, or `--resume` with exactly the original
workflow, commands, limits, and policy. Concurrent governors in the same runtime
directory are rejected. Resume retains attempts and the original absolute total
deadline; it never resets spending or replays an interrupted work call.
Unresolved reservations must reconcile successfully through the read-only usage
hook before any new work. If actuals cannot be recovered, the run remains halted.
A converged run is not relaunched. Generic `reexecute` rejects workflow receipts:
initialize a new adapter workspace explicitly so absolute hooks/credentials cannot
accidentally act on the original candidate.

## Numeric progress

`score` declares `command, direction, min, max, target, min_improvement,
tolerance, patience`. Direction is `maximize` or `minimize`.
A score collector emits `schema_version, run_id, iteration, candidate_id,
reviewed, technical`, and `value` only for reviewed technical success.
Optional `veto:true` rejects candidate promotion even when the score improves;
it consumes a reviewed revision without resetting patience. The example sets
this when a category floor fails, independently of the weighted total.
The deterministic collector, not the critic, establishes `technical`.
A failing overall check may still have technically valid work below its visual
target. The critic can veto that work; it cannot override failed engine tests.

Comparisons round differences to nine decimal places before testing inclusive
boundaries. Domain validation occurs first. Tolerance applies to acceptance
against the best score and target; it never helps reset patience.
`min_improvement` compares against the last patience-resetting score, so several
small gains can accumulate. Best accepted score and that anchor are separate.

The example maximizes a 0-10 weighted score with target 8.5, improvement 0.2,
zero acceptance tolerance, patience 2 reviewed revisions, and four total attempts.
Identical failure IDs do not cause oscillation in numeric mode. Unreviewed
technical failures consume attempts and time, but no visual patience.
With candidates, regressions restore best and continue; without candidates a
technical/score regression after success halts `same_test_regressed`.
Default legacy set-based oscillation behavior is unchanged.

## Candidate transaction

`candidate` declares a relative `root`, exact `include` paths (files/directories),
explicit `exclude` paths, `protected` policy/test/config files relative to the
workdir, and optional `baseline`. The root must be separate from runtime storage
and cannot name reserved governance directories. Includes cannot overlap.
Symlinks, special files, and escaping paths are rejected. File hashing/copying
is cancellable; manifests permit up to 10,000 regular files, at most 1 GiB each.

Runtime state, policies, verifier code, evidence, caches, and feedback live outside
the restorable manifest. Generated binaries and untracked deliverables inside
the manifest are captured. Files outside it are untouched. There is no git
reset/clean and no assumption that a commit captures generated deliverables.

The initial manifest is saved even if no valid candidate exists.
`baseline:true` optionally validates existing evidence using read-only check/score
hooks at iteration 0. Otherwise no candidate is called valid until an attempt
passes technical and acceptance guards. Sub-target technical improvements may be
promoted as best while the loop continues toward convergence.

The state records attempted, accepted, restored, initial, and best IDs separately,
plus whether best is exposed in the working candidate directory. Rejections retain
their snapshots, review/feedback, and actual resource charges, then restore best
(or the initial tree if none was valid). A tolerated score decrease can be accepted
without replacing the higher-scoring best snapshot. Cached evidence from rejected
content cannot bless restored or subsequently changed content.

Snapshot files are staged before their directory is published. A promotion journal
keeps the old best pointer until the new pointer commits atomically. Interrupted
promotion/restoration resumes by restoring the last committed best. Restoration
failure is terminal, with best exposure false. `best_candidate` is returned only
for an exposed, verified candidate. Nothing treats half-written content as accepted.

Protected hashes detect drift; they are not a permission boundary. For live use,
enforce builder RW access only to the candidate manifest using a separate user,
container, VM, or equivalent OS policy. Keep controller state, verifier/config,
test fixtures, private critic credentials, and critic evidence inaccessible for
builder writes. Give the critic read-only candidate/reference access and its own
evidence channel. A host process with the same user's permissions can defeat
filesystem conventions or steal a private key. The fixture explicitly simulates
roles; it does not claim adversarial sandboxing.

## Offline example

From the repository root:

```bash
mkdir -p build
go build -o build/loopexec ./cmd/loopexec
go build -o build/creative-workflow ./examples/creative-workflow
./scripts/creative-proof.sh ./build/loopexec ./build/creative-workflow build/creative-proof
```

The proof output directory must be new or empty. Success prints
`CREATIVE_PROOF_OK`. The script asserts convergence (10), budget stop (18),
regression restoration then convergence (10), stale rejection (13), critic skip
after technical failure (10), and the absolute four-attempt cap (12).
It replays saved accepted verdicts without calling builder/critic.

To inspect a single initialized fixture:

```bash
./build/creative-workflow init --dir build/my-creative-run --scenario convergence
set +e
./build/loopexec run --json --workdir build/my-creative-run \
  --workflow workflow.json --run-id creative --max-iterations 4 \
  --budget-usd 3 --timeout 30s --command-timeout 3s --terminate-grace 100ms
code=$?
set -e
test "$code" -eq 10
./build/loopexec report --workdir build/my-creative-run --run-id creative
./build/loopexec replay --json --workdir build/my-creative-run --run-id creative
```

The actual example operations are `init`, `attempt <request-file>`,
`verify <request-file>`, `score <request-file>`, `reserve <request-file>`, and
`usage <request-file>`. The governor supplies request files; do not invent
provider usage or invoke attempts outside its reservation/lifetime boundary.

Receipts and journals: `<fixture>/.loopexec/run-creative.{jsonl,state.json}`
and `<fixture>/.loopexec/creative/`. Synthetic evidence:
`<fixture>/evidence/attempt-000001/` and subsequent attempts.
Ranked next-attempt feedback is `feedback.json`; the builder receives those
issues plus their hash. Accepted and rejected binary assets live in candidate
snapshots. Private fixture signing keys are under `private/`; CI excludes them.

The five categories use weights 25/25/20/15/15, all floors 7, total at least 8.5.
Six fixed camera/view IDs and up to two exploratory captures are verified.
Motion evidence is required only when `engine.json.requires_motion` is true.
The verifier calculates the aggregate. A separate critic invocation signs the
candidate/test/capture/policy bindings; the builder never receives the key.
Each candidate claims one review before critic launch; an interrupted claim does
not authorize a second invocation. The canonical verdict excludes timestamps and
volatile paths. Timing metadata is retained separately.

Every score, capture, performance implication, and dollar charge in these fixtures
is SYNTHETIC. They demonstrate contract wiring, not image quality, real spending,
Blender/Godot validation, or AAA certification. Replaying a saved verdict is not a
fresh performance measurement. Live use still requires verified local argv
interfaces, Blender asset scripts, Godot test/camera routes, an independent critic,
provider-specific usage/reservation enforcement, and the OS isolation above.
No live Codex CLI or MCP protocol is assumed by this repository.
