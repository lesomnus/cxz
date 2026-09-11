#!/usr/bin/env bash
set -euo pipefail
# Build Linux artifacts into a dedicated output directory.
version=${1:?usage: bash scripts/release.sh vX.Y.Z[-prerelease] [output-directory]}
output=${2:-dist}
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]]; then
  echo 'invalid version' >&2
  exit 1
fi
mkdir -p "$output"
for arch in amd64 arm64; do
  mkdir -p "$output/linux-$arch"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$output/linux-$arch/cxz" ./cmd/cxz
  tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -C "$output/linux-$arch" -cf - cxz | gzip -n > "$output/cxz-$version-linux-$arch.tar.gz"
done
(
  cd "$output"
  sha256sum "cxz-$version-linux-amd64.tar.gz" "cxz-$version-linux-arm64.tar.gz" > SHA256SUMS
  sha256sum -c SHA256SUMS
)
