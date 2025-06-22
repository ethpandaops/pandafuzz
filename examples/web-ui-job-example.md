# Web UI Job Creation Example

## Quick Start - Using the Web UI

1. **Navigate to Jobs Page**
   - Open http://localhost:8080/jobs
   - Click the "New Job" button

2. **Fill in the Job Form**

### Example 1: Simple Test Job
```
Job Name: Test Echo Fuzzing
Fuzzer: afl++
Target Binary: /bin/echo
Target Arguments: @@
Timeout: 60 (seconds)
Memory Limit: 512 (MB)
```

### Example 2: Real Fuzzing Job (if you have a target)
```
Job Name: Fuzz JSON Parser
Fuzzer: libfuzzer
Target Binary: /app/targets/json_parser
Target Arguments: 
Timeout: 300 (seconds)
Memory Limit: 1024 (MB)
```

## Using Pre-built Vulnerable Target

First, build the example vulnerable program:

```bash
cd examples
./build-target.sh
```

Then copy the binaries to the bot containers:

```bash
# Copy to bot containers
docker cp vulnerable_afl pandafuzz-bot-1:/app/targets/
docker cp vulnerable_libfuzzer pandafuzz-bot-1:/app/targets/
docker cp vuln.dict pandafuzz-bot-1:/app/targets/
docker cp corpus pandafuzz-bot-1:/app/

docker cp vulnerable_afl pandafuzz-bot-2:/app/targets/
docker cp vulnerable_libfuzzer pandafuzz-bot-2:/app/targets/
docker cp vuln.dict pandafuzz-bot-2:/app/targets/
docker cp corpus pandafuzz-bot-2:/app/
```

## Create Jobs via API

Run the example script:

```bash
cd examples
./create-fuzzing-job.sh
```

This will create two jobs:
1. AFL++ job fuzzing the vulnerable program
2. LibFuzzer job fuzzing the same program

## What to Expect

1. **Job Status Changes**:
   - `pending` → `assigned` → `running` → `completed`

2. **During Fuzzing**:
   - Bots will execute the fuzzer
   - Coverage information will be collected
   - Crashes will be detected and reported

3. **Expected Crashes**:
   - Input starting with "CRASH" → Null pointer dereference
   - Input > 64 characters → Buffer overflow
   - Input with "BUG" at position 10-12 → Array out of bounds

## Monitoring Progress

1. **Jobs Page**: Shows job status and assigned bot
2. **Bots Page**: Shows which bots are busy
3. **Crashes Page**: Will show any crashes found
4. **Coverage Page**: Shows code coverage progress
5. **Corpus Page**: View/manage the test inputs

## Tips for Testing

1. **Quick Test**: Use `/bin/echo` as target - it won't crash but shows the system working
2. **Real Fuzzing**: Use the vulnerable program to see actual crashes
3. **Duration**: Start with short durations (60-300 seconds) for testing
4. **Memory**: 512MB-1GB is usually sufficient for testing

## Troubleshooting

If jobs stay in "pending" status:
- Check if bots are online: http://localhost:8080/bots
- Check bot capabilities match the fuzzer type
- Check bot logs: `docker-compose logs bot`

If jobs fail immediately:
- Check if target binary exists in bot container
- Check bot logs for error messages
- Verify target binary is executable