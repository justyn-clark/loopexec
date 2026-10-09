# Acceptance, objectives and evidence

LoopExec governs bounded execution, computed stops, accounting and retention.
The integration defines its objective and protected external checks. Software,
research, asset creation and operational workflows can use different criteria
without changing the governor's meaning.

## Separate the decisions

- Desired objective: the quality or behavior the owner ultimately wants.
- Recorded run contract: the exact checks, target, rubric and bounds for that run.
- Candidate retention: keeping a best verified intermediate result.
- Computed convergence: all configured gates passed for the recorded contract.
- Human acceptance: an explicit owner's decision, including temporary exceptions.
- Promotion/publication: a separate action bound to the accepted candidate digest.

An owner may deliberately accept an intermediate result while retaining a higher
desired objective. Record who decided, the candidate digest, original and revised
policy, reason, evidence and remaining gaps. Never relabel an earlier failed run
as converged or overwrite its immutable receipt. A score from an older candidate
does not apply to later edits, and a human adjustment is not a model verdict.

## Improve without weakening the desired quality

Freeze references, rubric and evidence conditions before comparing candidates.
Keep protected functional, safety and integrity constraints as vetoes, not
averages that a high subjective score can hide. Break a composite score into
criterion-level gaps with observable correction and verification requirements.

For each next intervention, record the strongest failing criterion, its evidence,
one proposed change, expected observable improvement and regression check. Prefer
cheap deterministic measurements before another independent model review. Keep
contradictory feedback and uncertain judgments explicit; calibrate the oracle
against labeled examples before asking it to drive more work.

Separate artifact defects from measurement defects. If repeated attempts produce
no measurable gain, halt and diagnose capture conditions, reference suitability,
rubric consistency, adapter effectiveness and feasibility. Increasing patience
without changing the intervention does not supply new evidence. Do not silently
lower the objective to make the counter turn green.

Bind comparisons to objective ID, candidate digest, policy/rubric revision,
reference/capture configuration, checks and model identity. Preserve an earlier
better candidate with its evidence; explicitly decide which revision to promote.
Any new allowance or policy revision requires owner authorization, retains the
original spend ledger and records the change. Changing run IDs is not permission
to reset budgets or relaunch paid work.

## Current capabilities and remaining gap

Published LoopExec provides per-run receipts, replay, bounds, numeric patience,
candidate retention/restoration and opt-in shared model-call reservations across
runs. The local report patch makes evidence gaps and retention clearer. The
adapter remains responsible for capturing full evidence and human decisions.

A native cross-run objective/candidate/policy/decision history is not implemented.
Until it exists, retain a project-owned append-only decision record outside run
workspaces and join it to immutable receipts by IDs/digests. Do not promise that
the current governor automatically reconciles changed rubrics or owner decisions.
The [creative verification guide](creative-verification.md) is one worked domain
example, not a universal score or certification policy.
