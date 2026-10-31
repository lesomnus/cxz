FROM --platform=$BUILDPLATFORM golang:1.27 AS cxz-collector
ARG TARGETARCH
WORKDIR /src
COPY collector-src/ ./
RUN --mount=type=cache,id=cxz-otel-mod,target=/go/pkg/mod \
    --mount=type=cache,id=cxz-otel-build,target=/root/.cache/go-build \
    go install go.opentelemetry.io/collector/cmd/builder@v0.164.0 \
    && builder --config=builder.yaml --skip-compilation --skip-get-modules \
    && cp collector.lock.mod .build/go.mod && cp collector.lock.sum .build/go.sum \
    && cd .build && CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
       go build -mod=readonly -trimpath -ldflags='-s -w' -o /out/cxz-otelcol .

FROM node:24-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git curl util-linux && rm -rf /var/lib/apt/lists/*
COPY --from=docker:29-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker:29-cli /usr/local/libexec/docker/cli-plugins /usr/local/libexec/docker/cli-plugins
RUN npm install -g @devcontainers/cli@0.89.0
ARG TARGETARCH
COPY linux-${TARGETARCH}/cxz /usr/local/bin/cxz
COPY --from=cxz-collector /out/cxz-otelcol /usr/local/bin/cxz-otelcol
COPY webui/ /usr/local/share/cxz/webui/
ENTRYPOINT ["/usr/local/bin/cxz"]
CMD ["--state", "/var/lib/cxz", "manager", "serve"]
