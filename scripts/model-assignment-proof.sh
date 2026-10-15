#!/usr/bin/env bash
set -euo pipefail
fail() { printf 'MODEL_ASSIGNMENT_PROOF_FAIL: %s\n' "$1" >&2; exit 1; }
[[ "$#" -eq 3 ]] || fail "usage: model-assignment-proof.sh <loopexec> <fixture-adapter> <new-output-directory>"
BIN="$(cd "$(dirname "$1")" && pwd -P)/$(basename "$1")"
ADAPTER="$(cd "$(dirname "$2")" && pwd -P)/$(basename "$2")"
OUT="$3"
[[ ! -e "$OUT" ]] || fail "preserve existing proof directory"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd -P)"
expect() {
  local want="$1" result="$2" code
  shift 2
  set +e
  "$@" > "$result.json" 2> "$result.stderr"
  code=$?
  set -e
  [[ "$code" -eq "$want" ]] || fail "unexpected exit $code, expected $want; inspect the relative proof case"
}
for scenario in normal budget reserve-drift usage-drift technical; do
  ROOT="$OUT/$scenario"
  "$ADAPTER" init --dir "$ROOT" --scenario "$scenario"
  expect 0 "$OUT/$scenario-plan" "$BIN" models plan --json --workflow workflow.json --workdir "$ROOT"
  [[ ! -e "$ROOT/.loopexec" && ! -e "$ROOT/simulated-execution.json" ]] || fail "planning performed work"
  want=10
  case "$scenario" in
    budget) want=18 ;;
    reserve-drift|usage-drift) want=19 ;;
    technical) want=12 ;;
  esac
  expect "$want" "$OUT/$scenario-run" "$BIN" run --json --workdir "$ROOT" --workflow workflow.json --run-id roles --max-iterations 1 --timeout 15s --command-timeout 2s
  expect 0 "$OUT/$scenario-report" "$BIN" report --json --workdir "$ROOT" --run-id roles
done
[[ ! -e "$OUT/budget/simulated-execution.json" ]] || fail "budget denial launched work"
[[ ! -e "$OUT/reserve-drift/simulated-execution.json" ]] || fail "identity mismatch launched work"
expect 10 "$OUT/normal-resume" "$BIN" run --json --workdir "$OUT/normal" --workflow workflow.json --run-id roles --max-iterations 1 --timeout 15s --command-timeout 2s --resume
expect 0 "$OUT/normal-replay" "$BIN" replay --json --workdir "$OUT/normal" --run-id roles
expect 0 "$OUT/normal-attest" "$BIN" attest --json --workdir "$OUT/normal" --run-id roles
expect 0 "$OUT/normal-verify" "$BIN" attest --verify --json --workdir "$OUT/normal" --run-id roles
printf 'MODEL_ASSIGNMENT_PROOF_OK: two configured roles, no-spend planning, pre/post-call drift guards, unchanged budget/technical gates, resume/replay/attestation; all model work synthetic\n'
