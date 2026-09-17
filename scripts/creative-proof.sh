#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'CREATIVE_PROOF_FAIL: %s\n' "$1" >&2; exit 1; }
[[ "$#" -eq 3 ]] || fail "usage: creative-proof.sh <loopexec> <adapter> <empty-output-directory>"
[[ -x "$1" && -x "$2" ]] || fail "build both Go binaries first"
BIN="$(cd "$(dirname "$1")" && pwd -P)/$(basename "$1")"
ADAPTER="$(cd "$(dirname "$2")" && pwd -P)/$(basename "$2")"
OUT="$3"
mkdir -p "$OUT"
[[ -z "$(find "$OUT" -mindepth 1 -print -quit)" ]] || fail "output must be empty"
OUT="$(cd "$OUT" && pwd -P)"

expect() {
  local want="$1" result="$2" code
  shift 2
  set +e
  "$@" > "$result.json" 2> "$result.stderr"
  code=$?
  set -e
  [[ "$code" -eq "$want" ]] || fail "expected exit $want, got $code; inspect $result.json"
}

# This enumerates independent proof cases. Attempts/retries belong only to run.
for scenario in convergence budget regression stale technical cap; do
  budget=3
  want=10
  case "$scenario" in
    budget) budget=0.9; want=18 ;;
    stale) want=13 ;;
    cap) want=12 ;;
  esac
  ROOT="$OUT/$scenario"
  "$ADAPTER" init --dir "$ROOT" --scenario "$scenario" > "$OUT/$scenario-init.txt"
  expect "$want" "$OUT/$scenario-run" "$BIN" run --json --workdir "$ROOT" \
    --workflow workflow.json --run-id creative --max-iterations 4 --budget-usd "$budget" \
    --timeout 30s --command-timeout 3s --terminate-grace 100ms
  [[ -s "$ROOT/.loopexec/run-creative.jsonl" ]] || fail "missing receipt"
  [[ -s "$ROOT/.loopexec/run-creative.state.json" ]] || fail "missing state"
  expect 0 "$OUT/$scenario-report" "$BIN" report --json --workdir "$ROOT" --run-id creative
  if [[ "$scenario" != stale ]]; then
    expect 0 "$OUT/$scenario-replay" "$BIN" replay --json --workdir "$ROOT" --run-id creative
  fi
done

grep -q '"iteration":4' "$OUT/cap-run.json" || fail "attempt cap changed"
grep -q '"event":"candidate_restored"' "$OUT/regression/.loopexec/run-creative.jsonl" || fail "no restoration receipt"
[[ ! -e "$OUT/budget/evidence/attempt-000002" ]] || fail "work launched after reservation denial"
if grep -q '"phase": "critic"' "$OUT/technical/evidence/attempt-000001/timing.json"; then
  fail "critic ran after failed technical tests"
fi
printf 'CREATIVE_PROOF_OK: %s\n' "$OUT"
