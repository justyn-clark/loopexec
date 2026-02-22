# loopexec CLI

This document defines the command surface and machine contract for loopexec.

## Commands

- `loopexec init`
  - Initialize local loopexec workspace metadata.
- `loopexec run`
  - Run one bounded loop iteration.
- `loopexec status`
  - Show loop status.
- `loopexec check`
  - Validate invariants.
- `loopexec step`
  - Execute a single step.

## Global flags

- `--json`
  - Emit exactly one JSON object to stdout.
  - Human logs and error text go to stderr.

## JSON output schema

All commands in `--json` mode emit one object with these fields:

- `tool` (string)
- `version` (string)
- `status` (string)
- `run_id` (string, optional)
- `iteration` (integer, optional)
- `halt_reason` (string, optional)
- `errors` (array of strings)

Example:

```json
{
  "tool": "loopexec",
  "version": "0.1.0-rc1",
  "status": "ok",
  "run_id": "local",
  "iteration": 1,
  "errors": []
}
```

## Exit codes

- `0` success
- `10` halted success condition met
- `11` halted blocked
- `12` halted max iterations reached
- `20` invariant failed
- `30` workspace invalid or missing
- `40` execution failure (timeout or command failure)
- `50` internal error

## Full example

Human mode:

```sh
loopexec init
loopexec run
loopexec status --run-id local --iteration 1
```

JSON mode:

```sh
loopexec run --json
```
