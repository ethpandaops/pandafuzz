# 09: Configuration Cleanup

## Priority: LOW
## Risk Level: LOW
## Estimated Effort: 4-6 hours

## Prerequisites

- Complete steps 01-08 first
- Especially step 03 (refactor common package) which moves config types

## Problem Statement

The configuration system has several usability issues:

1. **Deep nesting**: Configuration structures are deeply nested, making them hard to navigate
2. **Scattered defaults**: Default values are spread across code, YAML comments, and constants
3. **Feature flag sprawl**: Many `enable_*` flags suggest incomplete features
4. **Inconsistent naming**: Some fields use snake_case, others don't match
5. **Missing validation**: Some invalid configurations aren't caught early
6. **No environment variable support documented**: Env var overrides not clear

## Invariants (MUST NOT CHANGE)

1. Existing configuration files must continue to work
2. All documented configuration options must remain valid
3. Default behavior must remain unchanged
4. Docker container configuration must work

## Current Configuration Structure

**configs/master.yaml (142 lines):**
```yaml
master:
  server:
    host: "0.0.0.0"
    port: 8080
    # ... 8 more fields
  database:
    # ... 5 fields
  storage:
    # ... 10+ fields with complex nesting
  timeouts:
    # ... 9 fields
  limits:
    # ... 8 fields
  retry:
    database:      # nested
    bot_operation: # nested
    file_system:   # nested
    network:       # nested
  circuit:
    # ... 3 fields
  monitoring:
    # ... 8 fields
  security:
    # ... 10 fields
  logging:
    # ... 9 fields
```

## Proposed Improvements

### 1. Consolidate Defaults in One Place

**Create: pkg/config/defaults.go**
```go
package config

import "time"

// DefaultMasterConfig returns the default master configuration
func DefaultMasterConfig() *MasterConfig {
    return &MasterConfig{
        Server: ServerConfig{
            Host:           "0.0.0.0",
            Port:           8080,
            ReadTimeout:    30 * time.Second,
            WriteTimeout:   30 * time.Second,
            IdleTimeout:    60 * time.Second,
            MaxHeaderBytes: 1 << 20, // 1MB
            EnableTLS:      false,
            EnableCORS:     false,
            RateLimitRPS:   100,
            RateLimitBurst: 200,
        },
        Database: DatabaseConfig{
            Type:      "sqlite",
            Path:      "./data/pandafuzz.db",
            MaxConns:  1,
            IdleConns: 1,
            Timeout:   30 * time.Second,
        },
        Timeouts: TimeoutConfig{
            BotHeartbeat:    60 * time.Second,
            JobExecution:    3600 * time.Second,
            MasterRecovery:  300 * time.Second,
            DatabaseOp:      10 * time.Second,
            DatabaseRetries: 5,
            HTTPRequest:     30 * time.Second,
            BotRegistration: 60 * time.Second,
            JobAssignment:   30 * time.Second,
        },
        // ... complete defaults for all sections
    }
}

// DefaultBotConfig returns the default bot configuration
func DefaultBotConfig() *BotConfig {
    return &BotConfig{
        // ... defaults
    }
}
```

### 2. Add Configuration Validation

**Create: pkg/config/validate.go**
```go
package config

import (
    "fmt"
    "net"
    "time"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
    Field   string
    Value   interface{}
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("config validation error: %s = %v: %s", e.Field, e.Value, e.Message)
}

// Validate validates the master configuration
func (c *MasterConfig) Validate() error {
    var errors []error

    // Server validation
    if c.Server.Port < 1 || c.Server.Port > 65535 {
        errors = append(errors, &ValidationError{
            Field:   "server.port",
            Value:   c.Server.Port,
            Message: "port must be between 1 and 65535",
        })
    }

    if c.Server.ReadTimeout <= 0 {
        errors = append(errors, &ValidationError{
            Field:   "server.read_timeout",
            Value:   c.Server.ReadTimeout,
            Message: "read_timeout must be positive",
        })
    }

    // Database validation
    if c.Database.Type != "sqlite" && c.Database.Type != "postgres" {
        errors = append(errors, &ValidationError{
            Field:   "database.type",
            Value:   c.Database.Type,
            Message: "database type must be 'sqlite' or 'postgres'",
        })
    }

    if c.Database.MaxConns < 1 {
        errors = append(errors, &ValidationError{
            Field:   "database.max_conns",
            Value:   c.Database.MaxConns,
            Message: "max_conns must be at least 1",
        })
    }

    // Timeout validation
    if c.Timeouts.BotHeartbeat < 10*time.Second {
        errors = append(errors, &ValidationError{
            Field:   "timeouts.bot_heartbeat",
            Value:   c.Timeouts.BotHeartbeat,
            Message: "bot_heartbeat should be at least 10s",
        })
    }

    // Storage validation
    if err := c.Storage.Validate(); err != nil {
        errors = append(errors, err)
    }

    if len(errors) > 0 {
        return fmt.Errorf("configuration has %d errors: %v", len(errors), errors)
    }

    return nil
}

// Validate validates storage configuration
func (c *StorageConfig) Validate() error {
    switch c.Type {
    case "filesystem":
        if c.Filesystem.BasePath == "" {
            return &ValidationError{
                Field:   "storage.filesystem.base_path",
                Value:   "",
                Message: "base_path is required for filesystem storage",
            }
        }
    case "s3":
        if c.S3.Region == "" {
            return &ValidationError{
                Field:   "storage.s3.region",
                Value:   "",
                Message: "region is required for S3 storage",
            }
        }
        if c.S3.CorpusBucket == "" {
            return &ValidationError{
                Field:   "storage.s3.corpus_bucket",
                Value:   "",
                Message: "corpus_bucket is required for S3 storage",
            }
        }
    case "minio":
        if c.Minio.Endpoint == "" {
            return &ValidationError{
                Field:   "storage.minio.endpoint",
                Value:   "",
                Message: "endpoint is required for MinIO storage",
            }
        }
    default:
        return &ValidationError{
            Field:   "storage.type",
            Value:   c.Type,
            Message: "storage type must be 'filesystem', 's3', or 'minio'",
        }
    }

    return nil
}
```

### 3. Document Environment Variable Overrides

**Update: pkg/config/loader.go**
```go
package config

import (
    "os"
    "strconv"
    "strings"
    "time"
)

// Environment variable prefix
const EnvPrefix = "PANDAFUZZ_"

// LoadMasterConfig loads configuration from file and environment
func LoadMasterConfig(configPath string) (*MasterConfig, error) {
    // Start with defaults
    config := DefaultMasterConfig()

    // Load from file if provided
    if configPath != "" {
        if err := loadFromFile(configPath, config); err != nil {
            return nil, err
        }
    }

    // Override with environment variables
    applyEnvOverrides(config)

    // Validate
    if err := config.Validate(); err != nil {
        return nil, err
    }

    return config, nil
}

// applyEnvOverrides applies environment variable overrides
// Environment variables follow the pattern: PANDAFUZZ_<SECTION>_<FIELD>
// Examples:
//   PANDAFUZZ_SERVER_PORT=9090
//   PANDAFUZZ_DATABASE_PATH=/data/custom.db
//   PANDAFUZZ_TIMEOUTS_BOT_HEARTBEAT=120s
func applyEnvOverrides(config *MasterConfig) {
    // Server overrides
    if v := os.Getenv(EnvPrefix + "SERVER_HOST"); v != "" {
        config.Server.Host = v
    }
    if v := os.Getenv(EnvPrefix + "SERVER_PORT"); v != "" {
        if port, err := strconv.Atoi(v); err == nil {
            config.Server.Port = port
        }
    }

    // Database overrides
    if v := os.Getenv(EnvPrefix + "DATABASE_TYPE"); v != "" {
        config.Database.Type = v
    }
    if v := os.Getenv(EnvPrefix + "DATABASE_PATH"); v != "" {
        config.Database.Path = v
    }

    // Timeout overrides
    if v := os.Getenv(EnvPrefix + "TIMEOUTS_BOT_HEARTBEAT"); v != "" {
        if d, err := time.ParseDuration(v); err == nil {
            config.Timeouts.BotHeartbeat = d
        }
    }

    // Storage overrides
    if v := os.Getenv(EnvPrefix + "STORAGE_TYPE"); v != "" {
        config.Storage.Type = v
    }

    // ... more overrides
}
```

### 4. Simplify Nested Structures

**Flatten retry configuration:**

```yaml
# Before (deeply nested)
retry:
  database:
    max_retries: 5
    initial_delay: "1s"
    max_delay: "30s"
    multiplier: 2.0
  bot_operation:
    max_retries: 3
    # ...

# After (flattened with prefixes)
retry_database_max_retries: 5
retry_database_initial_delay: "1s"
retry_database_max_delay: "30s"
retry_database_multiplier: 2.0
retry_bot_max_retries: 3
# ...
```

**Or keep nested but provide helper methods:**
```go
func (c *MasterConfig) GetRetryPolicy(name string) RetryPolicy {
    switch name {
    case "database":
        return c.Retry.Database
    case "bot":
        return c.Retry.BotOperation
    case "filesystem":
        return c.Retry.FileSystem
    case "network":
        return c.Retry.Network
    default:
        return DefaultRetryPolicy()
    }
}
```

### 5. Remove/Document Feature Flags

Audit all `enable_*` flags and document status:

| Flag | Status | Action |
|------|--------|--------|
| `enable_tls` | Implemented | Document |
| `enable_cors` | Implemented | Document |
| `enable_metrics` | Implemented | Document |
| `enable_profiler` | Implemented | Document |
| `enable_input_validation` | Implemented | Document |
| `enable_sanitization` | Partial | Complete or remove |
| `enable_dedup` | Implemented | Document |
| `enable_compression` | Not implemented | Implement or remove |
| `enable_trace` | Partial | Complete or remove |

### 6. Create Configuration Reference Documentation

**docs/configuration.md:**
```markdown
# PandaFuzz Configuration Reference

## Master Configuration

### Server Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `server.host` | string | `"0.0.0.0"` | Server bind address |
| `server.port` | int | `8080` | Server port |
| `server.read_timeout` | duration | `30s` | HTTP read timeout |
| `server.write_timeout` | duration | `30s` | HTTP write timeout |
| `server.idle_timeout` | duration | `60s` | HTTP idle timeout |
| `server.enable_tls` | bool | `false` | Enable TLS |
| `server.enable_cors` | bool | `false` | Enable CORS |

### Database Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `database.type` | string | `"sqlite"` | Database type: `sqlite`, `postgres` |
| `database.path` | string | `"./data/pandafuzz.db"` | SQLite database path |
| `database.max_conns` | int | `1` | Maximum connections |
| `database.timeout` | duration | `30s` | Connection timeout |

### Environment Variables

All configuration options can be overridden with environment variables:

```bash
# Pattern: PANDAFUZZ_<SECTION>_<FIELD>
export PANDAFUZZ_SERVER_PORT=9090
export PANDAFUZZ_DATABASE_PATH=/data/custom.db
export PANDAFUZZ_TIMEOUTS_BOT_HEARTBEAT=120s
```

### Minimal Configuration

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
```

## Implementation Steps

### Step 1: Create Defaults File
```bash
# Create pkg/config/defaults.go with all defaults
```

### Step 2: Add Validation
```bash
# Create pkg/config/validate.go with validation logic
# Update LoadConfig to call Validate()
```

### Step 3: Add Env Override Support
```bash
# Update pkg/config/loader.go with env override logic
```

### Step 4: Update Documentation
```bash
# Create/update docs/configuration.md
# Add env var examples to configs/*.yaml
```

### Step 5: Clean Up Feature Flags
```bash
# Audit enable_* flags
# Implement or remove incomplete features
```

## Verification Steps

### 1. Default Config Works
```bash
# Start with no config file
./pandafuzz-master
# Should use all defaults
```

### 2. File Config Works
```bash
# Start with config file
./pandafuzz-master -config configs/master.yaml
```

### 3. Env Overrides Work
```bash
PANDAFUZZ_SERVER_PORT=9090 ./pandafuzz-master
# Should use port 9090
```

### 4. Validation Catches Errors
```bash
# Test with invalid config
echo "master:
  server:
    port: -1
" > /tmp/bad.yaml
./pandafuzz-master -config /tmp/bad.yaml
# Should fail with validation error
```

### 5. Docker Works
```bash
docker-compose up -d
# Should start with env vars from docker-compose.yml
```

## Notes for Future Runs

### Configuration Loading Order

1. Start with compiled defaults
2. Load from config file (if provided)
3. Apply environment variable overrides
4. Validate final configuration

### Duration Parsing

Go duration strings: `10s`, `5m`, `1h30m`, `24h`

### Backward Compatibility

When adding new config options:
1. Always provide a default
2. Make the new option optional
3. Document in configuration.md

### Secret Management

For production secrets (API keys, passwords):
- Use environment variables
- Consider secrets manager integration
- Never commit secrets to config files

## Completion Checklist

- [ ] Create pkg/config/defaults.go
- [ ] Create pkg/config/validate.go
- [ ] Add environment variable support
- [ ] Update configuration loading
- [ ] Audit and document feature flags
- [ ] Create docs/configuration.md
- [ ] Update example configs with comments
- [ ] Test default-only startup
- [ ] Test env var overrides
- [ ] Test validation error messages
- [ ] Update Docker documentation
