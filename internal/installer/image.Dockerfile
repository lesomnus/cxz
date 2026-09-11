FROM node:24-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git curl && rm -rf /var/lib/apt/lists/*
COPY --from=docker:29-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker:29-cli /usr/local/libexec/docker/cli-plugins /usr/local/libexec/docker/cli-plugins
RUN npm install -g @devcontainers/cli@0.89.0
ARG TARGETARCH
COPY linux-${TARGETARCH}/cxz /usr/local/bin/cxz
ENTRYPOINT ["/usr/local/bin/cxz"]
CMD ["--state", "/var/lib/cxz", "serve"]
