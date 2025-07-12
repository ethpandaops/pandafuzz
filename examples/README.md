# PandaFuzz Examples

This directory contains examples for getting started with PandaFuzz.

## Quick Start

### 1. Create a Working Fuzzing Job

The main example script demonstrates a complete fuzzing workflow:

```bash
./create-fuzzing-job.sh
```

This script:
- Creates a vulnerable C program with multiple crash triggers
- Compiles it with LibFuzzer using Docker (no local setup required)
- Uploads the binary and creates a fuzzing job
- Monitors the job and checks for crashes

**Expected output:**
- Job created successfully with status "running" or "completed"
- Crashes found within seconds (check bot logs if web UI doesn't show them)
- Crash inputs like "BOOM" or "FUZZ" trigger the programmed vulnerabilities

### 2. Monitor Your Job

After running the script, you can:

```bash
# View in web UI
open http://localhost:8080/jobs

# Check job status via API
curl http://localhost:8080/api/v1/jobs/{job_id} | jq '.'

# Check bot logs for crash detection
docker logs pandafuzz-bot-1 | grep "libfuzzer_crash"

# Find crash files directly
docker exec pandafuzz-bot-1 find /app/work -name "*crash*" -type f
```

## Files in This Directory

- `create-fuzzing-job.sh` - Main example script that creates a complete fuzzing job
- `web-ui-job-example.md` - Guide for creating jobs through the web UI
- `FUZZER_CONFIGURATION.md` - Advanced configuration guide for fuzzer-specific settings
- `README.md` - This file

## How It Works

1. **Target Creation**: The script creates a simple C program with deliberate vulnerabilities:
   - Crashes on input "BOOM" (abort)
   - Crashes on input "FUZZ" (trap)
   - Buffer overflow on inputs > 100 bytes

2. **Compilation**: Uses Docker to compile with LibFuzzer, ensuring consistency across environments

3. **Job Creation**: Uses the `/api/v1/jobs/upload` endpoint to upload the binary and create a job

4. **Fuzzing**: LibFuzzer quickly finds the crashes using the provided seed corpus

## Troubleshooting

### Job stays in "pending" state
- Check if bots are running: `docker ps | grep bot`
- View bot logs: `docker logs pandafuzz-bot-1`

### Crashes not showing in web UI
- This is a known issue with the crash reporting API
- Verify crashes are found by checking bot logs or work directory
- Look for "libfuzzer_crash" entries in bot logs

### Binary validation errors
- Ensure the binary is compiled with `-fsanitize=fuzzer`
- Check that the binary exports `LLVMFuzzerTestOneInput`

## Next Steps

1. **Modify the target**: Edit the vulnerable program in the script to test your own code
2. **Adjust fuzzing parameters**: Change duration, memory limits, or timeout values
3. **Use different fuzzers**: Modify the script to use AFL++ or other supported fuzzers
4. **Scale up**: Run multiple bots for distributed fuzzing

## Additional Resources

- [PandaFuzz Documentation](../docs/)
- [LibFuzzer Documentation](https://llvm.org/docs/LibFuzzer.html)
- [AFL++ Documentation](https://github.com/AFLplusplus/AFLplusplus)