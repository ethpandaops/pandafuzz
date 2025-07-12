# Fuzzer Configuration in PandaFuzz

## Understanding Fuzzer Arguments

PandaFuzz separates job-level configuration from fuzzer-specific arguments:

### Job-Level Configuration (via API)
These settings are configured per job through the API:
- `duration` - Maximum runtime for the job
- `memory_limit` - Memory limit in bytes
- `timeout` - Execution timeout for each test case
- `dictionary` - Path to dictionary file
- `seed_corpus` - Array of seed corpus paths
- `output_dir` - Output directory for results

### Fuzzer-Specific Arguments (via Bot Config)
Fuzzer-specific arguments like AFL++ flags (`-m`, `-t`, `-p`) or LibFuzzer options (`-max_len`, `-use_value_profile`) are configured at the bot level in the bot configuration file.

## Bot Configuration Example

Create or edit `/app/config/bot.yaml` in your bot container:

```yaml
# Bot configuration with fuzzer-specific settings
bot:
  id: "bot-1"
  name: "Fuzzing Bot 1"
  
# Fuzzer configurations
fuzzers:
  afl++:
    enabled: true
    # Default AFL++ arguments applied to all jobs
    default_args:
      - "-m"
      - "none"     # No memory limit
      - "-t"
      - "1000"     # 1 second timeout
      - "-p"
      - "fast"     # Power schedule
    
  libfuzzer:
    enabled: true
    # Default LibFuzzer arguments
    default_args:
      - "-max_len=4096"
      - "-len_control=100"
      - "-use_value_profile=1"
      - "-print_stats=1"
      - "-close_fd_mask=3"
    # Environment variables for LibFuzzer
    env:
      ASAN_OPTIONS: "detect_leaks=1:halt_on_error=0:print_stats=1"
      UBSAN_OPTIONS: "print_stacktrace=1:halt_on_error=0"
```

## Applying Bot Configuration

1. **Copy configuration to bot:**
   ```bash
   docker cp bot.yaml pandafuzz-bot-1:/app/config/bot.yaml
   ```

2. **Restart bot to apply changes:**
   ```bash
   docker restart pandafuzz-bot-1
   ```

## Per-Target Configuration

For more advanced setups, you can create target-specific configurations:

### Option 1: Wrapper Scripts
Create a wrapper script for your target:

```bash
#!/bin/bash
# /app/targets/example_afl_wrapper.sh

# Target-specific AFL++ arguments
exec afl-fuzz -m none -t 2000 -p explore -i "$1" -o "$2" -- /app/targets/example_afl
```

Then use the wrapper as your target in job creation.

### Option 2: Configuration Files
Some fuzzers support configuration files:

**LibFuzzer options file** (`/app/targets/libfuzzer.options`):
```
max_len = 4096
len_control = 100
use_value_profile = 1
detect_leaks = 1
```

Set environment variable in bot config:
```yaml
libfuzzer:
  env:
    LIBFUZZER_OPTIONS_FILE: "/app/targets/libfuzzer.options"
```

## Common Fuzzer Arguments

### AFL++ Common Arguments
- `-m none` - No memory limit (let ASAN handle it)
- `-t 1000` - Timeout per test case (ms)
- `-p fast/explore/coe/lin/quad` - Power schedule
- `-L 0` - Disable trimming for speed
- `-c 0` - Disable CPU affinity
- `-a binary` - Assume binary input

### LibFuzzer Common Arguments
- `-max_len=N` - Maximum input length
- `-len_control=N` - Length control coefficient
- `-use_value_profile=1` - Value profile for better coverage
- `-print_stats=1` - Print statistics
- `-detect_leaks=1` - Enable leak detection
- `-only_ascii=1` - Generate only ASCII inputs
- `-max_total_time=N` - Maximum total time in seconds

## Best Practices

1. **Start Simple**: Use default settings first, then optimize
2. **Monitor Performance**: Check CPU and memory usage
3. **Adjust Timeouts**: Based on target complexity
4. **Use Sanitizers**: Always compile with ASAN/UBSAN
5. **Persistent Mode**: Use AFL++ persistent mode when possible
6. **Corpus Management**: Minimize corpus periodically

## Troubleshooting

### Jobs Not Using Expected Arguments
1. Check bot logs: `docker logs pandafuzz-bot-1`
2. Verify bot configuration is loaded
3. Ensure bot was restarted after config changes

### Performance Issues
1. Reduce `-max_len` for LibFuzzer
2. Increase timeout values
3. Use fewer sanitizers during initial fuzzing
4. Enable persistent mode for AFL++

### Memory Issues
1. Set appropriate `memory_limit` in job config
2. Use `-m none` for AFL++ with ASAN
3. Monitor actual memory usage
4. Consider using `-rss_limit_mb` for LibFuzzer