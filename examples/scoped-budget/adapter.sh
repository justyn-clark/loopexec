#!/usr/bin/env bash
set -euo pipefail
if [[ "$#" -lt 7 ]]; then
  echo 'usage: adapter.sh <loopexec> <store-dir> <policy.json> <run-id> <phase> -- <child-command> [args...]' >&2
  exit 2
fi
binary=$1
store=$2
policy=$3
run_id=$4
phase=$5
separator=$6
shift 6
[[ "$separator" == '--' && "$#" -gt 0 ]] || { echo 'child command must follow --' >&2; exit 2; }
[[ "$run_id" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$ ]] || { echo 'invalid run ID' >&2; exit 2; }
"$binary" budget reserve --json --store-dir "$store" --policy "$policy" --phase "$phase" --call-id "$run_id/$phase" >/dev/null
exec "$@"
