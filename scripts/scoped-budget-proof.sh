#!/usr/bin/env bash
set -euo pipefail
[[ "$#" -eq 2 ]] || { echo 'usage: scoped-budget-proof.sh <loopexec> <fresh-output-directory>' >&2; exit 2; }
binary=$(cd "$(dirname "$1")" && pwd -P)/$(basename "$1")
out=$2
mkdir -p "$out"
[[ -z "$(find "$out" -mindepth 1 -print -quit)" ]] || { echo 'output directory must be empty' >&2; exit 2; }
out=$(cd "$out" && pwd -P)
store="$out/store"
mkdir "$store"
policy="$out/policy.json"
cat > "$policy" <<'JSON'
{"schema_version":1,"scope":"studio-floor-rack","max_calls":2,"phase_limits":{"builder":1,"critic":1}}
JSON
adapter=$(cd "$(dirname "${BASH_SOURCE[0]}")/../examples/scoped-budget" && pwd -P)/adapter.sh
"$adapter" "$binary" "$store" "$policy" run-a builder -- /bin/sh -c 'printf builder > "$1"' sh "$out/builder.marker"
"$adapter" "$binary" "$store" "$policy" run-b critic -- /bin/sh -c 'printf critic > "$1"' sh "$out/critic.marker"
test -f "$out/builder.marker"
test -f "$out/critic.marker"
set +e
"$adapter" "$binary" "$store" "$policy" run-c builder -- /bin/sh -c 'printf forbidden > "$1"' sh "$out/forbidden.marker" >"$out/blocked.stdout" 2>"$out/blocked.stderr"
code=$?
set -e
[[ "$code" -eq 18 ]] || { echo "expected budget exit 18, got $code" >&2; exit 1; }
test ! -e "$out/forbidden.marker"
"$binary" budget status --json --store-dir "$store" --policy "$policy" > "$out/status.json"
grep -q '"used":2' "$out/status.json"
grep -q '"remaining":0' "$out/status.json"
printf 'SCOPED_BUDGET_PROOF_OK: two independent run IDs used two calls; third child never started\n'
