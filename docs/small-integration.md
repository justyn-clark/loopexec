# SMALL integration

LoopExec v0.3.0 composes with SMALL CLI v1.1.0, supporting both v1 artifact
workspaces and explicitly migrated v2 session workspaces. SMALL is an optional
external CLI, not a runtime dependency.

SMALL records work state, attribution, evidence, and handoff. LoopExec owns the
bounded work loop, retries, subprocess lifetime, accounting, and candidate
acceptance. The shipped integration does not automatically select SMALL tasks,
spawn agents, migrate projects, or resolve collaborative conflicts.

## Executable compatibility proof

Build LoopExec, install the released SMALL CLI, and choose an empty output path:

```sh
go build -o build/loopexec ./cmd/loopexec
small version
./scripts/small-proof.sh ./build/loopexec "$(command -v small)" build/small-proof
```

The optional proof requires Bash, jq, a POSIX shell, and SMALL v1.1.0 or newer.
CI pins and checksum-verifies v1.1.0. It creates disposable workspaces; your
project's profile is unchanged. The proof covers:

- v1 state with explicit task checkpoint after verified LoopExec convergence;
- migration of a fixture to v2 and explicit collaborative mode;
- two distinct sessions and separately attributable, bounded repair runs;
- apply success leaving task acceptance unchanged;
- candidate-independent external checks, offline replay, saved portable
  convergence receipts, strict validation, and preserved handoff narratives.

The two session runs execute sequentially in this proof. This verifies CLI
composition and attribution; it is not a distributed-concurrency or live-model
benchmark. Use separate worktrees/candidate directories and runtime storage for
independent live work. A single workflow runtime has one governor.

## Exit codes and acceptance

An external check passes with exit 0. LoopExec returns exit 10 for verified
convergence, so a shell wrapper passed to SMALL must normalize exactly that
outcome. Other LoopExec exits must remain failures. For example, inside an
already prepared workspace:

```sh
small check --strict
small apply --task <task-id> --session <session-id> --cmd '
  set +e
  loopexec run --run-id repair --max-iterations 3 \
    --timeout 60s --command-timeout 10s \
    --exec "./repair-once.sh" --check "./verify.sh"
  code=$?
  test "$code" -eq 10
'
```

Substitute a task/session and real one-attempt work/check commands. Omit
`--session` for v1. Inspect the governor receipt and acceptance result before
running `small checkpoint`. A SMALL strict check validates state; it is not the
application's external acceptance oracle.

Capture the run result to a JSON file if you want a portable SMALL receipt:
`small evidence save --dir <workspace> --task <task-id> --file <absolute-result-path> --validator loopexec-convergence --json`.
SMALL then owns its evidence copy; private diagnostics and credentials must not
be exported. Keep LoopExec's JSONL/state receipts for detailed replay and audit.

## Sessions, policy, and recovery

Use SMALL's supported status/reconstruct/mode/session commands to inspect the
selected profile. Do not parse or modify the five legacy YAML files in a v2
workspace. Its immutable session events and generated views have different
storage and authority.

A session ID identifies the SMALL writer. A LoopExec run ID identifies one
governed execution. Keep both in integration receipts; neither replaces SMALL
lineage identity. Pass the explicit session ID when more than one writer exists.

For workflow resume, use the original LoopExec run ID, configuration, limits and
`--resume`. SMALL session continuation does not reset the governor's cumulative
budget, attempts, or total deadline. Resolve SMALL conflicts before a gated
handoff; never use a green LoopExec check to override a SMALL conflict.

No migration is implicit. Follow the released
[SMALL session-profile guide](https://github.com/justyn-clark/small-protocol/blob/v1.1.0/docs/session-profile-v2.md)
for preview/apply migration and solo/collaborative transitions.
