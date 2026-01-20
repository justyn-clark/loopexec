# Spindle

Spindle is a state-driven execution CLI designed to run bounded work loops against SMALL-governed repositories.

It does not replace agents, models, CI systems, or task runners.
It enforces **how work is executed, recorded, and resumed**.

Spindle exists to make AI-assisted and human-assisted work **durable, auditable, reproducible, and interrupt-safe**.

---

## Why Spindle Exists

Modern agent tooling is good at generating actions.
It is bad at maintaining state.

- Plans drift from reality
- Work completes without evidence
- Context windows reset
- Execution environments are implicit
- Handoffs are informal or missing

SMALL solves **state**.
Spindle solves **execution discipline**.

Together, they create a system where:
- Work happens in bounded steps
- Each step is validated before and after execution
- State is authoritative, not chat history
- Execution can stop and resume safely at any time

---

## What Spindle Is

Spindle is a CLI that:

- Reads project state from `.small/`
- Selects the next actionable task
- Executes **exactly one bounded step**
- Records results atomically using SMALL checkpoints
- Repeats until work is complete, blocked, or halted

At its core, Spindle implements a controlled execution loop
(often called a "Ralph loop") with hard state boundaries.

---

## What Spindle Is Not

Spindle is **not**:

- An AI model
- An agent framework
- A workflow orchestrator
- A CI replacement
- A task runner like make, just, or task

Spindle **composes** with these tools instead of replacing them.

---

## Core Principles

### 1. State Is the Source of Truth

Conversation history is ephemeral.
Logs are insufficient.
Spindle treats `.small/` artifacts as authoritative.

Every execution step must:
- Respect intent and constraints
- Be represented in plan
- Be evidenced in progress
- Be resumable via handoff

---

### 2. One Step at a Time

Spindle enforces **single-step execution**.

Each loop iteration:
- Executes one command
- Produces one checkpoint
- Mutates state once

This makes failures obvious, retries safe, and audits possible.

---

### 3. Human on the Loop, Not in the Loop

Spindle is designed for:
- AI agents
- Humans
- Or both together

Humans define intent and constraints.
Agents propose actions.
Spindle enforces boundaries and records outcomes.

No invisible work.
No silent failures.

---

### 4. Reproducible Substrates

Execution should not depend on ambient machine state.

Spindle supports pluggable execution substrates, such as:
- Local shell
- Containerized environments
- Reproducible dev shells (e.g. Nix)

The execution environment becomes part of the evidence.

---

## How Spindle Works (High Level)

A typical loop looks like:

1. Validate state

small check --strict

2. Select next actionable task
- status: pending
- dependencies met (if used)

3. Mark task in progress

small progress add --task <id> --status in_progress

4. Execute one bounded command
- local shell
- container
- reproducible environment

5. Record outcome atomically

small checkpoint --task <id> --status completed --evidence "..."

6. Re-validate state

small check --strict

7. Repeat until no actionable tasks remain

---

## Relationship to SMALL

Spindle does not redefine SMALL.
It enforces it.

| SMALL | Spindle |
|------|---------|
| Defines state | Drives execution |
| Stores intent | Respects intent |
| Stores constraints | Enforces constraints |
| Records progress | Writes checkpoints |
| Enables handoff | Requires handoff |

If SMALL is the ledger,
Spindle is the disciplined operator.

---

## Repository Structure (Planned)

spindle/ ├── cmd/ │   └── spindle/ │       └── main.go ├── internal/ │   ├── loop/          # Execution loop logic │   ├── substrate/     # Execution adapters (local, nix, etc.) │   ├── selector/      # Task selection logic │   └── emit/          # Integration output (json) ├── docs/ │   ├── architecture.md │   ├── loop-contract.md │   └── integrations.md ├── examples/ │   └── nix/ ├── .small/ │   ├── intent.small.yml │   ├── constraints.small.yml │   ├── plan.small.yml │   ├── progress.small.yml │   └── handoff.small.yml └── README.md

---

## CLI Philosophy

Spindle is intentionally minimal.

Expected commands (subject to iteration):

- `spindle status`
- `spindle run`
- `spindle loop`
- `spindle emit --json`
- `spindle substrate list`

Anything more complex belongs in:
- SMALL
- make / just
- CI systems
- External orchestration

---

## Current Status

Spindle is under active development.

Initial goals:
- Local substrate execution
- SMALL strict-gated loop
- Deterministic checkpointing
- JSON output for integrations

This repo is governed by SMALL and is intentionally strict.
State correctness is more important than speed.

---

## Who This Is For

Spindle is for developers who:
- Use AI agents seriously
- Care about correctness and auditability
- Want work to survive context loss
- Prefer explicit systems over magic

If that sounds like you, you are in the right place.

---

## License

TBD.

---

## Final Note

Spindle is not flashy by design.

It exists to make everything else safer, calmer, and more trustworthy.

Say the word and we proceed.
