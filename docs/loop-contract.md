Note: This document describes planned behavior. Some commands may not yet be implemented.

# Loop Contract Specification

This document defines the normative contract for loopexec execution loops.

Version: 1.0.0

---

## Terminology

- **Loop**: A bounded sequence of task executions governed by SMALL state.
- **Iteration**: One cycle of select-execute-checkpoint.
- **Gate**: A validation checkpoint that MUST pass before proceeding.
- **Halt**: Termination of the loop due to completion or failure.

The key words "MUST", "MUST NOT", "SHOULD", "SHOULD NOT", and "MAY" in this document are to be interpreted as described in RFC 2119.

---

## Preconditions

Before starting a loop, the following preconditions MUST be satisfied:

### P1: Workspace Exists

A `.small/` directory MUST exist in the workspace root.

### P2: Artifacts Present

The following artifacts MUST be present and non-empty:
- `intent.small.yml`
- `constraints.small.yml`
- `plan.small.yml`
- `progress.small.yml`
- `handoff.small.yml`

### P3: Strict Check Passes

`small check --strict` MUST return exit code 0.

If strict check fails, the loop MUST NOT start. The failure MUST be resolved first.

### P4: ReplayId Present

`handoff.small.yml` MUST contain a valid replayId.

If replayId is missing, run `small handoff` before starting the loop.

### P5: Intent Non-Empty

`intent.small.yml` MUST contain a non-empty intent string.

---

## Iteration Lifecycle

Each iteration MUST follow this exact sequence:

### Step 1: Validate State

```
small check --strict
```

The loop MUST run strict check at the start of every iteration.

If strict check fails:
- The iteration MUST NOT proceed
- The loop MUST halt
- The failure reason MUST be recorded

### Step 2: Select Task

The loop MUST select exactly one task meeting these criteria:
- Status is `pending`
- All dependencies (if any) are satisfied

If no task meets these criteria:
- If all tasks are `completed`: proceed to normal halt
- If tasks are `blocked` with no pending tasks: proceed to blocked halt
- Otherwise: the plan is malformed and the loop MUST halt with error

The loop MUST NOT execute multiple tasks in a single iteration.

### Step 3: Mark In Progress

```
small progress add --task <task-id> --status in_progress --evidence "<reason>"
```

The loop MUST mark the selected task as in_progress before execution.

The evidence field MUST describe what will be done.

The loop MUST NOT execute commands before marking in_progress.

### Step 4: Execute Command

The loop MUST execute exactly one bounded command via the configured substrate.

Execution requirements:
- The command MUST be captured (stdout, stderr, exit code)
- The command MUST respect workspace constraints
- The command MUST NOT modify SMALL artifacts directly

For mutating commands, use:
```
small apply --cmd "<command>" --task <task-id>
```

Read-only exploration MAY run outside `apply`.

### Step 5: Evaluate Outcome

After execution, the loop MUST evaluate the outcome:

| Condition | Classification |
|-----------|---------------|
| Exit code 0, objective met | Completed |
| Exit code 0, objective not met | Blocked |
| Exit code non-zero | Blocked |
| Constraint violated | Blocked |
| Timeout exceeded | Blocked |

The loop MUST NOT mark a task as completed if the objective was not met.

### Step 6: Checkpoint

For completed tasks:
```
small checkpoint --task <task-id> --status completed --evidence "<what changed>"
```

For blocked tasks:
```
small checkpoint --task <task-id> --status blocked --evidence "<why blocked>"
```

The loop MUST checkpoint after every execution.

The loop MUST NOT skip checkpointing.

The evidence field MUST:
- Name files or systems changed
- Explain why the change was made
- Be precise and verifiable

The evidence field MUST NOT:
- Be vague (e.g., "fixed it", "done")
- Omit relevant details
- Include speculation

### Step 7: Re-validate State

```
small check --strict
```

After checkpointing, the loop MUST run strict check again.

If strict check fails:
- The loop MUST halt
- The failure MUST be recorded
- The next session MUST resolve the failure before resuming

### Step 8: Continue or Halt

If strict check passes and actionable tasks remain: return to Step 1.

If no actionable tasks remain: proceed to halt.

---

## Halt Conditions

The loop MUST halt under these conditions:

### Normal Halt

All tasks in the plan have status `completed`.

On normal halt:
1. Run `small check --strict`
2. Run `small handoff --summary "<completion summary>"`

### Blocked Halt

No tasks are actionable (all remaining tasks are `blocked` or have unmet dependencies).

On blocked halt:
1. Run `small check --strict`
2. Run `small handoff --summary "<blocked summary with reasons>"`

### Error Halt

A gate failure or invariant violation occurred.

On error halt:
1. Record the error reason
2. Do NOT run handoff (state may be invalid)
3. The error MUST be resolved before the next session

### Interrupt Halt

External signal received (SIGINT, SIGTERM) or manual stop.

On interrupt halt:
1. If mid-execution: checkpoint as blocked with evidence "interrupted"
2. Run `small check --strict`
3. If strict passes: run `small handoff --summary "<interrupted summary>"`
4. If strict fails: record failure, do not handoff

---

## Forbidden Operations

The loop MUST NOT:

### F1: Skip Gates

The loop MUST NOT proceed past a failed strict check.

### F2: Skip Checkpoints

The loop MUST NOT complete an iteration without checkpointing.

### F3: Batch Executions

The loop MUST NOT execute multiple commands per iteration.

### F4: Modify SMALL Directly

The loop MUST NOT write to `.small/*.yml` files directly. All mutations MUST go through the SMALL CLI.

### F5: Assume Memory

The loop MUST NOT rely on in-memory state between iterations. State MUST be read from disk.

### F6: Override Timestamps

The loop MUST NOT use timestamp override flags (`-at`, `-after`).

### F7: Plan-Only Completion

The loop MUST NOT use `small plan --done` to mark tasks complete. Completion is only via `small checkpoint`.

---

## Invariants

These invariants MUST hold at all times:

### I1: Single In-Progress

At most one task MAY have status `in_progress` at any time.

### I2: Progress Append-Only

`progress.small.yml` is append-only. Entries MUST NOT be deleted or modified.

### I3: Checkpoint Atomicity

A checkpoint MUST update both plan status and progress in one operation.

### I4: ReplayId Continuity

All progress entries within a session MUST reference the same replayId.

### I5: Evidence Required

Every progress entry and checkpoint MUST include non-empty evidence.

---

## Error Recovery

When recovering from errors:

1. Read `handoff.small.yml` to understand last known state
2. Run `small status --json` to see current state
3. Run `small doctor` to diagnose issues
4. Fix identified issues
5. Run `small check --strict` to validate fixes
6. If strict passes, resume loop
7. If strict fails, repeat from step 3
