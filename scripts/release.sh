#!/usr/bin/env bash
# Build release binaries and archives using Go and standard shell tools.
set -euo pipefail

if [[ $# -ne 2 || ! $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]]; then
  echo 'usage: release.sh vMAJOR.MINOR.PATCH OUTPUT_DIRECTORY' >&2
  exit 2
fi
version=$1
cd "$(dirname "$0")/.."
mkdir -p "$2"
destination=$(cd "$2" && pwd)
commit=$(git rev-parse HEAD)
fork=$(go list -m -f '{{if .Replace}}{{.Replace.Version}}{{end}}' github.com/glesys/glesys-go/v8)
if [[ -z $fork ]]; then
  echo 'release requires an exact remote SDK replacement version' >&2
  exit 2
fi
sdk=$(go list -m -f '{{.Version}}' github.com/glesys/glesys-go/v8)+fork.$fork
flags="-s -w -buildid= -X main.version=$version -X main.commit=$commit -X main.sdkVersion=$sdk"
build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT
cp README.md LICENSE THIRD-PARTY-NOTICES.txt "$build_dir/"
: > "$destination/checksums.txt"

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  system=${target%/*}
  architecture=${target#*/}
  binary=elg
  if [[ $system == windows ]]; then binary=elg.exe; fi
  GOOS=$system GOARCH=$architecture CGO_ENABLED=0 go build \
    -trimpath -buildvcs=false -ldflags "$flags" -o "$build_dir/$binary" ./cmd/elg
  stem=elg_${version}_${system}_${architecture}
  if [[ $system == windows ]]; then
    archive=$stem.zip
    (cd "$build_dir" && zip -q -X "$destination/$archive" "$binary" README.md LICENSE THIRD-PARTY-NOTICES.txt)
    unzip -tq "$destination/$archive" >/dev/null
  else
    archive=$stem.tar.gz
    tar -czf "$destination/$archive" -C "$build_dir" "$binary" README.md LICENSE THIRD-PARTY-NOTICES.txt
    tar -tzf "$destination/$archive" >/dev/null
  fi
  (cd "$destination" && shasum -a 256 "$archive") >> "$destination/checksums.txt"
  echo "$archive"
done
test "$(wc -l < "$destination/checksums.txt")" -eq 5
