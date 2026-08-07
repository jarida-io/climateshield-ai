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
ENTRYPOINT ["/app"]
