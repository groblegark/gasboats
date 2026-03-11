# roles-server: web UI for managing agent roles and configuration.
# Multi-stage build: Go builder -> distroless runtime.
#
# Build:
#   docker build -t gasboat/roles-server:latest -f images/roles-server/Dockerfile .

FROM golang:1.25-bookworm AS builder

ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /build
COPY controller/ ./

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /roles-server ./cmd/roles-server/

# -- Runtime -------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /roles-server /roles-server

USER nonroot:nonroot
EXPOSE 8092

ENTRYPOINT ["/roles-server"]
