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
CI pins and checksum-verifies v1.1.2. It creates disposable workspaces; your
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

## Development-state command proof

The released SMALL v1.1.0 can truncate an allowed localhost URL inside a command
summary, then reject that display during strict validation. This checkout has
two such historical screenshot captures. Keep their original receipts unchanged
and use the released SMALL v1.1.2, which includes the command-summary fix and
the captured-source correction needed for the retained QA script.
Verify the resolved binary rather than assuming a Homebrew installation updated.

The correction validates exact original command bytes against the saved ref,
SHA256 and legacy summary. Individually reviewed, non-secret proof can be kept
under `.small-command-proofs/`, mirroring the supported `.small-cache/` ref.
The proof store is portable validation input, not replacement audit history.
Never export the whole private cache. Missing or changed proof must fail closed.

New truncated captures marked `command_summary_version: 2` also need portable
exact-byte proof before committing state for a cache-free checkout. Review each
full command for private material before copying it; do not strip the marker.
Short captures retain the existing metadata contract. See the upstream
[command-proof guide](https://github.com/justyn-clark/small-protocol/blob/v1.1.2/docs/command-proof.md).

v1.1.2 recognizes a narrowly defined backtick-quoted local URL with a supported
runtime-port expression only after exact command SHA256 and summary verification.
It does not execute source to validate links. Authored fields, external HTTP,
unsupported expressions, credentials and missing/tampered proof remain strict.
The existing QA capture can therefore pass without rewriting its original bytes.

Validate with the explicitly selected binary:

```sh
small version # must resolve to v1.1.2 for this checkout
small check --strict
```

Check a disposable snapshot with no cache: selected proof must pass, while
missing or tampered proof must fail without changing the saved state. CI adopts
the same v1.1.2 release; local success alone does not establish hosted compatibility
or release readiness. Verify checks for the committed candidate before publication.

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
