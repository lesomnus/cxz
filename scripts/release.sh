#!/usr/bin/env bash
set -euo pipefail
# Build Linux runtimes and Windows remote frontends.
version=${1:?usage: bash scripts/release.sh vX.Y.Z[-prerelease]|edge [output-directory]}
output=${2:-dist}
if [[ "$version" != edge && ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]]; then
  echo 'invalid version' >&2
  exit 1
fi
build_version=$version
if [[ "$version" == edge ]]; then
  build_version="source-$(git rev-parse --short=12 HEAD)"
fi
mkdir -p "$output"
for arch in amd64 arm64; do
  mkdir -p "$output/linux-$arch"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=$build_version" -o "$output/linux-$arch/cxz" ./cmd/cxz
  tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -C "$output/linux-$arch" -cf - cxz | gzip -n > "$output/cxz-$version-linux-$arch.tar.gz"
done
for arch in amd64 arm64; do
  mkdir -p "$output/windows-$arch"
  CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=$build_version" -o "$output/windows-$arch/cxz.exe" ./cmd/cxz
  python3 - "$output/windows-$arch/cxz.exe" "$output/cxz-$version-windows-$arch.zip" <<'PY'
import sys, zipfile
from pathlib import Path
with zipfile.ZipFile(sys.argv[2], 'w', compression=zipfile.ZIP_DEFLATED) as archive:
    info = zipfile.ZipInfo('cxz.exe', (1980, 1, 1, 0, 0, 0))
    info.compress_type = zipfile.ZIP_DEFLATED
    archive.writestr(info, Path(sys.argv[1]).read_bytes())
PY
done
(
  cd "$output"
  sha256sum "cxz-$version-linux-amd64.tar.gz" "cxz-$version-linux-arm64.tar.gz" "cxz-$version-windows-amd64.zip" "cxz-$version-windows-arm64.zip" > SHA256SUMS
  sha256sum -c SHA256SUMS
)
