# JCN Repository Map

This map uses discovered local paths under:

`/Users/justin/Documents/Justyn Clark Network/REPOS`

## Registry

```yaml
repos:
  - id: small-protocol
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/small/small-protocol
  - id: small-docs-site
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/small/smallprotocol.dev
  - id: loopexec
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/loopexec
  - id: toolbus-spike
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/toolbus
  - id: jcn-toolbus-wrapper
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/jcn-toolbus
  - id: musketeer
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/musketeer
  - id: mindrail
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/mindrail.dev
  - id: chat-notebook
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/chat-notebook
    db_path: /Users/justin/Documents/Justyn Clark Network/REPOS/chat-notebook/chat-notebook.db
  - id: reaper-studio
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/reaper-studio
  - id: jcn-game-dev-ops
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/jcn-game-dev-ops
  - id: jcn-pai
    path: /Users/justin/Documents/Justyn Clark Network/REPOS/jcn-pai
    note: discovered candidate for jcnbot-equivalent Telegram ingress
```

## Repo Details

### small-protocol

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/small/small-protocol`
- Purpose: SMALL governance protocol and canonical schemas/artifacts.
- Primary entrypoints: `cmd/small/main.go`
- Run/test/lint:
  - `go test ./...` (verified by Go module presence)
  - `make` tasks are available (`Makefile` present)
- Docs: `docs/`, `spec/`
- Governance files: `AGENTS.md`

### small-docs-site

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/small/smallprotocol.dev`
- Purpose: SMALL documentation/spec site.
- Primary entrypoints: docs site scripts and `package.json` commands.
- Run/test/lint:
  - `bun install`
  - `bun run sync:small-protocol-docs`
  - `bun dev`
- Docs: `docs/`, `spec/`
- Governance files: `AGENTS.md`

### loopexec

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/loopexec`
- Purpose: deterministic execution loop engine.
- Primary entrypoints: `cmd/loopexec/main.go`
- Run/test/lint:
  - `go test ./...`
- Docs: `docs/`
- Governance files: `AGENTS.md`, `CLAUDE.md`

### toolbus-spike

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/toolbus`
- Purpose: tool routing spike as Pi extension wrapper for TBE tools.
- Primary entrypoints: `src/index.ts`
- Run/test/lint:
  - `npm install`
  - `npm run build`
  - `npm run check`
- Docs: `README.md`, `SPIKE-DECISION.md`
- Governance files: none discovered

### jcn-toolbus-wrapper

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/jcn-toolbus`
- Purpose: JCN wrapper repo containing extension assets and vendored experiments.
- Primary entrypoints: `jcn-toolbus-extension/` package surfaces.
- Run/test/lint:
  - inspect `jcn-toolbus-extension/package.json` for package scripts
- Docs: `jcn-toolbus-extension/README.md`
- Governance files: `AGENTS.md`

### musketeer

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/musketeer`
- Purpose: local-first orchestration harness with role separation.
- Primary entrypoints: `src/main.rs`, `src/lib.rs`
- Run/test/lint:
  - `cargo build`
  - `cargo fmt`
  - `cargo test`
- Docs: `docs/`
- Governance files: `CLAUDE.md`

### mindrail

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/mindrail.dev`
- Purpose: cognitive capture and deterministic routing system, SMALL-governed reference.
- Primary entrypoints: README-defined architecture only (no runtime source files discovered in repo root).
- Run/test/lint:
  - no runnable commands discovered in current repo snapshot
- Docs: `README.md`
- Governance files: `AGENTS.md`

### chat-notebook

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/chat-notebook`
- Purpose: OpenAI export intake into canonical SQLite notebook with query and serve commands.
- Primary entrypoints: `cmd/chat-notebook/main.go`
- Run/test/lint:
  - `go build -o target/chat-notebook ./cmd/chat-notebook`
  - `./target/chat-notebook import ...`
  - `./target/chat-notebook inspect stats ...`
  - `./target/chat-notebook serve ...`
- Docs: `README.md`, `SPEC.md`
- Governance files: none discovered

### reaper-studio

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/reaper-studio`
- Purpose: REAPER CLI knowledge and search control plane.
- Primary entrypoints: `src/cli/index.ts`
- Run/test/lint:
  - `npm run build`
  - `npm run lint`
  - `npm run test`
- Docs: `README.md`, `sources/README.md`
- Governance files: `AGENTS.md`, `CLAUDE.md`, `PROMPT_CONTRACT.md`

### jcn-game-dev-ops

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/jcn-game-dev-ops`
- Purpose: deterministic game asset pipeline operations.
- Primary entrypoints: task scripts invoked through `justfile`.
- Run/test/lint:
  - `just tools-check`
  - `just sprite-validate`
  - `just sprite-optimize`
  - `just atlas-build`
  - `just blender-render`
  - `just engine-import-check`
  - `just godot-import-check`
  - `just ci-smoke`
- Docs: `README.md`, `spec/integration-contract.md`
- Governance files: none discovered

### jcn-pai (jcnbot-equivalent candidate)

- Path: `/Users/justin/Documents/Justyn Clark Network/REPOS/jcn-pai`
- Purpose: local-first personal AI infrastructure with Telegram ingress and policy routing.
- Primary entrypoints: service directories described in README (`services/relay-telegram`, `router`, `memory`, `tools`, `observatory`).
- Run/test/lint:
  - `npm run lint`
  - `npm run check`
  - `npm run test`
- Docs: `README.md`
- Governance files: `AGENTS.md`, `CLAUDE.md`
