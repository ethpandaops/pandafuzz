# Multi-stage Dockerfile for PandaFuzz

# Build stage for HongFuzz
FROM ubuntu:22.04 AS honggfuzz-builder

# Install build dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    libunwind-dev \
    libblocksruntime-dev \
    liblzma-dev \
    libssl-dev \
    zlib1g-dev \
    pkg-config \
    clang \
    binutils-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Build HongFuzz from source with full instrumentation support
# The hfuzz_cc compiler wrappers provide the instrumentation needed for fuzzing
# HFBUILD_USE_BFD=0 disables the optional BFD disassembler support to avoid dependency issues
RUN git clone --depth 1 --branch 2.6 https://github.com/google/honggfuzz.git && \
    cd honggfuzz && \
    # Build HongFuzz with clang for better instrumentation support
    # BUILD_LINUX_NO_BFD=true disables the BFD disassembler which requires binutils headers
    CC=clang CXX=clang++ make BUILD_LINUX_NO_BFD=true && \
    # Install the main honggfuzz binary
    cp honggfuzz /usr/bin/ && \
    # Install the hfuzz_cc compiler wrappers (these provide instrumentation)
    cp hfuzz_cc/hfuzz-cc /usr/bin/ && \
    cp hfuzz_cc/hfuzz-clang /usr/bin/ && \
    cp hfuzz_cc/hfuzz-clang++ /usr/bin/ && \
    cp hfuzz_cc/hfuzz-g++ /usr/bin/ && \
    cp hfuzz_cc/hfuzz-gcc /usr/bin/ && \
    chmod +x /usr/bin/hfuzz-* && \
    # Install the libhfuzz libraries needed for instrumentation
    mkdir -p /usr/local/lib/hfuzz && \
    cp libhfuzz/*.a /usr/local/lib/hfuzz/ 2>/dev/null || true && \
    cp libhfuzz/*.o /usr/local/lib/hfuzz/ 2>/dev/null || true && \
    # Verify installation
    echo "HongFuzz build completed:" && \
    ls -la /usr/bin/hfuzz-* && \
    ls -la /usr/local/lib/hfuzz/ 2>/dev/null || echo "No libhfuzz files found" && \
    cd / && rm -rf /build

# Build stage for web UI
FROM node:16-alpine AS web-builder

WORKDIR /build

# Copy web source
COPY web/package*.json ./

# Install dependencies with clean install
RUN npm install --legacy-peer-deps

COPY web/ ./

# Set NODE_OPTIONS to increase memory limit for build
ENV NODE_OPTIONS="--max-old-space-size=2048"
ENV SKIP_PREFLIGHT_CHECK=true
ENV GENERATE_SOURCEMAP=false

RUN npm run build

# Build stage for Go binaries
FROM golang:1.23 AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y \
    git make gcc libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod ./
COPY go.sum* ./

# Download dependencies - if go.sum is incomplete, this will download missing ones
RUN go mod download all

# Install oapi-codegen for API code generation
RUN go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

# Copy source code
COPY . .

# Generate API code before building
RUN mkdir -p pkg/api/v1/generated && \
    /go/bin/oapi-codegen -generate "types" -package generated -o pkg/api/v1/generated/types.gen.go pkg/api/v1/openapi/pandafuzz.yaml && \
    /go/bin/oapi-codegen -generate "chi-server" -package generated -o pkg/api/v1/generated/server.gen.go pkg/api/v1/openapi/pandafuzz.yaml && \
    /go/bin/oapi-codegen -generate "spec" -package generated -o pkg/api/v1/generated/spec.gen.go pkg/api/v1/openapi/pandafuzz.yaml

# Get version info
ARG VERSION=dev
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown

# Build binaries with version info
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}" \
    -o pandafuzz-master ./cmd/master
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}" \
    -o pandafuzz-bot ./cmd/bot

# Runtime stage for master
FROM ubuntu:22.04 AS master

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install runtime dependencies with retry logic for flaky mirrors
RUN apt-get update || apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    libsqlite3-0 \
    sqlite3 \
    curl \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -g 1000 pandafuzz && \
    useradd -u 1000 -g pandafuzz -m -s /bin/bash pandafuzz

# Create necessary directories
RUN mkdir -p /app/data /app/logs && \
    chown -R pandafuzz:pandafuzz /app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/pandafuzz-master /app/
COPY --from=builder /build/master.yaml /app/

# Copy web UI build to where the server expects it
COPY --from=web-builder /build/build /app/web/static

# Switch to non-root user
USER pandafuzz

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run master
CMD ["./pandafuzz-master", "-config", "master.yaml"]

# Runtime stage for bot with fuzzing tools
FROM ubuntu:22.04 AS bot

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install runtime dependencies and fuzzing tools with retry logic
# Fix: Add more essential packages for AFL++ and ensure proper LLVM setup
RUN apt-get update || apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    libsqlite3-0 \
    sqlite3 \
    bash \
    git \
    make \
    gcc \
    g++ \
    libc6-dev \
    clang-14 \
    llvm-14 \
    llvm-14-dev \
    llvm-14-tools \
    libclang-14-dev \
    lld-14 \
    python3 \
    python3-dev \
    python3-pip \
    libstdc++6 \
    libc++1 \
    libc++-dev \
    libc++abi-dev \
    automake \
    autoconf \
    libtool \
    libgmp-dev \
    zlib1g-dev \
    wget \
    curl \
    file \
    binutils \
    libasan6 \
    libubsan1 \
    libtsan0 \
    libfuzzer-14-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Set up LLVM alternatives to ensure correct version is used
RUN update-alternatives --install /usr/bin/llvm-config llvm-config /usr/bin/llvm-config-14 100 && \
    update-alternatives --install /usr/bin/clang clang /usr/bin/clang-14 100 && \
    update-alternatives --install /usr/bin/clang++ clang++ /usr/bin/clang++-14 100 && \
    update-alternatives --install /usr/bin/llvm-profdata llvm-profdata /usr/bin/llvm-profdata-14 100 && \
    update-alternatives --install /usr/bin/llvm-cov llvm-cov /usr/bin/llvm-cov-14 100

# Set LLVM_CONFIG for AFL++ build
ENV LLVM_CONFIG=llvm-config-14
ENV CC=clang-14
ENV CXX=clang++-14

# Install AFL++ with proper build flags and from a stable release
RUN cd /tmp && \
    wget https://github.com/AFLplusplus/AFLplusplus/archive/refs/tags/v4.21c.tar.gz && \
    tar -xzf v4.21c.tar.gz && \
    cd AFLplusplus-4.21c && \
    # Build without Python support to avoid compatibility issues
    NO_PYTHON=1 NO_NYX=1 make all && \
    make install && \
    cd / && \
    rm -rf /tmp/AFLplusplus-4.21c /tmp/v4.21c.tar.gz

# Create non-root user
RUN groupadd -g 1000 pandafuzz && \
    useradd -u 1000 -g pandafuzz -m -s /bin/bash pandafuzz

# Create necessary directories
RUN mkdir -p /app/work /app/logs /app/data/binaries && \
    chown -R pandafuzz:pandafuzz /app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/pandafuzz-bot /app/
COPY --from=builder /build/bot.yaml /app/

# Set AFL++ environment variables
ENV AFL_SKIP_CPUFREQ=1
ENV AFL_NO_AFFINITY=1
ENV AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES=1
ENV AFL_NO_UI=1
# Removed AFL_SKIP_BIN_CHECK to allow AFL++ to detect instrumentation
# Fix: Add path for AFL++ to find its support files
ENV AFL_PATH=/usr/local/lib/afl
# Add AFL++ binaries to PATH
ENV PATH=/usr/local/bin:$PATH

# Set coverage environment variables
ENV LLVM_PROFILE_FILE=/app/work/coverage-%p-%m.profraw
ENV AFL_LLVM_DOCUMENT_IDS=1
ENV AFL_LLVM_CMPLOG=1

# Install HongFuzz runtime dependencies
RUN apt-get update && apt-get install -y \
    libunwind8 \
    libblocksruntime0 \
    libssl3 \
    liblzma5 \
    && rm -rf /var/lib/apt/lists/*

# Copy HongFuzz binaries and compiler wrappers from builder stage
COPY --from=honggfuzz-builder /usr/bin/honggfuzz /usr/local/bin/
COPY --from=honggfuzz-builder /usr/bin/hfuzz-cc /usr/local/bin/
COPY --from=honggfuzz-builder /usr/bin/hfuzz-clang /usr/local/bin/
COPY --from=honggfuzz-builder /usr/bin/hfuzz-clang++ /usr/local/bin/
COPY --from=honggfuzz-builder /usr/bin/hfuzz-g++ /usr/local/bin/
COPY --from=honggfuzz-builder /usr/bin/hfuzz-gcc /usr/local/bin/
COPY --from=honggfuzz-builder /usr/local/lib/hfuzz /usr/local/lib/hfuzz

# Install coverage tools
# Note: genhtml is part of lcov package, afl-cov needs to be installed from source
RUN apt-get update && apt-get install -y \
    lcov \
    gcovr \
    perl \
    && rm -rf /var/lib/apt/lists/* && \
    # Install afl-cov from GitHub
    cd /tmp && \
    wget https://github.com/vanhauser-thc/afl-cov/archive/refs/tags/0.6.2.tar.gz && \
    tar -xzf 0.6.2.tar.gz && \
    cd afl-cov-0.6.2 && \
    cp afl-cov /usr/local/bin/ && \
    chmod +x /usr/local/bin/afl-cov && \
    cd / && \
    rm -rf /tmp/afl-cov-0.6.2 /tmp/0.6.2.tar.gz && \
    # Verify LLVM coverage tools are accessible
    ln -sf /usr/bin/llvm-profdata-14 /usr/bin/llvm-profdata || true && \
    ln -sf /usr/bin/llvm-cov-14 /usr/bin/llvm-cov || true

# Set proper permissions for AFL++ and ensure binaries are accessible
RUN chmod 755 /usr/local/bin/afl-* && \
    # Create symbolic links for AFL++ binaries if not already in PATH
    for tool in /usr/local/bin/afl-*; do \
        if [ -f "$tool" ] && [ ! -f "/usr/bin/$(basename $tool)" ]; then \
            ln -sf "$tool" "/usr/bin/$(basename $tool)"; \
        fi \
    done && \
    # Validate coverage tools installation
    echo "Validating coverage tools installation..." && \
    which llvm-profdata && echo "llvm-profdata found" && \
    which llvm-cov && llvm-cov -version | head -1 && \
    which afl-clang-fast && echo "afl-clang-fast found" && \
    which lcov && lcov --version | head -1 && \
    which genhtml && genhtml --version | head -1 && \
    which afl-cov && echo "afl-cov found" && \
    # Test LibFuzzer availability (optional - may not work in all environments)
    (echo 'extern "C" int LLVMFuzzerTestOneInput(const uint8_t*, size_t) { return 0; }' | \
        clang++ -fsanitize=fuzzer -x c++ -o /tmp/test_fuzzer - 2>/dev/null && \
        echo "LibFuzzer support validated" && rm -f /tmp/test_fuzzer) || \
        echo "LibFuzzer support not available (will be handled at runtime)" && \
    echo "All coverage tools validated successfully!"

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

# Install additional development tools with retry logic
RUN apt-get update || apt-get update && \
    apt-get install -y --no-install-recommends \
    vim \
    tmux \
    htop \
    strace \
    gdb \
    valgrind \
    linux-tools-generic \
    tcpdump \
    netcat-openbsd \
    jq \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install Go for development
COPY --from=builder /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"

# HongFuzz already copied in bot stage

USER pandafuzz

# Development environment
ENV PANDAFUZZ_DEV=true