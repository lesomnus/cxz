# UI for a source-based local install: use exactly the executable's revision.
FROM node:24-bookworm-slim AS cxz-web
ARG CXZ_WEB_REVISION
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
RUN git init --quiet . && git remote add origin https://github.com/lesomnus/cxz.git \
    && git fetch --no-tags --depth=1 origin "$CXZ_WEB_REVISION" \
    && git checkout --detach FETCH_HEAD
RUN --mount=type=cache,target=/root/.npm npm ci --prefix ts
RUN npm run --prefix ts build
