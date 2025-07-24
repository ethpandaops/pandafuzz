# Fuzzer Configuration Guide

## Overview

PandaFuzz supports multiple fuzzing engines, each with specific configuration options. This guide covers the configuration parameters for AFL++, LibFuzzer, and HongFuzz.

## General Fuzzer Configuration

All fuzzers share common configuration parameters through the `FuzzConfig` structure:

```yaml
fuzzer_config:
  job_id: "unique-job-id"
  target: "/path/to/target/binary"
  target_args: ["arg1", "arg2"]
  work_directory: "/tmp/fuzzing"
  duration: 3600s
  timeout: 10s
  memory_limit: 2GB
  environment:
    ASAN_OPTIONS: "detect_leaks=0"
    UBSAN_OPTIONS: "print_stacktrace=1"
  fuzzer_args: ["-v", "-x", "dict.txt"]
  fuzzer_type: "honggfuzz"  # or "afl++", "libfuzzer"
```

### Common Parameters

- `job_id`: Unique identifier for the fuzzing job
- `target`: Path to the target binary to fuzz
- `target_args`: Arguments to pass to the target binary
- `work_directory`: Working directory for fuzzer operations
- `duration`: Maximum fuzzing duration
- `timeout`: Timeout for individual test cases
- `memory_limit`: Memory limit for the fuzzing process
- `environment`: Environment variables for the fuzzing process
- `fuzzer_args`: Additional arguments specific to the fuzzer
- `fuzzer_type`: The fuzzing engine to use

## HongFuzz Configuration

HongFuzz is a hardware-accelerated fuzzer that supports persistent mode and various feedback mechanisms.

### Configuration Options

```yaml
honggfuzz_config:
  persistent_mode: true        # Enable persistent mode for LLVMFuzzerTestOneInput targets
  hardware_feedback: "edges"   # Hardware feedback type: none, instructions, branches, edges
  verify_crashes: true         # Enable crash verification to reduce false positives
  network_port: 0             # Port for network service fuzzing (0 = disabled)
  mutations_per_run: 256      # Number of mutations per run
  use_instrumentation: true   # Enable instrumentation for better coverage
  minimize_corpus: true       # Enable corpus minimization
  report_file: "honggfuzz.report"  # Path to detailed stats report file
  max_file_size: 1048576      # Maximum file size in bytes for generated inputs (1MB)
```

### Persistent Mode

Persistent mode significantly improves fuzzing performance by keeping the target process alive between test cases. HongFuzz automatically detects and uses persistent mode when:

- The target implements `LLVMFuzzerTestOneInput`
- The binary is compiled with persistent mode support
- `persistent_mode` is set to `true` in configuration

**Benefits:**
- 10-100x performance improvement
- Reduced process creation overhead
- Better coverage feedback accuracy

**Usage Example:**
```c
// Target implementation
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    // Process input data
    return 0;
}
```

### Hardware Feedback Options

HongFuzz leverages hardware performance counters for coverage feedback:

- **none**: No hardware feedback, rely on software instrumentation only
  - Use when: Hardware counters are unavailable or causing issues
  
- **instructions**: Count unique instruction addresses
  - Use when: Basic code coverage is sufficient
  - Lower overhead than branch coverage
  
- **branches**: Track branch execution (taken/not taken)
  - Use when: Need detailed control flow coverage
  - Good balance of overhead and precision
  
- **edges**: Track control flow edges (most precise)
  - Use when: Maximum coverage precision is needed
  - Default and recommended option
  - Highest overhead but best results

### Performance Tuning

#### Optimal Settings for Different Scenarios

**High-Performance Targets (Fast Execution)**
```yaml
honggfuzz_config:
  persistent_mode: true
  hardware_feedback: "edges"
  mutations_per_run: 512      # Increase for fast targets
  use_instrumentation: true
  minimize_corpus: false      # Disable for speed
```

**Network Services**
```yaml
honggfuzz_config:
  persistent_mode: false      # Usually not compatible with network services
  hardware_feedback: "branches"
  network_port: 8080         # Service port
  verify_crashes: true       # Important for network fuzzing
  mutations_per_run: 128     # Lower for network latency
```

**Resource-Constrained Environments**
```yaml
honggfuzz_config:
  persistent_mode: true
  hardware_feedback: "instructions"  # Lower overhead
  mutations_per_run: 64              # Reduce memory usage
  minimize_corpus: true              # Keep corpus small
  max_file_size: 262144              # 256KB limit
```

**Maximum Coverage (Research/Analysis)**
```yaml
honggfuzz_config:
  persistent_mode: true
  hardware_feedback: "edges"
  verify_crashes: true
  mutations_per_run: 256
  use_instrumentation: true
  minimize_corpus: true
  report_file: "detailed_coverage.report"
```

### Example Configurations

#### Basic Configuration
```yaml
# Simple HongFuzz setup for general purpose fuzzing
fuzzer_config:
  target: "/path/to/target"
  fuzzer_type: "honggfuzz"
  duration: 3600s
  
honggfuzz_config:
  persistent_mode: true
  hardware_feedback: "edges"
  verify_crashes: true
```

#### Advanced Configuration with Custom Dictionary
```yaml
# HongFuzz with dictionary and custom mutations
fuzzer_config:
  target: "/path/to/parser"
  fuzzer_type: "honggfuzz"
  fuzzer_args: ["-w", "wordlist.txt", "-n", "8"]  # 8 threads
  duration: 7200s
  
honggfuzz_config:
  persistent_mode: true
  hardware_feedback: "edges"
  mutations_per_run: 1024      # Aggressive mutations
  use_instrumentation: true
  minimize_corpus: true
  max_file_size: 5242880       # 5MB for complex inputs
```

#### Network Service Fuzzing
```yaml
# HongFuzz configuration for network service
fuzzer_config:
  target: "/path/to/network_service"
  fuzzer_type: "honggfuzz"
  target_args: ["--port", "8080", "--daemon"]
  timeout: 30s                 # Longer timeout for network
  
honggfuzz_config:
  persistent_mode: false       # Network services usually can't use persistent mode
  hardware_feedback: "branches"
  network_port: 8080
  verify_crashes: true         # Essential for network fuzzing
  mutations_per_run: 64        # Lower due to network overhead
```

#### Crash Verification Focus
```yaml
# Configuration optimized for reducing false positives
fuzzer_config:
  target: "/path/to/complex_app"
  fuzzer_type: "honggfuzz"
  memory_limit: 4GB
  
honggfuzz_config:
  persistent_mode: true
  hardware_feedback: "edges"
  verify_crashes: true         # Enable verification
  mutations_per_run: 128       # Balanced approach
  report_file: "verified_crashes.report"
  minimize_corpus: true        # Keep only effective inputs
```

## AFL++ Configuration

AFL++ configuration options (to be documented)

## LibFuzzer Configuration

LibFuzzer configuration options (to be documented)

## Best Practices

1. **Start with Default Settings**: Begin with basic configuration and tune based on results
2. **Monitor Performance**: Use `report_file` to track fuzzing effectiveness
3. **Adjust for Target Type**: Different targets benefit from different settings
4. **Verify Crashes**: Always enable `verify_crashes` for production fuzzing
5. **Corpus Management**: Enable `minimize_corpus` to maintain efficiency

## Troubleshooting

### Common Issues

1. **Persistent Mode Not Working**
   - Ensure target implements `LLVMFuzzerTestOneInput`
   - Check binary is compiled with appropriate flags
   - Verify `persistent_mode: true` in configuration

2. **Hardware Feedback Errors**
   - Try fallback: `hardware_feedback: "none"`
   - Check kernel permissions for hardware counters
   - Ensure HongFuzz has necessary privileges

3. **High False Positive Rate**
   - Enable `verify_crashes: true`
   - Increase `timeout` value
   - Check for non-deterministic behavior in target

4. **Poor Performance**
   - Enable persistent mode if possible
   - Reduce `mutations_per_run` if memory constrained
   - Use appropriate `hardware_feedback` level

## Integration with PandaFuzz

HongFuzz integrates seamlessly with PandaFuzz's distributed architecture:

```yaml
# Bot configuration with HongFuzz support
bot:
  capabilities: ["afl++", "libfuzzer", "honggfuzz"]
  
# Job submission with HongFuzz
{
  "fuzzer_type": "honggfuzz",
  "honggfuzz_config": {
    "persistent_mode": true,
    "hardware_feedback": "edges",
    "verify_crashes": true
  }
}
```

The PandaFuzz master automatically distributes HongFuzz jobs to capable bots and collects results including:
- Unique crashes (deduplicated by stack trace)
- Coverage statistics
- Performance metrics
- Corpus evolution