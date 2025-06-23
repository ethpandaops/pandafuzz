# Multi-stage Dockerfile for PandaFuzz

# Build stage for web UI
FROM node:18-alpine AS web-builder

WORKDIR /build

# Copy web source
COPY web/package*.json ./

# Clean install with dependency resolution
RUN rm -f package-lock.json && \
    npm cache clean --force && \
    npm install --legacy-peer-deps --force

COPY web/ ./

# Set NODE_OPTIONS to increase memory limit for build
ENV NODE_OPTIONS="--max-old-space-size=2048"

RUN npm run build

# Build stage for Go binaries
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev sqlite-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod ./
COPY go.sum* ./

# Download dependencies - if go.sum is incomplete, this will download missing ones
RUN go mod download all

# Copy source code
COPY . .

# Build binaries
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o pandafuzz-master ./cmd/master
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o pandafuzz-bot ./cmd/bot

# Runtime stage for master
FROM alpine:3.19 AS master

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs

# Create non-root user
RUN addgroup -g 1000 pandafuzz && \
    adduser -D -u 1000 -G pandafuzz pandafuzz

# Create necessary directories
RUN mkdir -p /app/data /app/logs && \
    chown -R pandafuzz:pandafuzz /app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/pandafuzz-master /app/
COPY --from=builder /build/master.yaml /app/

# Copy web UI build
COPY --from=web-builder /build/build /app/web/build

# Switch to non-root user
USER pandafuzz

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run master
CMD ["./pandafuzz-master", "-config", "master.yaml"]

# Runtime stage for bot with fuzzing tools
FROM alpine:3.19 AS bot

# Install runtime dependencies and fuzzing tools
RUN apk add --no-cache \
    ca-certificates \
    sqlite-libs \
    bash \
    git \
    make \
    gcc \
    g++ \
    musl-dev \
    clang \
    llvm \
    compiler-rt \
    python3 \
    py3-pip \
    libstdc++ \
    # AFL++ dependencies
    automake \
    autoconf \
    libtool \
    # Additional tools
    wget \
    curl \
    file

# Install AFL++
RUN git clone https://github.com/AFLplusplus/AFLplusplus.git /tmp/aflplusplus && \
    cd /tmp/aflplusplus && \
    make all && \
    make install && \
    rm -rf /tmp/aflplusplus

# Create non-root user
RUN addgroup -g 1000 pandafuzz && \
    adduser -D -u 1000 -G pandafuzz pandafuzz

# Create necessary directories
RUN mkdir -p /app/work /app/logs && \
    chown -R pandafuzz:pandafuzz /app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/pandafuzz-bot /app/
COPY --from=builder /build/bot.yaml /app/

# Set AFL++ environment variables
ENV AFL_SKIP_CPUFREQ=1
ENV AFL_NO_AFFINITY=1
ENV AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES=1

# Switch to non-root user
USER pandafuzz

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD pgrep pandafuzz-bot || exit 1

# Run bot
CMD ["./pandafuzz-bot", "-config", "bot.yaml"]

# Development stage with all tools
FROM bot AS development

USER root

# Install additional development tools
RUN apk add --no-cache \
    vim \
    tmux \
    htop \
    strace \
    gdb \
    valgrind \
    perf-tools \
    tcpdump \
    netcat-openbsd

# Install Go for development
COPY --from=builder /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"

# Install additional fuzzing tools
# Honggfuzz
RUN git clone https://github.com/google/honggfuzz.git /tmp/honggfuzz && \
    cd /tmp/honggfuzz && \
    make && \
    cp honggfuzz /usr/local/bin/ && \
    rm -rf /tmp/honggfuzz

USER pandafuzz

# Development environment
ENV PANDAFUZZ_DEV=true