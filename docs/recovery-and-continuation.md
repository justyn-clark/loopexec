# Recovery and Continuation

Source `0.5.0-rc.2` is unpublished. Stable promotion is held in
`scripts/release-readiness.json` (bound by the source digest). Offline verification is not live campaign success
or independent review delivery.

## Bounded Restoration

`run --recovery-timeout` is independent of `--terminate-grace`. Zero selects two
minutes. Choose an explicit bound using realistic payload sizes and slow-I/O
tests. Recovery may finish after work cancellation but cannot start more work.
The original execution deadline is not extended. File operations cooperate with
cancellation; an OS call blocked in the kernel cannot be hard-preempted.

Before rollback, policy-included attempted files are captured outside the mutable
tree. The journal's `attempt_archive_id` and `attempt-recovery-only` metadata mark
this as unaccepted recovery evidence, never a best candidate or review. Adapters
must durably preserve event streams, native logs and files outside the include
policy before returning control. Core output tails are bounded diagnostics, not
a replacement for authoritative streamed model output; collectors fail closed on
truncation. Await output completion and never overwrite a durable event file with
an outer pipe capture.

The complete target snapshot is validated and a verified stage built before
application. Journals record target, phase, counts, deadline and failure cause.
During recovery, inspect `.loopexec/<run-id>/candidate.json`; progress is saved
every 64 files and at phase boundaries. The final run state is not a live counter.
Files are replaced atomically individually, not as one filesystem transaction.
An interrupted application can leave a partial working tree, but status remains
`restoring`, `best_exposed` is false and initial/best pointers remain authoritative.
Only full manifest verification, directory sync and durable journal commit expose
a completed restore. Never use a `restoring` tree as accepted work.
Final validation of that restored best uses the same absolute recovery deadline,
not an expired work context or a renewed cleanup allowance. This exception is
limited to the best verified by the current recovery; it cannot validate unrelated
snapshots or extend the work deadline. Expired recovery verification fails closed.
Staging completes before obsolete included files and empty conflicting directories
are pruned. File/directory transitions are supported. Retained excluded content
blocks a conflicting transition rather than being deleted. Full stored best
snapshots are checked before exposure, including an already accepted resume.

## Truthful Failure and Usage

`failure_cause` retains the primary cause; ordered `failures` also retain cleanup
failures. Failed restoration still yields an execution-failure exit. Typed events
record both; historical receipt lines are not edited to manufacture success.
An executed command failure precedes a failed usage reconciliation in the failure
chain. A failed reservation hook is not mislabeled as an executed command.

Final JSON and reports distinguish settled counters, unresolved reservations,
dispatch awaiting usage and adapter-reported executed-but-unsettled calls.
`accounting.total_calls`, `total_tokens` and `total_microusd` are null when unknown.
Settled zero is not total zero. Unsettled reports are observations, not validated
usage or provider attestation. Valid incurred usage survives limit/identity halts.
Unrecoverable usage stays unresolved and blocks automated continuation.

Before dispatch, both the accounting journal and saved run state durably record
the pending reservation and dispatch obligation. A checkpoint failure prevents
execution. Resume rejects a stale pre-dispatch journal, even after an uncatchable
governor crash. This is local file continuity, not protection against an operator
rolling back every authoritative state file together.

Each usage collector capture is retained byte-for-byte under
`.loopexec/<run-id>/usage-reports/<iteration>-<phase>-<sha256>.json`.
Failed/truncated captures use `.diagnostic-tail` and are not complete usage proof.
Names are exclusive-create and hash-bound; identical reports can be reused only
after byte verification and sync. Reports are bounded to 1 MiB; archive failure
halts before settlement. These private files may contain sensitive adapter output;
do not publish them or mistake unvalidated JSON for provider attestation.
The adapter still owns durable full provider streams.

Conflicting later observations are archived separately and cannot erase an earlier
unsettled report. Every previously observed call and known token/money amount must
remain present and nondecreasing, including same-reservation resume. Missing fields
may gain information; valid actuals can still settle. Lower reports halt as
`usage_observation_conflict`; the governor does not guess which report is true or
infer zero from settled counters. Resolve evidence externally before continuation.

The adapter's physical-asset/provider ledger is a separate authority. A planned
reservation or launched adapter does not prove every planned model ran. Report
spent external reservations separately; never infer physical calls spent from
pending roles or invent missing token/dollar totals.

## Resume and Linked Contracts

True `--resume` retains original workflow/model policy, declared context pins,
recorded runtime version, bounds and absolute deadline. Pin adapter code and
resources through candidate protected files or `--context-file`; undeclared
dependencies cannot be detected automatically. Retain the original checksum-pinned
installation: the automatic version check does not identify different local
builds sharing a version. Old states lacking a runtime version also require the
operator to retain their original exact installation.
Legacy model identity and sampling pins are compared before any receipt append.
Resume requires a present, valid accounting journal containing every saved settled
entry and phase-cost prefix. Pending reservations cannot disappear without valid
settlement. Valid accounting or promotion progress ahead of the last run-state
write is allowed; stale/regressed usage, best iteration, fingerprints or protected
pins are rejected. Missing state is not a new empty budget or initial snapshot.
Observed/unmetered phases without named reservations still retain their dispatch
obligation. A changed pending phase needs new matching settled entries; an empty
pre-dispatch journal is not settlement. Previously observed calls, token counts
and known money cannot become skipped or smaller. An ambiguous empty settlement
ahead of the run state fails closed rather than assuming no external work ran.

For changed adapters/policy use a new run ID and an explicit schema-1 link:

```json
{"continuation":{"workdir":"../prior-workspace","run_id":"prior-run",
"state_sha256":"SHA256_OF_PRIOR_PER_RUN_STATE",
"receipt_sha256":"SHA256_OF_PRIOR_RECEIPT"}}
```

Digest placeholders are not runnable values. Freeze/hash the parent's per-run
state and JSONL receipt. Links reject changed/missing evidence, active parents,
unresolved accounting, unverified restoration and renewed bounds. The child
inherits settled usage, completed iteration count and absolute deadline. Call,
token, monetary and iteration caps cannot increase. Positive monetary caps apply
to observed as well as strict mode. Legacy observed/strict parents without a
recorded monetary-cap contract cannot establish a safe link. Parent receipt/state bytes
remain unchanged. Hash continuity is not independent attestation.
Link evidence hashing is cancellable and uses the independent recovery bound;
this preflight cannot extend the parent's absolute deadline.

Explicitly prepare retained candidate work under the new contract; check/score
hooks replay saved evidence without models. Links do not renew physical ledgers,
allowances or deadlines and do not authorize spending. Expired/exhausted parents
stop. Identical-contract resume may reconcile real saved usage before work; there
is no switch to waive an unresolved accounting gap or replay a paid phase.
An existing candidate journal can still be loaded and restored under the separate
recovery bound when work is expired or usage remains unresolved. This cannot run
an exec/check/score phase, settle invented usage, or renew any allowance. A missing
or changed initial journal pointer blocks recovery; a partial tree is never
recaptured as a new authoritative initial snapshot.

Replay verifies protected pins, the full working candidate and the full stored
snapshot both before and after the read-only check. Matching check output alone
does not verify uninspected or mutated payloads. Baseline promotion likewise
requires unchanged full manifests and protected pins after every verification hook.
Retained iteration-zero baselines replay their original `baseline.check` request,
including after a failed later attempt; no historical request is renamed.

## Fresh Live Pilot Contract

This reusable template does not grant spending authority. The separately
authorized October 16 release-fixture pilot delivered a rejecting independent
review, not campaign success. See the [dated pilot record](releases/v0.5.0-rc.2-pilot.md).
Its three-call allowance is exhausted; offline corrections do not authorize a
retry. A separately authorized follow-up engineering review also rejected the
candidate; see the [October 17 offline repair record](releases/v0.5.0-rc.2-offline-repair.md).
That review extension is exhausted too. Any next live pilot requires fresh
explicit bounds after exact validation:

1. Obtain approval for exact binary SHA, fresh release-owned fixture, models and
   effort, call/token bounds and known-versus-unknown money policy.
2. Use at least 1,387 regular files with realistic native payload sizes. Do not
   reuse a consumer's remaining allowance or relabel its physical asset to create
   a new ledger. Reusing a physical asset requires its original cumulative ledger
   and a separately explicit owner decision, not implicit new-run permission.
3. Propose one builder and one independent read-only critic, one attempt, no retry;
   a 20-minute active cumulative deadline, at most two native jobs within that same
   builder, and a five-minute recovery deadline. These proposed bounds are not
   authorization or provider-side token/dollar caps.
4. Require durable >200KB event streaming and a complete final usage event;
   technical/native checks precede criticism. Verify actual identities, signed
   candidate/capture-bound criticism and stale/exhausted-budget rejection.
5. Retain successful delivery and forced-failure recovery proofs: attempted
   archive, authoritative pointers, cause chain, actual usage, physical spent
   reservations, remaining allowance and cumulative deadlines.

Owner authorization, complete accounting, independent review delivery and exact
candidate source/docs deployment/readback remain gates. Domain quality targets
belong only to adapters, not this general-purpose runtime.
