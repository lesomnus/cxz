FROM --platform=$BUILDPLATFORM golang:1.27 AS collector
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY . .
RUN --mount=type=cache,id=cxz-otel-mod,target=/go/pkg/mod \
    --mount=type=cache,id=cxz-otel-build,target=/root/.cache/go-build \
    go install go.opentelemetry.io/collector/cmd/builder@v0.164.0 \
    && builder --config=builder.yaml --skip-compilation --skip-get-modules \
    && cp collector.lock.mod .build/go.mod && cp collector.lock.sum .build/go.sum \
    && cd .build && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
       go build -mod=readonly -trimpath -ldflags='-s -w' -o cxz-otelcol . \
    && cd .. \
    && mkdir -p /out && cp .build/cxz-otelcol /out/cxz-otelcol
FROM scratch
COPY --from=collector /out/ /
