# Multi-stage Dockerfile for SlimBox
# Fully supports linux/arm64 (Phicomm N1, Raspberry Pi 3/4/5) and linux/amd64

FROM golang:alpine AS builder

WORKDIR /build

ENV GOPROXY=https://goproxy.cn,direct \
    GOTOOLCHAIN=auto

# Copy dependency manifests
COPY go.mod go.sum* ./
RUN go mod download

# Copy source tree
COPY . .

RUN go mod tidy

# Compile server and CLI with CGO_ENABLED=0 for maximum portability
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /build/slimbox ./cmd/slimbox
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /build/slimbox-cli ./cmd/slimbox-cli

# Stage 2: Minimal runtime image with FFmpeg
FROM alpine:3.20

# Use fast mirror for apk in China
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

# Install FFmpeg and essential utilities
RUN apk add --no-cache \
    ffmpeg \
    tzdata \
    ca-certificates

# Create mount points and non-root data directories
ENV DOCKER_CONTAINER=1 \
    SLIMBOX_DATA_DIR=/data \
    PORT=8080

RUN mkdir -p /data/uploads /data/outputs

# Copy statically linked binaries from builder
COPY --from=builder /build/slimbox /usr/local/bin/slimbox
COPY --from=builder /build/slimbox-cli /usr/local/bin/slimbox-cli

# Persistent storage volume (mount external USB drive here)
VOLUME ["/data"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:8080/api/v1/system/stats || exit 1

ENTRYPOINT ["/usr/local/bin/slimbox"]
CMD ["-data-dir", "/data", "-port", "8080"]
