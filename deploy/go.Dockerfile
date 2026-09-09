# SPDX-License-Identifier: Apache-2.0
# One Dockerfile for all Go services: docker build --build-arg CMD=publicapi …
FROM golang:1.26-alpine AS build
ARG CMD
WORKDIR /src
COPY go.mod go.sum ./
# BuildKit cache mounts share the module download and the compiler cache
# across the eight service images built in parallel by `make up`. Without
# them every image re-downloads every module (the tool dependencies alone are
# hundreds of megabytes), which is what made a cold first build take so long.
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/app ./cmd/${CMD}

FROM alpine:3.22
# CA certificates come from the build stage rather than `apk add`, so the
# runtime image needs no package mirror: the build stays reproducible and
# works on a restricted network. busybox (in the base image) provides the
# wget used by the compose healthchecks. Numeric UID avoids needing adduser.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 10001:10001
COPY --from=build /out/app /app
# Golden fixtures ship in every image so the fixture climate source (demo/CI
# default) works inside containers; CLIMATE_FIXTURE_DIR points here.
COPY --from=build /src/testdata/golden /testdata/golden
ENTRYPOINT ["/app"]
