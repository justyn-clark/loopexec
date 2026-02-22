# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

loopexec is a state-driven execution CLI designed to run bounded work loops against SMALL-governed repositories. It enforces execution discipline, durability, and auditability for AI-assisted and human-assisted work.

loopexec is **not** an AI model, agent framework, or task runner - it composes with these tools by enforcing how work is executed, recorded, and resumed.

## SMALL Protocol Compliance

This repository is SMALL-governed. All work must follow the SMALL protocol defined in AGENTS.md.

### Critical Rules
- Never manually edit `.small/*.yml` files (except intent/constraints when explicitly authorized)
- Never edit `.env` files directly
- Completion is only valid via `small checkpoint`
- Never run `small handoff` unless `small check --strict` passes
- Always read state from disk and CLI, never assume chat memory

### Session Lifecycle
```bash
# Session start - always begin with state validation
small status --json
small check --strict

# Task execution (Ralph loop)
small progress add --task <task-id> --status in_progress --evidence "..."
small apply --cmd "<command>" --task <task-id>
small checkpoint --task <task-id> --status completed --evidence "..."
small check --strict

# Session end - always gate then handoff
small check --strict
small handoff --summary "Current state in one paragraph"
```

### Evidence Quality
Evidence must be precise, name files/systems touched, and explain why changes were made. Avoid vague descriptions like "fixed it" or "build works now".

## Architecture (Planned)

loopexec will be implemented in Go with this structure:
- `cmd/loopexec/` - CLI entry point
- `internal/loop/` - Execution loop logic
- `internal/substrate/` - Execution adapters (local shell, Nix, containers)
- `internal/selector/` - Task selection logic
- `internal/emit/` - Integration output (JSON)

## CLI Commands (Planned)

- `loopexec status` - Show current state
- `loopexec run` - Execute one step
- `loopexec loop` - Run bounded execution loop
- `loopexec emit --json` - Output for integrations
- `loopexec substrate list` - List available execution substrates
