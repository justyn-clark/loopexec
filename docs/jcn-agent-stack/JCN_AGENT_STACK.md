# JCN Agent Stack

## Purpose

This document defines the canonical JCN worker architecture and boundaries for local multi-agent operations.

## Layer Model

- L1 Infrastructure: host machines, storage, runtimes, local network, CI runners.
- L2 Protocol (SMALL): governance artifacts, intent/constraints/plan/progress/handoff, audit invariants.
- L3 Tool Routing (Toolbus): capability boundary, tool policy, tool selection and execution contracts.
- L4 Execution (LoopExec): deterministic bounded step execution and replayable loop discipline.
- L5 Orchestration (Musketeer): multi-role or multi-worker coordination, job routing, queue policy.
- L6 Agents and Workers: Codex, Claude, Gemini, local models, Ace control surfaces, and `jcn-worker` roles.
- L7 Interfaces: operator-facing channels like Ace Telegram and Discord.

## Operational Flow

Interfaces -> Musketeer -> Worker Queue -> jcn-worker -> LoopExec -> Toolbus -> SMALL -> Infrastructure

## Stripe Mapping to JCN

Stripe concepts are mapped as follows:

- Devboxes -> local repo workspaces and isolated run roots.
- Context builder -> deterministic context assembly from SMALL artifacts plus repo state.
- Blueprints -> JCN `worker-blueprint` definitions that choose tools, model policy, budgets, and gates.
- Validation gates -> JCN lint/test/build/policy/evidence gates before completion.
- PR generation -> worker output contract can target commit-ready changesets and PR-ready evidence.

In JCN, blueprints are repeatable workflows represented as code-defined or declarative specs. SMALL remains governance and system-of-record state; blueprints do not replace SMALL.

## jcn-worker Placement

`jcn-worker` is the worker runtime contract and role launcher. It is not a protocol and not the orchestrator. It executes bounded work under LoopExec, uses Toolbus for capabilities, and reports durable evidence through SMALL-governed workflows.

## Goose vs Pi in the Stack

- Goose can be used as an on-machine execution adapter and tool integration layer, often spanning L4 and L5 concerns.
- Pi is an agent toolkit surface that includes unified model APIs and coding-agent-oriented interfaces.
- Constraint: Goose or Pi may be integrated behind Toolbus or orchestration layers, but neither is the record of truth. SMALL plus LoopExec remain authoritative for governed execution state.

## Mindrail and chat-notebook Integration

- `chat-notebook` SQLite is a high-volume intake and memory substrate.
- Intake adapters convert notebook rows and metadata into event streams and search indexes.
- Mindrail consumes indexed memory artifacts for retrieval, summarization, and routing context.
- Primary storage posture is local-first vault operation on Apple Silicon hosts, especially Mac mini service nodes.

## Worker Naming Standard

- Official term: worker
- Runtime command: `jcn-worker`
- Role naming: `<domain>-worker`
- Pipeline staging names are allowed, but inventory defaults to domain workers.

## Required References

- Stripe Part 1: https://stripe.dev/blog/minions-stripes-one-shot-end-to-end-coding-agents
- Stripe Part 2: https://stripe.dev/blog/minions-stripes-one-shot-end-to-end-coding-agents-part-2
- Goose: https://github.com/block/goose
- Pi mono: https://github.com/badlogic/pi-mono
