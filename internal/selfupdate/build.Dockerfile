# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG CXZ_REF
ARG CXZ_FETCH
WORKDIR /src
# The changing fetch ID resolves a moving branch again on every invocation.
# A separate checkout never changes the user's working tree.
RUN test -n "$CXZ_FETCH" && git init --quiet --initial-branch=main . \
    && git remote add origin https://github.com/lesomnus/cxz.git \
    && git fetch --no-tags --depth=1 origin "$CXZ_REF" \
    && git checkout --detach FETCH_HEAD
ARG CXZ_GOOS
ARG CXZ_GOARCH
ARG CXZ_BINARY
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
RUN --mount=type=cache,id=cxz-update-mod,target=/go/pkg/mod \
    --mount=type=cache,id=cxz-update-build,target=/root/.cache/go-build \
    mkdir -p /out \
    && revision="$(git rev-parse HEAD)" \
    && short="$(git rev-parse --short=12 HEAD)" \
    && GOOS="$CXZ_GOOS" GOARCH="$CXZ_GOARCH" go build -trimpath \
       -ldflags="-s -w -X main.version=source-${short} -X main.buildRevision=${revision}" \
       -o "/out/$CXZ_BINARY" ./cmd/cxz \
    && printf '%s\n' "$revision" > /out/revision

FROM scratch
COPY --from=build /out/ /
