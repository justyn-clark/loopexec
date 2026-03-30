# JCN Worker Specification

## Definition

A JCN worker is a bounded execution role that takes a task plus constraints and produces a validated, auditable result.

In this repository, `jcn-worker` is currently a local worker-routing and replay prototype.

`jcn-worker` is not:

- a general assistant
- the orchestration system
- the governance protocol

## Current implementation in this repo

Implemented command surface:

- `jcn-worker version`
- `jcn-worker list`
- `jcn-worker status`
- `jcn-worker run <taskPath> [--policy <path>] [--registry <path>]`

Current runtime behavior:

1. Read a `worker-task` JSON file.
2. Read router policy JSON.
3. Read model registry JSON.
4. Deterministically choose a model and machine target.
5. Optionally override the routed model if `task.model` is set.
6. Call LM Studio chat completions at `http://localhost:1234` by default.
7. Write replay artifacts under `docs/jcn-agent-stack/runs/`.

## Runtime Contract

### Input

- `worker-task` file
- model registry JSON
- router policy JSON
- optional `JCN_LMSTUDIO_BASE_URL`

### Output

- selected `model_id`
- selected `machine_target`
- execution status
- replay artifacts containing hashes, timestamps, and transcript text

### Invariants

- deterministic route selection from declared inputs
- task JSON must contain `job_type`
- policy JSON must contain `job_type_priority` and `machine_priority`
- registry JSON must contain `models`
- no hidden route selection outside the declared files and environment override

## Lifecycle

Current `run` lifecycle:

1. Intake task.
2. Validate task, policy, and registry payloads.
3. Route model and machine.
4. Build deterministic prompt payload.
5. Call LM Studio local API.
6. Write run record and transcript artifacts.

The broader execution-loop and validation-gate story described elsewhere in the repo is still a target architecture, not fully implemented in `cmd/jcn-worker`.

## Relationships

- SMALL: governance and durable protocol state at the repo/workflow level.
- LoopExec: target deterministic execution loop.
- Toolbus: target tool routing and capability boundary.
- Musketeer: target multi-worker orchestration and queue management.

## Worker inventory in the current binary

The current built-in list returned by `jcn-worker list` is:

- `code-worker`
- `docs-worker`
- `infra-worker`
- `mindrail-worker`
- `reaper-worker`

These are the names the current binary actually exposes. Broader worker taxonomies may exist in architecture docs, but they are not implemented by this command yet.

## Task schema used by the prototype

Current task fields accepted by `cmd/jcn-worker`:

- `job_type`
- `repo_size`
- `latency_budget`
- `context_need`
- `tool_calling_needed`
- `prompt` (optional)
- `model` (optional explicit model override)
- `router_policy_path` (optional)
- `model_registry_path` (optional)

See:

- `docs/jcn-agent-stack/worker-task.example.json`
- `docs/jcn-agent-stack/router-policy.example.json`
- `docs/jcn-agent-stack/model-registry.example.json`

## Replay artifacts

A successful or failed run writes artifacts to:

- `docs/jcn-agent-stack/runs/<runId>.json`
- `docs/jcn-agent-stack/runs/<runId>.txt`

The JSON record includes:

- task path and SHA-256
- router policy path and SHA-256
- model registry path and SHA-256
- selected model id
- selected machine target
- LM Studio base URL
- prompt SHA-256
- response SHA-256 when available
- start/end timestamps
- final status and error text when present

## Validation gates

The full gate families discussed in the architecture docs remain aspirational for now.

Today, concrete enforcement in this repo comes from:

- JSON/file validation in `cmd/jcn-worker`
- Go unit tests
- repo CI (`gofmt`, `go vet`, `go test ./...`)

## Phase 2 replay command

Run from repo root:

```bash
JCN_LMSTUDIO_BASE_URL=http://localhost:1234 \
  go run ./cmd/jcn-worker run docs/jcn-agent-stack/worker-task.example.json
```

This replay command reuses the same task file and default router policy and model registry unless explicit overrides are provided.
