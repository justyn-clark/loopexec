#!/usr/bin/env bash
set -euo pipefail
# Changed paths arrive on stdin, like check-small-sync.sh.
behavior=0
prose=0
changelog=0
while IFS= read -r path; do
  case "$path" in
    cmd/*|internal/*|examples/*|tools/*|scripts/*|.github/workflows/*|go.mod|go.sum|SPEC.md|AGENTS.md)
      behavior=1 ;;
  esac
  case "$path" in
    README.md|SPEC.md|docs/cli.md|docs/architecture.md|docs/workflows.md|docs/integrations.md|docs/loop-contract.md|docs/small-integration.md|docs/acceptance-policy.md|docs/documentation-continuity.md|docs/creative-verification.md)
      prose=1 ;;
  esac
  [[ "$path" != CHANGELOG.md ]] || changelog=1
done
if [[ "$behavior" -eq 1 && ( "$prose" -ne 1 || "$changelog" -ne 1 ) ]]; then
  echo "Code/docs continuity failed: behavioral, tooling or policy changes require canonical prose AND CHANGELOG.md in the same change." >&2
  exit 1
fi
echo DOCS_SYNC_OK
