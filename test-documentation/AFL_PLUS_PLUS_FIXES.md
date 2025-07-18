# AFL++ Crash Detection and Log Capture Fixes

## Overview
This document describes the fixes implemented for AFL++ crash detection and log capture issues.

## Issues Fixed

1. **Incorrect Crash Directory Path**
   - AFL++ stores crashes in `output/afl_output/crashes/` but the bot was looking in `output/crashes/`
   - Fixed in: `pkg/bot/agent.go` (lines 908-924)

2. **Missing Log Capture**
   - AFL++ output wasn't being captured to job logs
   - Fixed in: `pkg/fuzzer/aflplusplus.go` (lines 328-332)

3. **Delayed Crash Reporting**
   - Crashes were only reported after job completion
   - Fixed in: `pkg/fuzzer/aflplusplus.go` (added periodic crash checking)

## Code Changes

### pkg/bot/agent.go
```go
// AFL++ stores crashes in output/afl_output/crashes/
aflCrashesDir := filepath.Join(aflOutput, "afl_output", "crashes")
if stat, err := os.Stat(aflCrashesDir); err == nil && stat.IsDir() {
    dirsToCheck = append(dirsToCheck, aflCrashesDir)
}

// Also check the old location for backwards compatibility
oldAflCrashesDir := filepath.Join(aflOutput, "crashes")
if stat, err := os.Stat(oldAflCrashesDir); err == nil && stat.IsDir() {
    dirsToCheck = append(dirsToCheck, oldAflCrashesDir)
}
```

### pkg/fuzzer/aflplusplus.go
```go
// Added OutputWriter support
if afl.config.OutputWriter != nil {
    fmt.Fprintf(afl.config.OutputWriter, "[%s] %s\n", name, line)
}

// Added periodic crash checking
func (afl *AFLPlusPlus) checkForNewCrashes() {
    crashes, err := afl.GetCrashes()
    if err != nil {
        return
    }
    
    if len(crashes) > afl.lastReportedCrashes {
        newCrashes := crashes[afl.lastReportedCrashes:]
        for _, crash := range newCrashes {
            if afl.config.OnCrashFound != nil {
                afl.config.OnCrashFound(crash)
            }
        }
        afl.lastReportedCrashes = len(crashes)
    }
}
```

## Testing

### Manual Testing
1. Create a job with AFL++ fuzzer
2. Use fuzzer type "afl++" (not "aflplusplus")
3. Verify crashes are detected in `output/afl_output/crashes/`
4. Verify job logs are captured and available via API
5. Verify crashes are reported during execution, not just at completion

### Test Script Example
```bash
# Create job with AFL++
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "AFL++ Test",
    "type": "fuzzing",
    "fuzzer": "afl++",
    "target": "test_binary",
    "config": {
      "duration": 60,
      "timeout": 1000,
      "memory_limit": 512
    }
  }'

# Check for crashes
curl http://localhost:8080/api/v1/results/crashes

# Check job logs
curl http://localhost:8080/api/v1/jobs/{job_id}/logs
```

## Verification
To verify the fixes are in place:

1. Check that `pkg/bot/agent.go` contains the new crash directory path
2. Check that `pkg/fuzzer/aflplusplus.go` has OutputWriter support
3. Check that periodic crash checking is implemented
4. LibFuzzer functionality remains unchanged

## Backward Compatibility
- The fix maintains backward compatibility by checking both the new and old crash directory locations
- Existing AFL++ setups that use the old directory structure will continue to work