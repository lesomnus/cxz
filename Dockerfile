# syntax=docker/dockerfile:1
# Compile both architectures natively; no emulation is needed for Go builds.
FROM --platform=$BUILDPLATFORM golang:1.27 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG APP_VERSION=dev
ARG BUILD_HASH=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    for arch in amd64 arm64; do \
      mkdir -p /dist/linux-${arch}; \
      CGO_ENABLED=0 GOOS=linux GOARCH=${arch} go build -trimpath \
        -ldflags="-s -w -X main.version=${APP_VERSION} -X main.buildRevision=${BUILD_HASH}" \
        -o /dist/linux-${arch}/cxz ./cmd/cxz || exit 1; \
    done

FROM scratch AS build
COPY --from=builder /dist/ /
