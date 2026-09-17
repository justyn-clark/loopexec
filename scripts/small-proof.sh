#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'SMALL_PROOF_FAIL: %s\n' "$1" >&2; exit 1; }
[[ "$#" -eq 3 ]] || fail "usage: small-proof.sh <loopexec> <small-1.1.0+> <empty-output-directory>"
command -v jq >/dev/null || fail "jq is required for this optional integration proof"
[[ -x "$1" && -x "$2" ]] || fail "both CLI paths must be executable"
BIN="$(cd "$(dirname "$1")" && pwd -P)/$(basename "$1")"
SMALL="$(cd "$(dirname "$2")" && pwd -P)/$(basename "$2")"
OUT="$3"
mkdir -p "$OUT"
[[ -z "$(find "$OUT" -mindepth 1 -print -quit)" ]] || fail "output must be empty"
OUT="$(cd "$OUT" && pwd -P)"
"$SMALL" version > "$OUT/small-version.txt"
quote() { jq -rn --arg value "$1" '$value | @sh'; }

# Normalize the governor's documented convergence exit (10) only. Every other
# result stays a failure for SMALL. This script executes one governor, no retries.
cat > "$OUT/run-once.sh" <<'SH'
#!/bin/sh
set -eu
bin="$1"
work="$2"
mkdir -p "$work"
printf 'red\n' > "$work/status.txt"
set +e
"$bin" run --json --workdir "$work" --run-id repair --max-iterations 2 \
  --timeout 15s --command-timeout 3s --terminate-grace 100ms \
  --exec "printf 'green\\n' > status.txt" --check "grep -qx green status.txt" > "$work/run.json"
code=$?
set -e
test "$code" -eq 10
"$bin" replay --json --workdir "$work" --run-id repair > "$work/replay.json"
SH

# Existing v1 workspaces remain supported with the released CLI.
V1="$OUT/v1"
"$SMALL" init --dir "$V1" --no-agents --intent "Offline bounded repair proof" > "$OUT/v1-init.txt"
"$SMALL" progress add --dir "$V1" --task task-1 --status in_progress --evidence "Run bounded repair" > "$OUT/v1-start.txt"
cmd="sh $(quote "$OUT/run-once.sh") $(quote "$BIN") $(quote "$V1/work")"
"$SMALL" apply --dir "$V1" --task task-1 --cmd "$cmd" > "$OUT/v1-apply.txt"
"$SMALL" status --dir "$V1" --json > "$OUT/v1-before-checkpoint.json"
jq -e '.plan.tasks_by_status.completed == 0' "$OUT/v1-before-checkpoint.json" >/dev/null
jq -e '.halt_reason == "success_condition_met" and .iteration == 1' "$V1/work/run.json" >/dev/null
jq -e '.verified == true' "$V1/work/replay.json" >/dev/null
"$SMALL" checkpoint --dir "$V1" --task task-1 --status completed --evidence "LoopExec converged once; external check and replay passed" > "$OUT/v1-checkpoint.txt"
"$SMALL" check --dir "$V1" --strict > "$OUT/v1-strict.txt"
"$SMALL" handoff --dir "$V1" --summary "Bounded repair and replay passed in v1." --json > "$OUT/v1-handoff.json"
grep -q 'Bounded repair and replay passed in v1.' "$OUT/v1-handoff.json" || fail "v1 narrative lost"

# Migrate only this disposable fixture; real operator projects are never migrated.
V2="$OUT/v2"
"$SMALL" init --dir "$V2" --no-agents --intent "Two attributed LoopExec runs" > "$OUT/v2-init.txt"
"$SMALL" check --dir "$V2" --strict > "$OUT/v2-bootstrap-strict.txt"
"$SMALL" checkpoint --dir "$V2" --task task-1 --status completed --evidence "Proof workspace initialized and strict validation passed" > "$OUT/v2-bootstrap.txt"
"$SMALL" migrate --dir "$V2" --to 2.0.0 --preview --namespace loopexec-small-proof --json > "$OUT/migration.json"
digest="$(jq -er '.expected_input_digest' "$OUT/migration.json")"
"$SMALL" migrate --dir "$V2" --apply "$OUT/migration.json" --expect-state "$digest" --json > "$OUT/migration-applied.json"
"$SMALL" session start --dir "$V2" --label release-proof-controller --json > "$OUT/controller.json"
controller="$(jq -er '.session.session_id' "$OUT/controller.json")"
"$SMALL" mode show --dir "$V2" --json > "$OUT/mode-before.json"
frontier="$(jq -er '.frontier' "$OUT/mode-before.json")"
"$SMALL" mode set collaborative --dir "$V2" --session "$controller" --expect-state "$frontier" --reason "Two independently attributed bounded runs" --json > "$OUT/mode-change.json"
"$SMALL" session close --dir "$V2" --session "$controller" --summary "Collaborative fixture prepared." --json > "$OUT/controller-close.json"
"$SMALL" session start --dir "$V2" --label builder-a --json > "$OUT/a-session.json"
"$SMALL" session start --dir "$V2" --label builder-b --json > "$OUT/b-session.json"
a="$(jq -er '.session.session_id' "$OUT/a-session.json")"
b="$(jq -er '.session.session_id' "$OUT/b-session.json")"
[[ "$a" != "$b" ]] || fail "sessions must be distinct"

# Independent session runs are enumerated, not retried or scheduled by SMALL.
for label in a b; do
  session="$a"
  [[ "$label" != b ]] || session="$b"
  "$SMALL" plan --dir "$V2" --session "$session" --add "Bounded repair $label" > "$OUT/$label-task.txt"
  task="$(sed -nE 's/^Added task [^ ]+ \(([^)]+)\).*/\1/p' "$OUT/$label-task.txt")"
  [[ -n "$task" ]] || fail "missing task identity"
  "$SMALL" progress add --dir "$V2" --session "$session" --task "$task" --status in_progress --evidence "Run independent bounded repair" > "$OUT/$label-start.txt"
  work="$V2/work-$label"
  cmd="sh $(quote "$OUT/run-once.sh") $(quote "$BIN") $(quote "$work")"
  "$SMALL" apply --dir "$V2" --session "$session" --task "$task" --cmd "$cmd" > "$OUT/$label-apply.txt"
  "$SMALL" reconstruct --dir "$V2" --task "$task" --json > "$OUT/$label-before-checkpoint.json"
  jq -e '.task.status == "in_progress"' "$OUT/$label-before-checkpoint.json" >/dev/null
  jq -e '.halt_reason == "success_condition_met" and .iteration == 1' "$work/run.json" >/dev/null
  jq -e '.verified == true' "$work/replay.json" >/dev/null
  "$SMALL" evidence save --dir "$V2" --task "$task" --file "$work/run.json" --validator loopexec-convergence --json > "$OUT/$label-evidence.json"
  "$SMALL" checkpoint --dir "$V2" --session "$session" --task "$task" --status completed --evidence "LoopExec converged once; external check and replay passed; portable receipt saved" --json > "$OUT/$label-checkpoint.json"
  "$SMALL" check --dir "$V2" --strict > "$OUT/$label-strict.txt"
  "$SMALL" handoff --dir "$V2" --session "$session" --summary "Session $label bounded repair verified." --json > "$OUT/$label-handoff.json"
  grep -q "Session $label bounded repair verified." "$OUT/$label-handoff.json" || fail "session narrative lost"
done
"$SMALL" evidence verify --dir "$V2" --json > "$OUT/evidence-verified.json"
"$SMALL" session close --dir "$V2" --session "$a" --summary "Session a complete." --json > "$OUT/a-close.json"
"$SMALL" session close --dir "$V2" --session "$b" --summary "Session b complete." --json > "$OUT/b-close.json"
"$SMALL" check --dir "$V2" --strict > "$OUT/v2-strict.txt"
"$SMALL" reconstruct --dir "$V2" --resume --session "$b" --limit 50 --max-bytes 65536 --json > "$OUT/v2-resume.json"
[[ ! -e "$V2/.small/progress.small.yml" ]] || fail "v2 regressed to shared progress"
[[ -s "$V1/work/.loopexec/run-repair.jsonl" ]] || fail "v1 governor receipt missing"
[[ -s "$V2/work-a/.loopexec/run-repair.jsonl" && -s "$V2/work-b/.loopexec/run-repair.jsonl" ]] || fail "session governor receipts missing"
printf 'SMALL_PROOF_OK\n' | tee "$OUT/summary.txt"
