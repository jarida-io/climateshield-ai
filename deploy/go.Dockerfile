# SPDX-License-Identifier: Apache-2.0
# One Dockerfile for all Go services: docker build --build-arg CMD=publicapi …
FROM golang:1.26-alpine AS build
ARG CMD
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/app ./cmd/${CMD}

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
USER app
COPY --from=build /out/app /app
# Golden fixtures ship in every image so the fixture climate source (demo/CI
# default) works inside containers; CLIMATE_FIXTURE_DIR points here.
COPY --from=build /src/testdata/golden /testdata/golden
ENTRYPOINT ["/app"]
