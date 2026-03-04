# JCN Worker Specification

## Definition

A JCN worker is a bounded execution role that takes a task plus constraints and produces a validated, auditable change set.

`jcn-worker` is not:

- a general assistant
- the orchestration system
- the governance protocol

## Runtime Contract

### Input

- `worker-task` file
- model registry
- router policy
- repo state and constraints from governed workflow context

### Output

- selected `model_id`
- selected `machine_target`
- execution status
- evidence references for gates

### Invariants

- deterministic route selection from declared inputs
- worker name must match `<domain>-worker`
- no implicit tool access outside Toolbus boundaries
- no completion without required validation gate evidence

### Lifecycle

1. Intake task.
2. Validate worker name and task schema.
3. Route model and machine.
4. Execute bounded step(s) via LoopExec discipline.
5. Run validation gates.
6. Package evidence and handoff artifacts.

## Relationships

- SMALL: governance and durable protocol state.
- LoopExec: deterministic execution loop.
- Toolbus: tool routing and capability policy boundary.
- Musketeer: multi-worker orchestration and queue management.

## Worker Types

### Engineering

- `code-worker`
- `repo-worker`
- `refactor-worker`
- `test-worker`
- `docs-worker`

### Data and Knowledge

- `mindrail-worker`
- `index-worker`
- `embed-worker`
- `search-worker`
- `summarize-worker`

### Infrastructure

- `infra-worker`
- `deploy-worker`
- `monitor-worker`
- `cron-worker`

### Multimedia and Game

- `reaper-worker`
- `audio-worker`
- `video-worker`
- `render-worker`
- `godot-worker`
- `sprite-worker`
- `shader-worker`
- `asset-worker`
- `atlas-worker`

## Validation Gates

Required gate families:

- lint
- test
- build
- policy checks
- evidence packaging

A worker run is not complete until required gates pass and evidence is attached.

## Worker Blueprints

A `worker-blueprint` defines repeatable workflow selection for a worker role:

- model preferences and fallback order
- tool allowlist
- budget limits
- required validation gates

Example `code-worker` blueprint:

```yaml
name: code-worker
model: qwen2.5-coder-14b
tools:
  - git
  - filesystem
  - tests
  - lint
budgets:
  max_input_tokens: 64000
  max_output_tokens: 8000
gates:
  - build
  - lint
  - test
```

## Terminology

- `jcn-worker`
- `worker-blueprint`
- `worker-task`
- `worker-run`
- `worker-cluster`
- `worker-queue`

## Phase 2 Replay Command

Run from repo root:

`JCN_LMSTUDIO_BASE_URL=http://localhost:1234 go run ./cmd/jcn-worker run docs/jcn-agent-stack/worker-task.example.json`

This replay command reuses the same task file and default router policy and model registry.
