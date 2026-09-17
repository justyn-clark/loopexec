# Integrations

Use `loopexec run` as the Go loop governor. Supply an external check and an
optional work command. A check exit of 0 means the oracle passed; loopexec exits
10 when all guards also pass. Halt reasons are computed from observed state.

```bash
mkdir -p build/integration-demo
printf 'red\n' > build/integration-demo/status.txt
set +e
./build/loopexec run --json --workdir build/integration-demo \
  --run-id integration --max-iterations 3 --timeout 10s --command-timeout 2s \
  --exec "printf 'green\\n' > status.txt" --check "grep -qx green status.txt"
code=$?
set -e
test "$code" -eq 10
./build/loopexec replay --json --workdir build/integration-demo --run-id integration
./build/loopexec report --workdir build/integration-demo --run-id integration
```

Machine output is one JSON object with `tool, version, status, errors[]` and,
where applicable, run ID, iteration, halt reason, check exit, and receipt path.
Workflow output adds failure cause and the best verified candidate path.
Use the halt reason for precise decisions and the exit class for CI branching.
The complete map is in [SPEC.md](../SPEC.md#5-halt-reasons---exit-codes-canonical-map).

`replay` re-runs only the read-only saved check and compares its verdict
fingerprint. It never runs an agent. `report` reads state/receipts without
executing anything. `reexecute --confirm` is a live rerun of legacy configs;
workflow receipts require explicit initialization of a new adapter workspace.

For unattended creative work, use the [workflow contract](workflows.md).
Its Go adapter executes one builder/test/conditional-critic sequence.
The governor owns retries, subprocess bounds, exact accounting, candidate
promotion/restoration, and numeric patience. The executable fixture needs no
Python runner, network, model key, Blender, or Godot.

A nonzero `--budget-usd` now requires workflow metering. Strict caps require
enforced per-call reservations; observed metering only stops subsequent work.
Local/subscription usage may report unknown dollars while time, iteration,
call, and token bounds remain available. Model/sampling flags record metadata;
the adapter's actual argv/configuration chooses and limits the invoked model.

Legacy `--failures-cmd` and `--integrity-cmd` collect newline identities or
`{"ids":[]}`. Collector failure or stderr cannot become valid evidence.
The integrity baseline must retain all members. Numeric mode accepts a separate
candidate-bound score and does not classify stable visual issue IDs as oscillation.

CI runs formatting, vet, tests, the existing deterministic dogfood scenarios,
and the creative proof script. Hidden receipt files are included in proof
artifacts; private signing keys and diagnostic tails are excluded.
See [CLI reference](cli.md) for the full command surface.

For SMALL CLI v1.1.0 and collaborative v2 sessions, see the
[SMALL integration guide](small-integration.md). The optional real-CLI proof
keeps task acceptance separate from a command exit and preserves session
attribution without introducing a second loop controller.
