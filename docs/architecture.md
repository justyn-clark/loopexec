# LoopExec architecture

This guide describes current source. [SPEC.md](../SPEC.md) is the normative
behavioral contract; [its capability matrix](../SPEC.md#11-capability-status-normative-source-for-the-docs-matrix)
distinguishes shipped cores from planned sub-parts.

## Runtime and adapter boundary

LoopExec is a deterministic Go runtime for bounded agentic execution loops.
An adapter supplies the work command and external check. LoopExec owns the
iteration loop, computed halts, durable runtime state, receipts and configured
guards. Models, quality oracles and infrastructure remain integration responsibilities.

SMALL is an optional external state/audit CLI. LoopExec does not select SMALL
tasks, invoke SMALL checkpoints automatically, or require a `.small/` directory
for an ordinary run. See [SMALL integration](small-integration.md) for tested
composition; the [historical task-list design](loop-contract.md) remains planned.

## External-check execution loop

```text
configuration + external check
              |
       validate workspace/policy
              |
       bounded work command <----------------+
              |                              |
       external check + configured guards    |
              |                              |
       record evidence and durable state     |
              |                              |
       compute halt or continue -------------+
```

`cmd/loopexec/run.go` implements this loop. Missing checks fail closed.
The work step runs before the check. Integrity, metering and acceptance guards
dominate a green check; an agent's completion statement is not a halt input.
The legacy check fingerprint records exit code and normalized output hash.
The external check exits 0 when green; verified governor convergence exits 10.

Set-based failure collectors enable the no-regression ratchet and
no-progress/oscillation detection. Opt-in workflow configuration adds argv
hooks, exact usage evidence, numeric progress/patience and candidate manifests.
See the [workflow contract](workflows.md) for ordering and resume rules.

## Components and evidence

| Component | Implementation and responsibility |
| --- | --- |
| CLI/configuration | `cmd/loopexec/main.go`; command tree, flags, JSON identity and exit classes |
| Loop/guards | `run.go`, `progress.go`, `numeric.go`; execution, check, integrity and progress decisions |
| Subprocess lifecycle | `internal/subprocess` and `lifecycle.go`; deadlines, cancellation, process groups and bounded output |
| Workflow/budget | `workflow.go`, `budget.go`, `scoped_budget.go`; pinned policy, exact usage and durable pre-call reservations |
| Candidates | `candidate.go`, `internal/workflow`; manifest-bound snapshots, validation and restoration |
| Receipts/reporting | `receipt.go`, `report.go`; typed JSONL events, saved state, replay, attestation and read-only reports |
| Isolation hooks | `isolate.go`; detached clone, credential lifecycle and two-zone launch orchestration |

Runtime state lives under `.loopexec/`; it is independent of SMALL audit state.
Receipts carry observed outcomes and check fingerprints. `replay` re-runs the
saved check and compares its fingerprint; it does not run the work command.
`report` reads evidence without execution. Signature-file presence is
unverified until explicit `attest --verify`; retaining a candidate is not
owner acceptance. See [acceptance policy](acceptance-policy.md).

## Bounds and isolation

Iteration bounds always govern runs. Total/per-command deadlines default to
disabled (zero) and must be configured when time bounds are required.
Linux/macOS cancellation owns a process group; other platforms bound the direct
child only. Native Windows runtime behavior is not verified by current CI.

Shared scoped budgets count adapter-reserved model calls across run IDs,
independently of token/dollar accounting. Strict monetary guarantees require
trusted adapter/provider enforcement. Observed metering stops later work;
it cannot prevent a provider from exceeding an in-flight allowance.

`isolate` orchestrates operator-provided container runtime, egress proxy and
credential APIs. LoopExec itself does not supply full OS/container containment.
Container/Nix/remote substrates and multi-worker orchestration remain planned.

## Resume and source verification

Workflow resume revalidates pinned configuration, policy and saved candidates;
it preserves cumulative usage instead of resetting allowances. Interrupted
work is not evidence of completion. A workflow runtime has one governor.

The generated [CLI/JSON reference](cli-reference.generated.md) is checked
against the runtime. [Documentation continuity](documentation-continuity.md)
binds allowlisted guides to the release source and requires official deployed
readback. Hash provenance establishes which source was presented; behavioral
tests and semantic review establish what it means.
