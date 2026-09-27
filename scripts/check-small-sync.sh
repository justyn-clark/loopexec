#!/usr/bin/env bash
set -euo pipefail

changed=0
progress=0
while IFS= read -r file; do
  case "$file" in
    .small/progress.small.yml) progress=1 ;;
    .small/*) ;;
    '') ;;
    *) changed=1 ;;
  esac
done

if (( changed && !progress )); then
  echo 'Development changes require an accompanying .small/progress.small.yml update from the SMALL CLI.' >&2
  exit 1
fi
