# Fix for Bot Storage/Binary Not Found Issue

## Problem
When you exec into the bot container, you don't find `/app/storage` or the binary at the expected location. This happens because:

1. The work directory is configured as `./work` (relative path)
2. The binary is stored in `/app/work/jobs/{job_id}/target_binary`
3. There's no `/app/storage` directory - that's not where binaries are stored

## Solution

### 1. **Check the Correct Location**
The binary should be at:
```bash
/app/work/jobs/{JOB_ID}/target_binary
```

Not at `/app/storage`.

### 2. **Debug Commands**
Run these commands inside the bot container:

```bash
# Check if work directory exists
ls -la /app/work/

# Find all job directories
find /app/work -name "job*" -type d

# Find all target_binary files
find /app/work -name "target_binary" -type f

# Check bot logs for job ID
grep "job_id" /app/logs/bot.log | tail -10
```

### 3. **Common Issues and Fixes**

#### Issue: Work directory doesn't exist
```bash
# Inside bot container
mkdir -p /app/work
chown -R $(id -u):$(id -g) /app/work
```

#### Issue: Volume not mounted properly
Check docker-compose.yml has:
```yaml
volumes:
  - bot-work:/app/work
```

Then restart:
```bash
docker-compose down
docker-compose up -d
```

#### Issue: Binary download failed
Check bot logs:
```bash
docker-compose logs bot | grep -E "(download|binary|failed)"
```

### 4. **Verify Binary Download Process**

The bot downloads binaries through these steps:
1. Creates job directory: `/app/work/jobs/{job_id}/`
2. Downloads from master: `GET /api/v1/jobs/{job_id}/binary/download`
3. Saves to: `/app/work/jobs/{job_id}/target_binary`
4. Sets permissions: `chmod 0755`

### 5. **Quick Test**

Create a test structure manually:
```bash
# Inside bot container
mkdir -p /app/work/jobs/test123
cd /app/work/jobs/test123

# Create a simple test binary
cat > test_fuzzer.cc << 'EOF'
#include <stdint.h>
#include <stddef.h>
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
  return 0;
}
EOF

# Compile it
clang++ -fsanitize=fuzzer test_fuzzer.cc -o target_binary
chmod +x target_binary

# Test it
./target_binary -help=1
```

### 6. **Check Job Status**

Use the master API to check job details:
```bash
# From host
curl http://localhost:8080/api/v1/jobs
```

### 7. **Enable Debug Logging**

The bot is already configured with debug logging. Check logs:
```bash
docker-compose logs -f bot | grep -E "(work_dir|binary|download)"
```

## Root Cause

The issue is likely one of:
1. **Wrong path** - Looking in `/app/storage` instead of `/app/work`
2. **Job not started** - Binary only downloads when job executes
3. **Download failed** - Check bot logs for download errors
4. **Volume issue** - Work directory not properly mounted

## Verification

Run the debug script:
```bash
./debug_bot_paths.sh
```

This will show:
- Actual work directory location
- Existing job directories
- Binary file locations
- Volume mount status