#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FEELTEST_FAIL: $1" >&2
  exit 1
}

if ! command -v jq >/dev/null 2>&1; then
  fail "jq is required but not found in PATH"
fi

if ! command -v loopexec >/dev/null 2>&1; then
  fail "loopexec binary not found in PATH"
fi

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

cd "$tmpdir"
rm -rf .loopexec .small 2>/dev/null || true

json_check_common() {
  local payload="$1"
  echo "$payload" | jq -e 'type == "object"' >/dev/null || fail "stdout is not a single JSON object"
  echo "$payload" | jq -e '.tool == "loopexec"' >/dev/null || fail "tool must be loopexec"
  echo "$payload" | jq -e '.status != null' >/dev/null || fail "status must be present"
  echo "$payload" | jq -e '.errors | type == "array"' >/dev/null || fail "errors must be an array"
}

run_capture() {
  local cmd_out cmd_err code
  set +e
  cmd_out="$($@ 2> >(cat >&2))"
  code=$?
  set -e
  printf '%s\n' "$cmd_out"
  return $code
}

# A) init --json
set +e
out="$(loopexec init --json)"
code=$?
set -e
[ "$code" -eq 0 ] || fail "init --json exit code $code, expected 0"
json_check_common "$out"

# B) run --json
set +e
out="$(loopexec run --json)"
code=$?
set -e
if [ "$code" -ne 0 ] && [ "$code" -ne 10 ]; then
  fail "run --json exit code $code, expected 0 or 10"
fi
json_check_common "$out"

# C) run --json --halt-reason blocked
set +e
out="$(loopexec run --json --halt-reason blocked)"
code=$?
set -e
[ "$code" -eq 11 ] || fail "run blocked exit code $code, expected 11"
json_check_common "$out"
echo "$out" | jq -e '.halt_reason == "blocked"' >/dev/null || fail "halt_reason must be blocked"

# D) status --json
set +e
out="$(loopexec status --json)"
code=$?
set -e
[ "$code" -eq 0 ] || fail "status --json exit code $code, expected 0"
json_check_common "$out"

# E) check --json
set +e
out="$(loopexec check --json)"
code=$?
set -e
[ "$code" -eq 0 ] || fail "check --json exit code $code, expected 0"
json_check_common "$out"

# F) agent loop simulation
while true; do
  set +e
  out="$(loopexec run --json)"
  code=$?
  set -e

  printf "%s\n" "$out" > .last_loopexec.json
  echo "$out" | jq -e 'type == "object"' >/dev/null || fail "agent loop output is not valid JSON object"

  if [ "$code" -eq 0 ] || [ "$code" -eq 10 ]; then
    echo "HALT_SUCCESS"
    break
  fi
  if [ "$code" -eq 11 ]; then
    echo "HALT_BLOCKED"
    break
  fi
  if [ "$code" -eq 12 ]; then
    echo "HALT_MAX_ITER"
    break
  fi
  if [ "$code" -eq 20 ]; then
    echo "HALT_INVARIANT"
    break
  fi
  if [ "$code" -eq 30 ]; then
    echo "HALT_WORKSPACE"
    break
  fi
  if [ "$code" -eq 40 ]; then
    echo "HALT_EXECUTION"
    break
  fi

  fail "agent loop unknown exit code $code"
done

[ -d .loopexec ] || fail ".loopexec directory was not created"

echo "FEELTEST_OK"