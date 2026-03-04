# JCN Local Models Routing Matrix

## Hardware Facts and Assumptions

Verified from current host:

- Host model: `Mac14,2`
- RAM: 16 GB

Assumptions for planning (explicit):

- Mac Studio M1 class: 64 GB RAM
- Mac mini Apple Silicon: 16 GB RAM

## LM Studio Runtime Notes

- Model catalog reference: https://lmstudio.ai/models
- Import structure reference: https://lmstudio.ai/docs/app/advanced/import-model
- System requirements reference: https://lmstudio.ai/docs/app/system-requirements

JCN uses LM Studio as a local runtime endpoint and model catalog manager. Keep model directories explicit and versioned by filename.

## Quantization Guidance

- Default: `Q4_K_M` for quality-memory balance.
- Constrained RAM: use `Q3` or IQ variants.
- On 16 GB machines: keep context windows modest for 14B+ class models.

## Job Class Routing by Machine Tier

### 1) Repo-wide code edits and refactors (SWE)

- Mac Studio 64 GB:
  - Qwen2.5-Coder-32B (`Q4_K_M`)
  - Codestral 22B (`Q4_K_M`)
  - Mistral Small 24B class (`Q4_K_M`)
- Mac mini 16 GB:
  - Qwen2.5-Coder-14B (`Q4_K_M` or `Q3`)
  - DeepSeek-Coder-V2-Lite-Instruct (`Q4_K_M`)
- MacBook Air M2 16 GB:
  - Qwen2.5-Coder-7B (`Q4_K_M`)
  - Llama 3.1 8B Instruct (`Q4_K_M`)

### 2) Tight code generation and patching

- Mac Studio 64 GB:
  - Qwen2.5-Coder-14B (`Q4_K_M`)
  - DeepSeek-Coder-V2-Lite-Instruct (`Q4_K_M`)
- Mac mini 16 GB:
  - Qwen2.5-Coder-7B (`Q4_K_M`)
  - Gemma 2 9B (`Q4_K_M`)
- MacBook Air M2 16 GB:
  - Qwen2.5-Coder-7B (`Q4_K_M`)
  - Phi-4 (`Q4_K_M`)

### 3) Reasoning and verification (math, logic)

- Mac Studio 64 GB:
  - Mistral Small 24B class (`Q4_K_M`)
  - Phi-4-reasoning (`Q4_K_M`)
- Mac mini 16 GB:
  - Phi-4-reasoning (`Q4_K_M` or reduced context)
  - Phi-4 (`Q4_K_M`)
- MacBook Air M2 16 GB:
  - Phi-4 (`Q4_K_M`)
  - Gemma 2 9B (`Q4_K_M`)

### 4) Summarization and indexing (Mindrail intake)

- Mac Studio 64 GB:
  - Mistral Small 24B class (`Q4_K_M`)
  - Llama 3.1 8B Instruct (`Q4_K_M` high context)
- Mac mini 16 GB:
  - Llama 3.1 8B Instruct (`Q4_K_M`)
  - Gemma 2 9B (`Q4_K_M`)
- MacBook Air M2 16 GB:
  - Llama 3.1 8B Instruct (`Q4_K_M`)
  - Phi-4 (`Q4_K_M`)

### 5) Planning and orchestration (Musketeer planner)

- Mac Studio 64 GB:
  - Mistral Small 24B class (`Q4_K_M`)
  - Qwen2.5-Coder-14B (`Q4_K_M`)
- Mac mini 16 GB:
  - Phi-4 (`Q4_K_M`)
  - Llama 3.1 8B Instruct (`Q4_K_M`)
- MacBook Air M2 16 GB:
  - Gemma 2 9B (`Q4_K_M`)
  - Phi-4 (`Q4_K_M`)

### 6) Multimodal support (optional)

- Mac Studio 64 GB:
  - Mistral Small (vision-capable variants where available)
  - Phi-4 multimodal-capable variants where available
- Mac mini 16 GB:
  - lightweight multimodal variants with reduced context
- MacBook Air M2 16 GB:
  - optional only, reduced resolution and context

## Deterministic Router Policy

Input:

- `job_type`
- `repo_size`
- `latency_budget`
- `context_need`
- `tool_calling_needed`

Output:

- `model_id`
- `machine_target`

Deterministic policy order:

1. Filter models by `job_type` capability tags.
2. Filter by machine RAM fit and quantization availability.
3. Filter by latency budget.
4. Prefer local machine affinity for active workspace.
5. Tie-break by fixed priority order in registry.

## Model Registry JSON Schema Draft

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "JCN Model Registry",
  "type": "object",
  "required": ["version", "machines", "models"],
  "properties": {
    "version": {"type": "string"},
    "machines": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "ram_gb", "role"],
        "properties": {
          "id": {"type": "string"},
          "ram_gb": {"type": "integer", "minimum": 1},
          "role": {"type": "string"}
        }
      }
    },
    "models": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "family", "quant", "memory_gb_estimate", "capabilities", "machine_allowlist", "priority"],
        "properties": {
          "id": {"type": "string"},
          "family": {"type": "string"},
          "quant": {"type": "string"},
          "memory_gb_estimate": {"type": "number", "minimum": 0},
          "capabilities": {"type": "array", "items": {"type": "string"}},
          "machine_allowlist": {"type": "array", "items": {"type": "string"}},
          "priority": {"type": "integer", "minimum": 1},
          "status": {"type": "string", "enum": ["active", "disabled"]}
        }
      }
    }
  }
}
```

## Model Source References

- https://huggingface.co/lmstudio-community/Qwen2.5-Coder-7B-Instruct-GGUF
- https://huggingface.co/bartowski/Qwen2.5-Coder-14B-Instruct-GGUF
- https://huggingface.co/lmstudio-community/DeepSeek-Coder-V2-Lite-Instruct-GGUF
- https://huggingface.co/bartowski/DeepSeek-Coder-V2-Lite-Instruct-GGUF
- https://lmstudio.ai/models/codestral
- https://lmstudio.ai/models/mistral-small
- https://lmstudio.ai/models/phi-4
- https://lmstudio.ai/models/microsoft/phi-4-reasoning
- https://lmstudio.ai/models/google/gemma-2-9b
- https://lmstudio.ai/blog/llama-3.1
- https://huggingface.co/lmstudio-community/Meta-Llama-3.1-8B-Instruct-GGUF
- https://lmstudio.ai/models/qwen/qwen2.5-coder-32b

## Hardware Discovery Command References

Run on each node:

- `system_profiler SPHardwareDataType`
- `sysctl hw.memsize`
