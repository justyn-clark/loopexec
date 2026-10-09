#!/usr/bin/env bash
set -euo pipefail
[[ "$#" -eq 2 ]] || { echo "usage: build-release.sh <vX.Y.Z> <empty-output-directory>" >&2; exit 2; }
TAG="$1"
OUT="$2"
[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "a stable vX.Y.Z tag is required" >&2; exit 2; }
command -v jq >/dev/null
version="$(go run ./cmd/loopexec version --json | jq -er '.version')"
[[ "v$version" == "$TAG" ]] || { echo "source version differs from release tag" >&2; exit 1; }
go test ./cmd/loopexec -run '^TestDocumentationContract$' -count=1
mkdir -p "$OUT"
[[ -z "$(find "$OUT" -mindepth 1 -print -quit)" ]] || { echo "output must be empty" >&2; exit 1; }
OUT="$(cd "$OUT" && pwd -P)"
go run ./tools/docs-continuity bundle "$OUT/documentation"
tar -czf "$OUT/loopexec-$TAG-documentation.tar.gz" -C "$OUT" documentation
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}"
  arch="${target#*/}"
  name="loopexec-$TAG-$os-$arch"
  stage="$OUT/.stage/$name"
  mkdir -p "$stage"
  executable=loopexec
  [[ "$os" != windows ]] || executable=loopexec.exe
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -o "$stage/$executable" ./cmd/loopexec
  cp -R "$OUT/documentation/." "$stage/"
  cp "docs/releases/$TAG.md" "$stage/RELEASE_NOTES.md"
  cp -R "$OUT/documentation" "$stage/documentation"
  if [[ "$os" == windows ]]; then
    (cd "$stage" && zip -qr "$OUT/$name.zip" "$executable" LICENSE README.md SPEC.md CHANGELOG.md RELEASE_NOTES.md docs examples documentation)
  else
    tar -czf "$OUT/$name.tar.gz" -C "$stage" "$executable" LICENSE README.md SPEC.md CHANGELOG.md RELEASE_NOTES.md docs examples documentation
  fi
done
go run ./tools/docs-continuity verify-bundle "$OUT/documentation"
(cd "$OUT" && shasum -a 256 loopexec-*.tar.gz loopexec-*.zip > checksums.txt && shasum -a 256 -c checksums.txt)
printf 'RELEASE_PACKAGES_OK: %s\n' "$OUT"
