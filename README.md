> Note: This document describes planned behavior. Some commands and integrations may not yet be implemented.

# loopexec

loopexec is a state-driven execution CLI designed to run bounded work loops against repositories governed by SMALL.

It does not replace agents, models, CI systems, or task runners.
It enforces how work is executed, recorded, compacted, and resumed.

loopexec exists to make AI-assisted and human-assisted work durable, auditable, reproducible, interrupt-safe, and token-efficient.


---

## Why loopexec Exists

Modern agent tooling is good at generating actions.
It is bad at maintaining state and execution discipline.

Common failure modes:

Plans drift from reality

Work completes without evidence

Context windows reset or balloon

Execution boundaries are implicit

Handoffs are informal or missing

Token usage grows without bound


SMALL solves state.
loopexec solves execution discipline and loop control.

Together, they enable:

Bounded, step-by-step execution

Deterministic validation before and after each step

State as the source of truth (not chat history)

Safe interruption and resumption

Measurable, comparable token usage



---

## What loopexec Is

loopexec is a CLI that:

Reads authoritative state from .small/

Selects the next actionable task

Executes exactly one bounded step

Writes results atomically via SMALL checkpoints

Re-validates state after execution

Emits structured telemetry for observation

Repeats until work is complete, blocked, or halted


At its core, loopexec implements a controlled execution loop (commonly called a Ralph loop) with hard state boundaries.


---

## What loopexec Is Not

loopexec is not:

An AI model

An agent framework

A workflow orchestrator

A CI replacement

A general task runner (make, just, task, etc.)


loopexec composes with these tools instead of replacing them.


---

# SMALL and loopexec: Clear Separation of Concerns

## Layer	Responsibility

SMALL	State, invariants, lineage
loopexec	Execution loop and discipline
Agent	Proposes actions
Human	Defines intent, constraints, approvals
CI	Post-commit validation
Observer	Visibility and analytics


If SMALL is the ledger,
loopexec is the disciplined operator.


---

## Core Principles

1. State Is Authoritative

Conversation history is ephemeral.
Logs are insufficient.

loopexec treats .small/ artifacts as the only source of truth.

Every execution step must:

Respect intent and constraints

Correspond to a plan task

Produce evidence in progress

Be resumable via handoff



---

2. One Step at a Time

loopexec enforces single-step execution.

Each loop iteration:

Executes one bounded command

Produces one checkpoint

Mutates state once


This makes failures obvious, retries safe, audits trivial, and compaction possible.


---

3. Human on the Loop, Not in the Loop

loopexec supports:

Humans

AI agents

Or both together


Humans define intent and constraints.
Agents propose actions.
loopexec enforces boundaries and records outcomes.

No invisible work.
No silent failures.


---

4. Bounded Context (Compaction by Design)

loopexec never hands an agent the entire repo history.

Instead, it generates a bounded context packet per step:

Current SMALL state (status JSON)

Active plan slice only

Recent progress slice only

One-step objective

Execution policy and stop conditions


This is the execution equivalent of mallocing a fixed buffer:

No unbounded growth

No repeated reallocation

Predictable token usage



---

## Human Workflow (Manual Execution)

loopexec is intentionally usable without any agent.

A human-only loop looks like:

1. Define intent and constraints
(via files or small init + edits)


2. Add plan tasks

small plan --add "Implement feature X"


3. Validate state

small check --strict


4. Mark task in progress

small progress add --task task-1 --status in_progress


5. Execute work manually

Edit code

Run tools

Run tests



6. Record outcome

small checkpoint --task task-1 --status completed --evidence "Tests pass"


7. Re-validate

small check --strict


8. Generate handoff when stopping

small handoff --summary "Implemented feature X"



This is loopexec by hand.
The CLI simply automates and enforces this discipline.


---

## Agent Workflow (Runner-Driven)

In an agent-driven loop:

loopexec selects the task

loopexec generates the bounded context packet

The agent receives only that packet

The agent proposes an action

loopexec executes via small apply

loopexec checkpoints and re-validates


The agent never mutates state directly.


---

### Relationship to Agents and Plugins

Agents do not replace loopexec.

Agents are execution advisors.

Possible integrations:

Claude Code plugin

Future Codex plugin

Custom loop systems

CI-adjacent runners


### All integrations must obey this rule:

> SMALL governs state.
loopexec governs execution.
Agents propose, never mutate.




---

## Bring Your Own Loop (BYO)

If you already have a loop system:

Use SMALL as your state layer

Implement the Ralph loop yourself

Call SMALL commands explicitly

Generate your own bounded context packets

Emit telemetry alongside SMALL


loopexec is the reference implementation, not a monopoly.


---

## Execution Substrates

loopexec supports pluggable execution substrates:

Local shell

Containers

Reproducible dev shells (e.g. Nix)

Remote runners (future)


The substrate is part of the evidence.


---

## Token and Cost Accounting (Planned)

loopexec can optionally record per-step telemetry:

Input tokens

Output tokens

Cached tokens (if available)

Estimated cost

Wall-clock time


### This enables side-by-side proof:

Free-form agent loops

loopexec-bounded loops


Same task. Same repo. Measurable difference.


---

## Observer and Visibility (Planned)

Telemetry is written outside .small/:

.loopexec/
  runs/
    <replayId>/
      run.json
      steps/
      packets/

This enables:

TUI (ratatui)

Web dashboards

CSV exports

Comparative analysis


Observer tools are read-only.


---

## Why This Is Not CI

CI answers: Is this code acceptable after the fact?
loopexec asks: Is this step valid to execute right now?

Use both. They solve different problems.


---

## Who This Is For

loopexec is for developers who:

Use AI agents seriously

Care about correctness and auditability

Want work to survive context loss

Want measurable efficiency, not vibes

Prefer explicit systems over magic



---

## Final Note

loopexec is not flashy by design.

It exists so everything else can be.
