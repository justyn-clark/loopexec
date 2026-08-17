#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'DOGFOOD_FAIL: %s\n' "$1" >&2
  exit 1
}

if [[ "$#" -ne 2 ]]; then
  printf 'usage: %s <loopexec-binary> <empty-output-directory>\n' "$0" >&2
  exit 2
fi

BIN="$1"
OUT="$2"
[[ -x "$BIN" ]] || fail "binary is not executable: $BIN"
if [[ -e "$OUT" && ! -d "$OUT" ]]; then
  fail "output path is not a directory: $OUT"
fi
mkdir -p "$OUT"
if [[ -n "$(find "$OUT" -mindepth 1 -print -quit)" ]]; then
  fail "output directory must be empty: $OUT"
fi
OUT="$(cd "$OUT" && pwd -P)"

run_expect() {
  local want="$1" stdout_file="$2" stderr_file="$3"
  shift 3
  local got
  set +e
  "$@" >"$stdout_file" 2>"$stderr_file"
  got=$?
  set -e
  if [[ "$got" -ne "$want" ]]; then
    cat "$stdout_file" >&2
    cat "$stderr_file" >&2
    fail "expected exit $want, got $got: $*"
  fi
}

require_contains() {
  local needle="$1" file="$2"
  grep -Fq -- "$needle" "$file" || fail "$file does not contain: $needle"
}

require_nonempty() {
  [[ -s "$1" ]] || fail "missing or empty proof artifact: $1"
}

# First-run surface: the built-in command proves the same core path in one call.
run_expect 0 "$OUT/00-demo.json" "$OUT/00-demo.stderr" \
  "$BIN" demo --json --workdir "$OUT/demo"
require_contains '"status":"verified"' "$OUT/00-demo.json"
require_contains '"verified":true' "$OUT/00-demo.json"
require_nonempty "$OUT/demo/.loopexec/run-demo.jsonl"
require_nonempty "$OUT/demo/.loopexec/run-demo.state.json"

# 1) Convergence: work repairs a red fixture, the external check turns green,
# and replay verifies the recorded fingerprint.
CONVERGE="$OUT/convergence"
mkdir "$CONVERGE"
printf 'red\n' >"$CONVERGE/status.txt"
run_expect 10 "$OUT/01-convergence-run.txt" "$OUT/01-convergence-run.stderr" \
  "$BIN" run --run-id dogfood-convergence --workdir "$CONVERGE" --max-iterations 3 \
  --exec "printf 'green\\n' > status.txt" --check "grep -qx green status.txt"
run_expect 0 "$OUT/02-convergence-report.txt" "$OUT/02-convergence-report.stderr" \
  "$BIN" report --workdir "$CONVERGE" --run-id dogfood-convergence
run_expect 0 "$OUT/03-convergence-replay.txt" "$OUT/03-convergence-replay.stderr" \
  "$BIN" replay --workdir "$CONVERGE" --run-id dogfood-convergence
require_contains 'halt_reason: success_condition_met' "$OUT/01-convergence-run.txt"
require_contains 'verified: true' "$OUT/03-convergence-replay.txt"
require_nonempty "$CONVERGE/.loopexec/run-dogfood-convergence.jsonl"
require_nonempty "$CONVERGE/.loopexec/run-dogfood-convergence.state.json"

# 2) Non-convergence: a repeated failing set produces an oscillation halt and
# the operator guidance is explicitly do-not-retry.
OSCILLATION="$OUT/oscillation"
mkdir "$OSCILLATION"
run_expect 17 "$OUT/04-oscillation-run.txt" "$OUT/04-oscillation-run.stderr" \
  "$BIN" run --run-id dogfood-oscillation --workdir "$OSCILLATION" --max-iterations 5 \
  --exec true --check false --failures-cmd "printf 'failure-A\\n'"
run_expect 0 "$OUT/05-oscillation-report.txt" "$OUT/05-oscillation-report.stderr" \
  "$BIN" report --workdir "$OSCILLATION" --run-id dogfood-oscillation
run_expect 0 "$OUT/06-oscillation-explain.txt" "$OUT/06-oscillation-explain.stderr" \
  "$BIN" explain-halt --workdir "$OSCILLATION" --run-id dogfood-oscillation
require_contains 'halt_reason: oscillation_detected' "$OUT/04-oscillation-run.txt"
require_contains 'verdict: do-not-retry' "$OUT/06-oscillation-explain.txt"
require_nonempty "$OSCILLATION/.loopexec/run-dogfood-oscillation.jsonl"
require_nonempty "$OSCILLATION/.loopexec/run-dogfood-oscillation.state.json"

# 3) Metric integrity: the check would be green, but the test-determining
# surface shrinks, so the guard dominates success and rejects the outcome.
INTEGRITY="$OUT/integrity"
mkdir "$INTEGRITY"
printf 'test-A\ntest-B\n' >"$INTEGRITY/test-surface.txt"
run_expect 13 "$OUT/07-integrity-run.txt" "$OUT/07-integrity-run.stderr" \
  "$BIN" run --run-id dogfood-integrity --workdir "$INTEGRITY" --max-iterations 3 \
  --exec "printf 'test-A\\n' > test-surface.txt" --integrity-cmd "cat test-surface.txt" --check true
run_expect 0 "$OUT/08-integrity-report.txt" "$OUT/08-integrity-report.stderr" \
  "$BIN" report --workdir "$INTEGRITY" --run-id dogfood-integrity
require_contains 'halt_reason: metric_integrity_violation' "$OUT/07-integrity-run.txt"
require_nonempty "$INTEGRITY/.loopexec/run-dogfood-integrity.jsonl"
require_nonempty "$INTEGRITY/.loopexec/run-dogfood-integrity.state.json"

printf 'DOGFOOD_OK\n' | tee "$OUT/00-summary.txt"
