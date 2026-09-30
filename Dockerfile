# Multi-stage Dockerfile for SlimBox
# Fully supports linux/arm64 (Phicomm N1, Raspberry Pi 3/4/5) and linux/amd64

FROM --platform=$BUILDPLATFORM golang:alpine AS builder

WORKDIR /build

ENV GOPROXY=https://goproxy.cn,direct \
    GOTOOLCHAIN=auto

# Copy dependency manifests
COPY go.mod go.sum* ./
RUN go mod download

# Copy source tree
COPY . .
RUN go mod tidy

# Native multi-arch cross compilation with separate target artifact names
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /build/slimbox_${TARGETARCH} ./cmd/slimbox
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /build/slimbox-cli_${TARGETARCH} ./cmd/slimbox-cli

# Pre-compile multi-platform standalone CLI binaries for direct web downloads
RUN mkdir -p /build/cli-dist && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o /build/cli-dist/slimbox-cli-windows-amd64.exe ./cmd/slimbox-cli && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /build/cli-dist/slimbox-cli-linux-amd64 ./cmd/slimbox-cli && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /build/cli-dist/slimbox-cli-linux-arm64 ./cmd/slimbox-cli && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o /build/cli-dist/slimbox-cli-darwin-arm64 ./cmd/slimbox-cli && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o /build/cli-dist/slimbox-cli-darwin-amd64 ./cmd/slimbox-cli

# Stage 2: Minimal runtime image with FFmpeg
FROM alpine:3.20

# Use fast mirror for apk in China
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

# Target hardware acceleration flavor: all/standard (Intel + AMD), intel, amd, or arm64 (pure CPU)
ARG FLAVOR=all

# Install FFmpeg and essential utilities with targeted hardware acceleration drivers
RUN apk add --no-cache \
    ffmpeg \
    tzdata \
    ca-certificates && \
    if [ "$(uname -m)" = "x86_64" ]; then \
        case "${FLAVOR}" in \
            intel) apk add --no-cache libva-intel-driver intel-media-driver || true ;; \
            amd)   apk add --no-cache mesa-va-gallium || true ;; \
            arm64) true ;; \
            all|standard|*) apk add --no-cache libva-intel-driver intel-media-driver mesa-va-gallium || true ;; \
        esac; \
    fi

# Create mount points and non-root data directories
ENV DOCKER_CONTAINER=1 \
    SLIMBOX_DATA_DIR=/data \
    PORT=8080

RUN mkdir -p /data/uploads /data/outputs /app/cli-dist

ARG TARGETARCH
# Copy statically linked binaries for exact matching target architecture
COPY --from=builder /build/slimbox_${TARGETARCH} /usr/local/bin/slimbox
COPY --from=builder /build/slimbox-cli_${TARGETARCH} /usr/local/bin/slimbox-cli
COPY --from=builder /build/cli-dist/ /app/cli-dist/

# Health check rule for Docker / 1Panel
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -q --spider http://127.0.0.1:8080/api/v1/auth/status || exit 1

# Persistent storage volume (mount external USB drive here)
VOLUME ["/data"]

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/slimbox"]
