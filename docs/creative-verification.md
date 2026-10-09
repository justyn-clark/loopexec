# Capturing creative verification evidence

LoopExec governs attempts, accounting, candidate retention, and computed stops.
A project's protected checks and independent critic define its quality standard.
This is one optional domain example; LoopExec is not specific to creative work.
An art score is meaningful only with a fixed rubric, calibrated references,
candidate-bound captures, and technical evidence. No generic score certifies AAA
quality across projects.

## One attempt, complete evidence

Keep each adapter invocation to one bounded attempt. LoopExec owns retries.
The adapter should record these phases before returning, including failed or
skipped phases:

| Phase | Retained evidence | Gate |
| --- | --- | --- |
| Readiness | Hashed source, export, runtime, reference calibration, material transfer, controlled lighting, detail comparison, regression checks and resolved contradictions | Missing/stale/unresolved evidence blocks model spending |
| Builder | Actual model/version, reasoning policy, stable call ID, result and available usage | Reserve immediately before launch; failed launch consumes its slot |
| Technical checks | Protected check results, import/load diagnostics, scale/collision/performance measurements | A red technical check skips the critic |
| Captures | Fixed camera IDs/poses, engine version, render warm-up, PNG hashes, capture receipts and diagnostics | Bind every image to this candidate; no recapture in verification |
| Critic | Read-only independent role, rubric hash, category scores, ranked issues tied to view/region, uncertainty and vetoes | A score cannot override a failed check |
| Verdict | Signed candidate/objective/policy/capture/result bindings, local replayable check and score output | Reject missing, tampered or stale evidence |
| Promotion | Explicit human decision, promoted candidate digest and backup | Convergence does not automatically publish or change the main project |

Store private model logs, references, signing keys and diagnostics outside the
deliverable. Retain compact summaries and hashed evidence. LoopExec keeps bounded
diagnostic tails, not complete model transcripts or engine logs; the adapter
must deliberately capture those if needed. See [workflow trust boundaries](workflows.md).

## Read a verdict correctly

```sh
loopexec report --json --workdir <run-workspace> --run-id <run-id>
loopexec report --workdir <run-workspace> --run-id <run-id>
```

Both commands read existing evidence without models or engines. The unreleased
report improvements expose typed `report.timeline`, `report.receipt_status`,
`report.warnings`, `failure_cause`, and signature verification status. Inspect
these alongside `halt_reason`, `report.numeric`, and `report.budget`.

- `success_condition_met` (exit 10) means configured gates converged.
- `max_iterations_reached` (exit 12) is a stopped attempt, even when
  `candidate.status` is `accepted` or a best candidate is exposed. That status
  describes retention within this run, not meeting its recorded acceptance target.
  A later explicit owner adjustment is a separate decision, not receipt convergence.
- `numeric.best_accepted` and patience apply to this run. They do not remember
  another run's higher score or prove the current promoted files still match.
- `budget.monetary_known:false` means unknown cost. `microusd:0` or `tokens:0`
  in that state does not establish free work or zero token consumption.
- `report.attestation_status:present_unverified` means a signature file exists.
  `attest --verify` checks its HMAC; project critic signatures have their own
  verifier. Neither signature establishes artistic truth.
- `report.receipt_status:readable` describes parsing. Use `replay` to re-run
  the recorded local check and compare its fingerprint. Inspect the check first:
  replay executes it and is safe without spending only when the adapter obeys
  the local, read-only hook contract.

Compare reviews only when the candidate digest, rubric, references, engine,
camera and settled lighting are known. Preserve score regressions and unresolved
issues; post-review edits are unreviewed until independently verified. Repeated
or contradictory criticism should lead to a measured diagnostic intervention,
not another run ID or an automatic model switch.

## Shared allowance and migration

See [acceptance policy](acceptance-policy.md) for recording an owner-adjusted
intermediate acceptance while preserving the desired quality objective. In the
home 3D integration, the owner deliberately adjusted acceptance to 8.1 after days
of work; 8.5 remains the desired target. The earlier receipt's 8.5 gate is
historical evidence, not a complete account of that later decision. No asset,
project target or review allowance is changed by this guide.

Use the published v0.4.0 `budget reserve` immediately before every model
subprocess, with a shared store outside all run workspaces and a stable scope
per asset. Include builder, critic and optional camera-director phase limits.
Unknown charges still consume call reservations. Verification, imports and
rendering should never contain hidden model launches.

For an existing project-specific ledger, preserve every historic reservation,
including failed launches. Migrate with stable call IDs and verify old/new used
and remaining counts before enabling the adapter. An exhausted allowance stays
exhausted. Do not rename the asset, recreate the store, reset a policy or count
only successful reviews to restore spending. See the [offline migration example](../examples/scoped-budget/README.md).

The shared budget counts calls only. Do not confuse its `budget_scope.used` with a
workflow budget's count of all reported work phases. Engine/technical phases
can appear as unknown workflow usage without consuming a model-call slot.

## Next usability increment

A useful cross-run history should join stable asset scope, run ID, candidate
digest, rubric hash, technical result, score/target, ranked issues, reserved
calls, known/unknown cost, retained/promoted revision, and evidence paths. Keep
missing evidence and unmatched promoted files explicit. That history is not
implemented by the current per-run `report`; collecting it should require no
models, engines or automatic retries.
