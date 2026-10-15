#!/usr/bin/env bash
set -euo pipefail
[[ "$#" -eq 2 ]] || { echo "usage: publish-release.sh <release-tag> <packaged-directory>" >&2; exit 2; }
TAG="$1"
OUT="$2"
policy="$(go run ./tools/release-policy "$TAG")"
prerelease="$(printf '%s' "$policy" | jq -er '.prerelease | tostring')"
latest="$(printf '%s' "$policy" | jq -er '.latest | tostring')"
[[ -f "$OUT/checksums.txt" && -f "docs/releases/$TAG.md" ]] || { echo "release packages or notes missing" >&2; exit 2; }
go run ./tools/docs-continuity verify-bundle "$OUT/documentation"
go run ./tools/docs-continuity verify-site "$OUT/documentation/manifest.json"
gh release create "$TAG" "$OUT"/loopexec-*.tar.gz "$OUT"/loopexec-*.zip "$OUT/checksums.txt" \
  --verify-tag --draft "--prerelease=$prerelease" "--latest=$latest" \
  --title "LoopExec $TAG" --notes-file "docs/releases/$TAG.md"
go run ./tools/docs-continuity verify-site "$OUT/documentation/manifest.json"
gh release edit "$TAG" --draft=false "--prerelease=$prerelease" "--latest=$latest"
