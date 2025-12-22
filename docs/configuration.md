# PandaFuzz Configuration Guide

## Overview

PandaFuzz uses YAML configuration files for both Master and Bot components. Configuration can be:
1. Provided via YAML files
2. Overridden with environment variables (prefix: `PANDAFUZZ_`)
3. Interpolated with `${VAR_NAME}` syntax in YAML files

## Configuration Loading Order

1. Default values are applied first
2. YAML file values override defaults
3. Environment variables override YAML values
4. Configuration is validated before use

## Master Configuration

### Complete Example

```yaml
master:
  # Server configuration
  server:
    host: "0.0.0.0"
    port: 8080
    read_timeout: "30s"
    write_timeout: "30s"
    idle_timeout: "60s"
    max_header_bytes: 1048576
    enable_tls: false
    enable_cors: false
    rate_limit_rps: 100
    rate_limit_burst: 200

  # Database configuration
  database:
    type: "sqlite"
    path: "./data/pandafuzz.db"
    max_conns: 1
    idle_conns: 1
    timeout: "30s"

  # Storage configuration
  storage:
    type: filesystem  # filesystem, s3, or minio
    filesystem:
      base_path: ./storage/corpus
    max_file_size: 104857600  # 100MB
    enable_dedup: true
    enable_compression: false

  # Timeout configuration
  timeouts:
    bot_heartbeat: "60s"
    job_execution: "3600s"
    master_recovery: "300s"
    database_op: "10s"
    database_retries: 5
    http_request: "30s"
    bot_registration: "60s"
    job_assignment: "30s"

  # Resource limits
  limits:
    max_concurrent_jobs: 10
    max_corpus_size: 1073741824  # 1GB
    max_crash_size: 10485760     # 10MB
    max_crash_count: 1000
    max_job_duration: "24h"
    max_bots_per_cluster: 100
    max_pending_jobs: 1000

  # Retry configuration
  retry:
    database:
      max_retries: 5
      initial_delay: "1s"
      max_delay: "30s"
      multiplier: 2.0
    bot_operation:
      max_retries: 3
      initial_delay: "1s"
      max_delay: "30s"
      multiplier: 2.0

  # Circuit breaker configuration
  circuit:
    max_failures: 5
    reset_timeout: "60s"
    enabled: true

  # Monitoring configuration
  monitoring:
    enabled: true
    metrics_enabled: true
    metrics_port: 9090
    metrics_path: "/metrics"
    health_enabled: true
    health_path: "/health"
    stats_interval: "30s"
    profiler_enabled: false

  # Security configuration
  security:
    enable_input_validation: true
    max_request_size: 10485760
    allowed_file_extensions: [".txt", ".bin", ".data", ".input"]
    forbidden_paths: ["/etc", "/proc", "/sys"]
    enable_sanitization: true
    max_crash_file_size: 10485760
    max_corpus_file_size: 1048576
    process_isolation_level: "sandbox"

  # Logging configuration
  logging:
    level: "info"
    format: "json"
    output: "file"
    file_path: "./logs/master.log"
    max_size: 100
    max_backups: 10
    max_age: 30
    compress: true
    enable_trace: false
```

### Configuration Sections

#### Server Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `"0.0.0.0"` | Server bind address |
| `port` | int | `8080` | HTTP server port |
| `read_timeout` | duration | `30s` | HTTP read timeout |
| `write_timeout` | duration | `30s` | HTTP write timeout |
| `idle_timeout` | duration | `60s` | HTTP idle timeout |
| `enable_tls` | bool | `false` | Enable TLS/HTTPS |
| `enable_cors` | bool | `false` | Enable CORS headers |
| `rate_limit_rps` | int | `100` | Rate limit requests/second |
| `rate_limit_burst` | int | `200` | Rate limit burst size |

#### Database Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `"sqlite"` | Database type: `sqlite`, `postgres` |
| `path` | string | `"./data/pandafuzz.db"` | SQLite database path |
| `max_conns` | int | `1` | Maximum open connections |
| `idle_conns` | int | `1` | Maximum idle connections |
| `timeout` | duration | `30s` | Connection timeout |

#### Storage Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `"filesystem"` | Storage type: `filesystem`, `s3`, `minio` |
| `max_file_size` | int | `104857600` | Maximum file size (100MB) |
| `enable_dedup` | bool | `true` | Enable file deduplication |
| `enable_compression` | bool | `false` | Enable file compression |

#### Timeout Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `bot_heartbeat` | duration | `60s` | Bot heartbeat timeout |
| `job_execution` | duration | `3600s` | Job execution timeout |
| `master_recovery` | duration | `300s` | Master recovery timeout |
| `database_op` | duration | `10s` | Database operation timeout |
| `http_request` | duration | `30s` | HTTP request timeout |

#### Resource Limits

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_concurrent_jobs` | int | `10` | Maximum parallel jobs |
| `max_corpus_size` | int | `1073741824` | Max corpus size (1GB) |
| `max_crash_size` | int | `10485760` | Max crash file size (10MB) |
| `max_crash_count` | int | `1000` | Maximum stored crashes |
| `max_job_duration` | duration | `24h` | Maximum job duration |

## Bot Configuration

### Complete Example

```yaml
bot:
  id: ${BOT_ID:-bot-1}
  name: "fuzzer-bot"
  master_url: ${MASTER_URL:-http://localhost:8080}
  api_port: 9049
  capabilities: ["afl++", "libfuzzer", "honggfuzz"]

  fuzzing:
    work_dir: /tmp/pandafuzz
    max_jobs: 1
    job_cleanup: true
    corpus_sync: true
    crash_reporting: true
    coverage_reporting: true
    fuzzer_paths:
      afl++: /usr/local/bin/afl-fuzz
      libfuzzer: ""
      honggfuzz: /usr/local/bin/honggfuzz

  timeouts:
    heartbeat_interval: "30s"
    job_execution: "3600s"
    master_communication: "30s"
    fuzzer_startup: "60s"
    result_reporting: "30s"

  retry:
    communication:
      max_retries: 3
      initial_delay: "1s"
      max_delay: "30s"
      multiplier: 2.0

  resources:
    max_memory_mb: 2048
    max_cpu_percent: 80
    max_disk_space_mb: 10240
    max_open_files: 1024
    max_processes: 100

  logging:
    level: "info"
    format: "json"
    output: "file"
```

## Environment Variables

All configuration options can be overridden with environment variables using the `PANDAFUZZ_` prefix:

### Variable Naming Convention

```
PANDAFUZZ_<SECTION>_<FIELD>
```

### Examples

```bash
# Server configuration
export PANDAFUZZ_SERVER_HOST=0.0.0.0
export PANDAFUZZ_SERVER_PORT=9090

# Database configuration
export PANDAFUZZ_DATABASE_TYPE=sqlite
export PANDAFUZZ_DATABASE_PATH=/data/custom.db

# Timeouts
export PANDAFUZZ_TIMEOUTS_BOT_HEARTBEAT=120s
export PANDAFUZZ_TIMEOUTS_JOB_EXECUTION=7200s

# Storage
export PANDAFUZZ_STORAGE_TYPE=filesystem
export PANDAFUZZ_STORAGE_MAX_FILE_SIZE=209715200

# Logging
export PANDAFUZZ_LOGGING_LEVEL=debug
export PANDAFUZZ_LOGGING_FORMAT=text

# Bot-specific
export BOT_ID=bot-worker-1
export MASTER_URL=http://master:8080
```

### YAML Variable Interpolation

Environment variables can be used in YAML files:

```yaml
bot:
  id: ${BOT_ID:-default-bot}
  master_url: ${MASTER_URL}

database:
  path: ${DATABASE_PATH:-./data/pandafuzz.db}
```

## Storage Backend Configuration

### Filesystem (Default)

```yaml
storage:
  type: filesystem
  filesystem:
    base_path: ./storage/corpus
```

### MinIO

```yaml
storage:
  type: minio
  minio:
    endpoint: localhost:9000
    access_key_id: pandafuzz
    secret_access_key: pandafuzz123
    corpus_bucket: corpus
    quarantine_bucket: quarantine
    backup_bucket: backup
    use_ssl: false
    use_path_style: true
```

### AWS S3

```yaml
storage:
  type: s3
  s3:
    region: us-east-1
    access_key_id: ${AWS_ACCESS_KEY_ID}
    secret_access_key: ${AWS_SECRET_ACCESS_KEY}
    corpus_bucket: pandafuzz-corpus
    quarantine_bucket: pandafuzz-quarantine
    use_ssl: true
```

## Configuration Validation

PandaFuzz validates configuration on startup:
- Server port must be 1-65535
- Timeouts must be positive durations
- Resource limits must be positive values
- Storage paths must be accessible
- Required fields must be present

Invalid configuration will cause startup failure with descriptive error messages.

## Minimal Configuration

For quick start, only these settings are typically needed:

```yaml
master:
  server:
    port: 8080
  database:
    path: ./data/pandafuzz.db
  storage:
    type: filesystem
    filesystem:
      base_path: ./storage
```

All other settings use sensible defaults.

## Docker Configuration

When running in Docker, use environment variables or mount configuration:

```bash
docker run -d \
  -e PANDAFUZZ_SERVER_PORT=8080 \
  -e PANDAFUZZ_DATABASE_PATH=/data/pandafuzz.db \
  -e PANDAFUZZ_STORAGE_TYPE=filesystem \
  -v ./data:/data \
  -v ./storage:/storage \
  pandafuzz-master
```

## Best Practices

1. **Production**
   - Increase timeouts for network latency
   - Set appropriate resource limits
   - Use persistent storage (S3/MinIO for high availability)
   - Enable TLS for security

2. **Development**
   - Shorter timeouts for faster feedback
   - Local filesystem storage
   - Debug logging level

3. **Security**
   - Never commit secrets to config files
   - Use environment variables for credentials
   - Set restrictive file permissions
   - Enable input validation
