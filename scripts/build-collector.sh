#!/usr/bin/env bash
set -euo pipefail
output=${1:?usage: bash scripts/build-collector.sh OUTPUT_DIRECTORY}
mkdir -p "$output"
output=$(cd "$output" && pwd)
go run ./cmd/cxz-collector-source "$output"
(
  cd "$output/components"
  go test -mod=readonly ./...
)
go install go.opentelemetry.io/collector/cmd/builder@v0.164.0
(
  cd "$output"
  "$(go env GOPATH)/bin/builder" --config=builder.yaml --skip-compilation --skip-get-modules
  cp collector.lock.mod .build/go.mod
  cp collector.lock.sum .build/go.sum
  cd .build
  CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags='-s -w' -o ../cxz-otelcol .
)
