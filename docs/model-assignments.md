# Model Assignments

Status: `0.5.0-rc.1` release-candidate source; GitHub Releases establishes
publication status. LoopExec remains a
provider-neutral governor. No model ranking, price catalog, SDK, automatic
routing or automatic escalation is included.

## Preview Without Spending

```bash
loopexec models plan --workflow workflow.json --workdir build/review
loopexec models plan --workflow workflow.json --workdir build/review --json
```

Planning explains the selected task's role/profile assignments, requested model,
effort, rationale, capabilities and assurance. It runs no hooks, initializes no
runtime state, reserves no calls and invokes no models. It validates model
policy, not project readiness, shared budget, executables or provider access.

Use configurable task-specific profiles, not a universal intelligence score.
Routine edits may suit an economical profile; difficult diagnosis may suit
deeper reasoning; image comparison requires declared vision capability. Evaluate
quality, latency and cost per accepted result, not just price per call.

## Configuration

Add `model_policy` to schema-1 workflow JSON. This is a fragment: existing check
and metering hooks are still required.

```json
{
  "model_policy": {
    "mode": "explicit",
    "task": "studio-review",
    "profiles": {
      "routine": {
        "provider": "configured-provider",
        "model_id": "configured-small-model",
        "reasoning_effort": "low"
      },
      "visual-review": {
        "provider": "configured-provider",
        "model_id": "configured-vision-model",
        "reasoning_effort": "high",
        "capabilities": ["vision"]
      }
    },
    "tasks": {
      "studio-review": {
        "roles": {
          "builder": {
            "profile": "routine",
            "reason": "Bounded routine preparation"
          },
          "critic": {
            "profile": "visual-review",
            "requires": ["vision"],
            "reason": "Compare saved equipment captures with references"
          }
        },
        "command": [
          "node", "adapter/attempt.mjs",
          "--builder-model", "{builder.model}",
          "--builder-effort", "{builder.reasoning}",
          "--critic-model", "{critic.model}",
          "--critic-effort", "{critic.reasoning}"
        ]
      }
    }
  }
}
```

The command is an operator-provided adapter, not a bundled provider launcher. It
must accept these arguments plus the final request-file argument and apply the
values to its actual child calls. Unsupported settings must fail, never fall
back silently. Effort values are provider-specific portable identifiers, not a
built-in provider capability database. Optional profile `version` pins the
adapter-reported version when supplied.

`advisory` mode records guidance and leaves the existing workflow `exec`
unchanged. It makes no identity-compliance claim.

`explicit` mode uses the selected task's `command`. Omit workflow `exec` and
legacy `--exec`; conflicting overrides fail before work. Each assigned model and
nonempty effort must have whole argv placeholders `{role.model}` and
`{role.reasoning}`. Optional placeholders are `{role.provider}` and
`{role.version}` when nonempty. No shell-text interpolation or partial-argument
substitution occurs. The existing absolute request-file path is appended last.
Credentials never belong in profiles, commands, reasons or receipts. Prefer
portable relative paths for configured executables and resources.

All tasks are validated. Missing profiles, modes, declared capabilities, bound
explicit fields and safe display text fail closed. Select a different task in
configuration before a new run. Resume requires identical policy, including
assignments, rationale and command. There are no automatic fallbacks.

## Adapter Evidence

Every hook receives `model_selection`: `mode`, `task`, `assurance`, and sorted
`roles`. Each role includes profile, provider, model ID, optional version/effort,
required/declared capabilities and reason. State, reports, final run output and
typed per-attempt receipt events expose the selection. Human runs print it before
each attempt; JSON mode retains one stdout object and exposes the live selection
through request/state/receipt files. Use `models plan` for a pre-launch JSON view.

Explicit mode requires a reserve hook even for unmetered workflows. Every role
needs a planned call with that role as `phase` and a matching `model` object:

```json
{
  "id": "review/000001/exec/critic",
  "phase": "critic",
  "max_tokens": 1000,
  "model": {
    "provider": "configured-provider",
    "model_id": "configured-vision-model",
    "reasoning_effort": "high"
  }
}
```

Usage calls include the same identity fields alongside existing status, money
and tokens. Report observed invocation values, not an unchecked copy of the
request. Executed assigned calls require matching identity; skipped calls may
omit it. Unassigned model roles are rejected; non-model tool phases remain
allowed. A configured version must match; an unspecified version may be reported.
Missing/wrong planned identities stop before work. Reported drift halts with
`model_drift_detected` (19), retaining valid incurred usage. Unknown dollar costs
remain unknown. Budget/call/token accounting still uses the existing contract.

Assurance is `guidance_only` or `adapter_report_required`, not independent
provider attestation. Capabilities are operator declarations. A malicious adapter
can lie or make undeclared calls; existing provider/containment enforcement is
still necessary. Legacy model flags remain recording-only and cannot be combined
with `model_policy`. Selection and reported usage are covered by existing receipt
attestation; old workflows and signatures omit new fields and remain compatible.

## Quality and Consumer Testing

Assignments never add retries, grant calls, renew allowances, reset ledgers,
launch extra critics or lower quality targets. LoopExec alone owns the loop; an
adapter performs one bounded sequence. Run technical checks before critics and
keep fresh independent read-only review boundaries. Check/score hooks replay
saved evidence without model work. A stronger model cannot approve a red check
or waive an unmet acceptance target.

The 3D Home World studio-hardware review is a planned consumer test, not a global
quality standard. Its adapter must deliberately consume assignments and report
actual invocation settings while preserving readiness gates and per-asset spend
history. A LoopExec update alone does not update that adapter or authorize another
critic. Do not change a pinned in-progress run. New runs require appropriate
approval and retain existing shared allowances and original receipts.

```bash
go build -trimpath -o build/loopexec ./cmd/loopexec
go build -trimpath -o build/model-assignment ./examples/model-assignment
bash scripts/model-assignment-proof.sh build/loopexec build/model-assignment build/model-proof
```

This proof uses synthetic builder/critic models, no engines, images, credentials
or paid provider. It covers two-role argv dispatch, no-spend planning, drift,
budget/technical denial, resume, replay and attestation. It is not a visual score
or a live provider test.
