Note: This document describes planned behavior. Some commands may not yet be implemented.

# Spindle Architecture

This document describes the high-level architecture of Spindle.

---

## Overview

Spindle is a state-driven execution CLI. It reads state from SMALL artifacts, executes bounded work, and records outcomes atomically.

Spindle does not:
- Define state (SMALL does)
- Orchestrate workflows
- Replace CI or task runners
- Manage agents or models

Spindle does:
- Drive execution loops
- Enforce state gates
- Record evidence
- Enable safe resume

---

## Execution Loop

Spindle implements a controlled execution loop (Ralph loop) with hard state boundaries.

```
+-----------------+
|  Validate State |<---------+
|  (strict check) |          |
+--------+--------+          |
         |                   |
         v                   |
+--------+--------+          |
|  Select Task    |          |
|  (pending, deps)|          |
+--------+--------+          |
         |                   |
         v                   |
+--------+--------+          |
|  Mark In Progress|         |
|  (progress add) |          |
+--------+--------+          |
         |                   |
         v                   |
+--------+--------+          |
|  Execute Step   |          |
|  (substrate)    |          |
+--------+--------+          |
         |                   |
         v                   |
+--------+--------+          |
|  Checkpoint     |          |
|  (atomic record)|          |
+--------+--------+          |
         |                   |
         v                   |
+--------+--------+          |
|  Re-validate    |-----------+
|  (strict check) |
+-----------------+
```

Each iteration:
1. Validates workspace state via `small check --strict`
2. Selects the next actionable task (status: pending, dependencies met)
3. Marks the task in progress with evidence
4. Executes exactly one bounded command via a substrate
5. Records the outcome atomically via `small checkpoint`
6. Re-validates state before continuing

The loop terminates when no actionable tasks remain or a halt condition is triggered.

---

## State Gates

Spindle enforces state validity at defined boundaries.

### Strict Check

`small check --strict` is the primary gate. It validates:
- All SMALL artifacts are present and well-formed
- Schema compliance for each artifact
- Invariant rules (non-empty intent, valid constraints)
- ReplayId presence in handoff

Strict check runs:
- Before any loop iteration
- After each checkpoint
- Before handoff

If strict check fails, execution halts. The failure must be resolved before work continues.

### Checkpoint

`small checkpoint` atomically updates plan and progress. A checkpoint:
- Sets task status (completed or blocked)
- Records evidence of what changed
- Appends to the progress audit trail

Checkpoints are the only valid way to mark work complete. Plan-only toggles are forbidden.

---

## Substrate Boundary

A substrate is an execution environment where commands run.

### Supported Substrates

| Substrate | Description |
|-----------|-------------|
| Local shell | Default. Commands run in the current shell environment. |
| Container | Commands run in an isolated container. Environment is explicit. |
| Reproducible shell | Commands run in a reproducible environment (e.g., Nix devshell). |

### Substrate Contract

Substrates must:
- Accept a command string
- Execute the command
- Return exit code, stdout, stderr
- Not modify SMALL artifacts directly

The substrate is recorded as part of execution evidence. This makes the environment explicit and reproducible.

### Substrate Selection

Spindle selects the substrate based on:
- Workspace configuration
- Command-line flags
- Constraint directives

The default is local shell. Other substrates require explicit configuration.

---

## Failure Modes

Spindle halts execution under these conditions:

### Gate Failures

| Failure | Cause | Resolution |
|---------|-------|------------|
| Strict check fails | Invalid SMALL state | Fix artifacts, re-run check |
| Missing replayId | Handoff not initialized | Run `small handoff` |
| Schema violation | Malformed artifact | Fix schema errors |

### Execution Failures

| Failure | Cause | Resolution |
|---------|-------|------------|
| Command exits non-zero | Substrate command failed | Checkpoint as blocked, investigate |
| Constraint violation | Command violates constraint | Checkpoint as blocked, revise approach |
| Timeout | Command exceeded limit | Checkpoint as blocked, adjust or split task |

### Task Failures

| Failure | Cause | Resolution |
|---------|-------|------------|
| No actionable tasks | All tasks completed or blocked | Run handoff |
| Dependency cycle | Tasks depend on each other | Fix plan structure |
| Missing task | Referenced task not in plan | Add task via `small plan --add` |

When execution halts, Spindle records the halt reason. The next session can resume from the recorded state.

---

## Resume Semantics

Spindle supports safe resume via SMALL handoff and replayId.

### Handoff

`small handoff` generates a resume context containing:
- Summary of current state
- Current task (if in progress)
- Next steps (pending tasks)
- ReplayId for session continuity

Handoff runs only after strict check passes. This ensures the recorded state is valid.

### ReplayId

The replayId is a unique identifier for the current execution session. It:
- Links progress entries to a specific session
- Enables detection of state drift between sessions
- Provides an audit anchor for lineage tracking

ReplayId is generated automatically when handoff runs. All subsequent progress and checkpoint operations reference this replayId.

### Resume Flow

1. New session starts
2. Read handoff.small.yml to get replayId and context
3. Run `small check --strict` to validate state
4. Continue loop from current_task_id or next pending task
5. All new progress entries inherit the replayId

If the workspace state has changed outside the session (manual edits, other agents), strict check will fail. The state must be reconciled before resuming.

---

## Component Boundaries

```
+---------------------+
|      Spindle CLI    |
+----------+----------+
           |
           v
+----------+----------+
|     Loop Driver     |
|  (iteration logic)  |
+----------+----------+
           |
     +-----+-----+
     |           |
     v           v
+----+----+ +----+----+
| Selector| | Emitter |
| (task)  | | (json)  |
+---------+ +---------+
     |
     v
+----+----+
|Substrate|
| (exec)  |
+---------+
     |
     v
+----+----+
|  SMALL  |
| (state) |
+---------+
```

- **Loop Driver**: Orchestrates the execution loop. Calls selector, substrate, and SMALL CLI.
- **Selector**: Chooses the next actionable task based on status and dependencies.
- **Substrate**: Executes commands in the configured environment.
- **Emitter**: Produces JSON output for integrations.
- **SMALL**: External authority for all state operations.

Spindle never bypasses SMALL. All state reads and writes go through the SMALL CLI.
