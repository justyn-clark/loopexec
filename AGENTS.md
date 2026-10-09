# HARD RULES
 Never directly edit .env files

## Code and documentation continuity

- LoopExec is a general-purpose bounded agentic loop governor. Project-specific
  quality standards belong to adapters, not the runtime's product definition.
- Runtime, CLI, dependency, tooling, workflow or policy changes MUST include
  matching canonical prose and CHANGELOG.md in the same change. Review behavior
  against docs; a path-co-change check alone cannot prove semantic correctness.
- Regenerate the CLI contract for command/flag/default/help changes, never hand
  edit it. Default tests MUST pass against the checked-in reference.
- Public documentation is allowlisted in docs/documentation-files.json. Add new
  public guides/release notes there; never bundle private handoffs or state.
- Releases MUST package the matching source-bound documentation and pass deployed
  docs readback before publication. Stale, missing or unreachable docs block
  release; do not bypass the gate to ship code alone.
- Follow docs/documentation-continuity.md. Never call local gates enforced on
  GitHub until committed workflows and required branch checks are verified live.
- Acceptance adjustments require an explicit owner decision and separate policy
  evidence. Never rewrite original receipts, silently lower an ideal target,
  reset shared allowances or launch more paid reviews to force convergence.

# SMALL Protocol Agent Guide

This is the operating contract for any AI agent or human acting like one. Follow it exactly. If anything here conflicts with `small --help`, the CLI wins.

SMALL governs state, not execution. The CLI is the only valid way to mutate `.small/`.

Target tooling: SMALL CLI 1.1.2 (supports protocol profiles 1.0.0 and 2.0.0).
This stable release includes command-summary correction and narrow captured-code
loopback URL handling required by this checkout's historical receipts. Verify the
resolved binary; older releases do not validate the retained QA template capture.
Preserve existing receipts. Review and retain only required non-secret command
proof in `.small-command-proofs/`; never export `.small-cache/` wholesale. New
truncated captures also require exact-byte proof before cache-free validation.
Local validation does not update the CI pin or prove hosted release gates.
This checkout remains on the v1 profile unless explicitly migrated through SMALL.
Check the resolved binary with `small version`; never use an old v1-only CLI to
write migrated v2 state.

Scope: all repos that claim SMALL compliance

---

## Zero-trust rules

- Never manually edit `.small/progress.small.yml`
- Never invent timestamps
- Never mark tasks complete with plan toggles
- Completion is only via `small checkpoint`
- Never run `small handoff` unless `small check --strict` passes
- Never assume chat memory; always read state from disk and CLI

---

## Artifact roles

- `.small/intent.small.yml` - purpose and scope
- `.small/constraints.small.yml` - hard limits and policy
- `.small/plan.small.yml` - planned tasks
- `.small/progress.small.yml` - append-only audit trail
- `.small/handoff.small.yml` - resume context
- `.small-runs/` - local run snapshots
- `.small-archive/` - local archived lineage

---

## Agent-safe command set

### Read-only

- `small version`
- `small status --json`
- `small emit --check --workspace root`
- `small doctor`
- `small run list`
- `small run diff`

### Plan and progress

- `small plan --add "Task title"`
- `small progress add --task <task-id> --status in_progress --evidence "..." --notes "..."`
- `small checkpoint --task <task-id> --status completed|blocked --evidence "..." --notes "..."`

### Execution capture

- `small apply --cmd "<command>" --task <task-id>`

### Gates and lifecycle

- `small check --strict`
- `small handoff --summary "..."`
- `small archive` (milestones only)

---

## Forbidden for agents

- Any timestamp override flags (`-at`, `-after`)
- Plan-only completion (`small plan --done`)
- Manual edits to `.small/*.yml` (except intent or constraints when explicitly authorized)
- Handoff before strict check passes

---

## Session lifecycle

Always begin clean. Always end with a gated handoff.

---

### Session start

Read artifacts on disk. Do not assume chat memory.

Then run:

```bash
small status --json
small check --strict
```

If strict fails, fix the workspace before any new work.

---

## Ralph loop

Repeat until all tasks are completed or blocked.

---

### Refresh state

```bash
small status --json
small check --strict
```

If strict fails, fix immediately.

---

### Select task

Choose a pending task with no unmet dependencies.

If none exists, add one:

```bash
small plan --add "Clear task title"
```

---

### Start task with evidence

```bash
small progress add \\
  --task <task-id> \\
  --status in_progress \\
  --evidence "Starting: what will be done" \\
  --notes "optional"
```

---

### Execute bounded changes

For any mutating command:

```bash
small apply --cmd "<command>" --task <task-id>
```

Read-only exploration may run outside `apply`.

Any command that changes files, deps, builds, tests, or migrations must use `apply`.

---

### Complete atomically

Completed:

```bash
small checkpoint \\
  --task <task-id> \\
  --status completed \\
  --evidence "What changed and why" \\
  --notes "optional"
```

Blocked:

```bash
small checkpoint \\
  --task <task-id> \\
  --status blocked \\
  --evidence "Why blocked and what is needed" \\
  --notes "optional"
```

---

### Enforce gate

```bash
small check --strict
```

Do not continue with a failing workspace.

---

## Session end

Always gate, then hand off:

```bash
small check --strict
small handoff --summary "Current state in one paragraph"
```

---

## Milestones only

When a meaningful milestone is reached:

```bash
small check --strict
small archive
```

---

## ReplayId threading

- Treat the replayId from `small status --json` as the primary key
- All progress and handoff context must match the active replayId
- Strict check should fail if replayId is missing or mismatched

---

## Evidence quality

Good evidence:

- Precise
- Names files or systems touched
- Explains why the change was made

Bad evidence:

- "Fixed it"
- "Build works now"

---

## Stop rules

Stop and block if:

- Intent or constraints are missing or unclear
- Secrets or external access are required
- A human decision is needed
- Strict check cannot pass

Blocked handoff example:

```bash
small checkpoint \\
  --task <task-id> \\
  --status blocked \\
  --evidence "Need decision on X before proceeding"

small check --strict
small handoff --summary "Blocked on decision X"
```

---

## Glossary

Agent

An LLM or automation acting through the SMALL CLI

Evidence

A factual description of what changed and where

Checkpoint

Atomic plan plus progress update with evidence

Strict check

The enforcement gate before any handoff

---

This contract exists to prevent drift. Follow it exactly.
