# PandaFuzz Configuration Guide

## Overview

PandaFuzz uses YAML configuration files for both Master and Bot components. Configuration can be provided via files or environment variables.

## Master Configuration Files

The project includes two master configuration files:
- `master.yaml` - Default configuration for local development
- `master-docker.yaml` - Docker-specific configuration with absolute paths and container-optimized settings

## Master Configuration

### Example (master.yaml)

```yaml
server:
  port: 8080
  timeout: 30s

database:
  type: sqlite  # or badger
  path: /storage/data/pandafuzz.db

storage:
  path: /storage

timeouts:
  bot_heartbeat: 60s
  job_execution: 3600s
  master_recovery: 300s

limits:
  max_concurrent_jobs: 10
  max_corpus_size: 1GB
  max_crash_size: 10MB
```

### Configuration Options

#### Server Settings
- `port`: HTTP server port (default: 8080)
- `timeout`: Request timeout duration

#### Database Settings
- `type`: Database backend (`sqlite` or `badger`)
- `path`: Database file location

#### Storage Settings
- `path`: Root storage directory for corpus, crashes, and metadata

#### Timeout Settings
- `bot_heartbeat`: Maximum time between bot heartbeats
- `job_execution`: Maximum job execution time
- `master_recovery`: Recovery timeout after master restart

#### Resource Limits
- `max_concurrent_jobs`: Maximum parallel jobs
- `max_corpus_size`: Maximum corpus storage per job
- `max_crash_size`: Maximum individual crash file size

## Bot Configuration

### Example (configs/bot.yaml)

```yaml
bot:
  id: ${BOT_ID}
  master_url: ${MASTER_URL}
  heartbeat_interval: 30s
  timeout: 30s

fuzzing:
  work_dir: /tmp/fuzzing
  capabilities: ["afl++", "libfuzzer"]
  
timeouts:
  job_execution: 3600s
  master_communication: 30s
```

### Configuration Options

#### Bot Settings
- `id`: Unique bot identifier (usually from environment)
- `master_url`: Master API endpoint
- `heartbeat_interval`: Heartbeat frequency
- `timeout`: General operation timeout

#### Fuzzing Settings
- `work_dir`: Local working directory for fuzzing
- `capabilities`: Supported fuzzing engines

#### Timeout Settings
- `job_execution`: Maximum time for a single job
- `master_communication`: API request timeout

## Environment Variables

Both Master and Bot support environment variable substitution in configuration files using `${VAR_NAME}` syntax.

### Common Variables
- `BOT_ID`: Unique bot identifier
- `MASTER_URL`: Master API endpoint
- `STORAGE_PATH`: Storage directory path
- `DATABASE_PATH`: Database file path

## Docker Configuration

When running in Docker, use the `master-docker.yaml` configuration which includes:
- Absolute paths for container filesystem
- Container-optimized resource limits
- Docker-specific networking settings

## Configuration Validation

PandaFuzz validates configuration on startup:
- Required fields must be present
- Paths must be accessible
- Timeouts must be positive durations
- Resource limits must be valid sizes

## Best Practices

1. **Production Settings**
   - Increase timeouts for network latency
   - Set appropriate resource limits
   - Use persistent storage paths

2. **Development Settings**
   - Shorter timeouts for faster feedback
   - Relaxed resource limits
   - Local storage paths

3. **Security**
   - Run behind VPN
   - Use absolute paths
   - Set restrictive file permissions