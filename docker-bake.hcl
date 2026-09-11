variable "REPO" {
  default = "ghcr.io/lesomnus/cxz"
}
variable "TAG" {
  default = "local"
}
variable "BUILD_HASH" {
  default = "local"
}
variable "APP_VERSION" {
  default = TAG
}

group "default" {
  targets = ["build"]
}

# Like cld, export binaries first; app consumes the resulting dist directory.
target "build" {
  context = "."
  dockerfile = "Dockerfile"
  target = "build"
  args = {
    APP_VERSION = APP_VERSION
    BUILD_HASH = BUILD_HASH
  }
  output = ["type=local,dest=dist"]
}

target "app" {
  context = "./dist"
  dockerfile = "../internal/installer/image.Dockerfile"
  platforms = ["linux/amd64", "linux/arm64"]
  tags = ["${REPO}:${TAG}", "${REPO}:sha-${BUILD_HASH}"]
  labels = {
    "org.opencontainers.image.title" = "cxz"
    "org.opencontainers.image.source" = "https://github.com/lesomnus/cxz"
    "org.opencontainers.image.revision" = BUILD_HASH
    "org.opencontainers.image.version" = APP_VERSION
  }
}
