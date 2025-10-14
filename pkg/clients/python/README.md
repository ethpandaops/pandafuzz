# PandaFuzz Python Client SDK

A comprehensive Python client library for the PandaFuzz distributed fuzzing orchestration platform. This SDK provides both synchronous and asynchronous interfaces for all PandaFuzz API operations, along with real-time event streaming support.

## Features

- **Complete API Coverage**: Full support for all PandaFuzz API endpoints
- **Async/Await Support**: Native asyncio support for high-performance applications
- **Real-time Events**: Server-sent events (SSE) client with automatic reconnection
- **Type Safety**: Full type hints and Pydantic model validation
- **Retry Logic**: Automatic retry with exponential backoff for resilient operations
- **Connection Pooling**: Efficient HTTP connection management
- **Rich Documentation**: Comprehensive examples and API documentation

## Installation

### From PyPI (Recommended)

```bash
pip install pandafuzz
```

### From Source

```bash
git clone https://github.com/ethpandaops/pandafuzz.git
cd pandafuzz/pkg/clients/python
pip install -e .
```

### Development Installation

```bash
git clone https://github.com/ethpandaops/pandafuzz.git
cd pandafuzz/pkg/clients/python
pip install -e ".[dev]"
```

## Quick Start

### Basic Usage

```python
import pandafuzz

# Initialize client
client = pandafuzz.PandaFuzzClient(
    host="http://localhost:8080",
    api_key="your-api-key"  # or bearer_token="your-jwt-token"
)

# Check system health
if client.quick_health_check():
    print("PandaFuzz is healthy!")

# Create a simple fuzzing job
job = client.create_simple_job(
    name="my-fuzzing-job",
    fuzzer_type="libfuzzer",
    target_binary="/path/to/target",
    duration_minutes=60
)

print(f"Created job: {job['id']}")

# Wait for job completion
final_job = client.wait_for_job_completion(job['id'])
print(f"Job completed with status: {final_job['status']}")
```

### Async Usage

```python
import asyncio
import pandafuzz

async def main():
    # Use async client for high-performance operations
    async with pandafuzz.AsyncPandaFuzzClient(
        host="http://localhost:8080",
        api_key="your-api-key"
    ) as client:
        
        # Check health asynchronously
        health = await client.get_health()
        print(f"System status: {health['status']}")
        
        # Create multiple jobs concurrently
        jobs = await asyncio.gather(*[
            client.create_job({
                "name": f"job-{i}",
                "fuzzer_type": "afl++",
                "target_binary": "/path/to/target",
                "timeout_seconds": 3600
            })
            for i in range(5)
        ])
        
        print(f"Created {len(jobs)} jobs")

asyncio.run(main())
```

### Real-time Event Streaming

```python
import pandafuzz

client = pandafuzz.PandaFuzzClient(host="http://localhost:8080", api_key="your-api-key")

# Stream all events
for event in client.stream_all_events():
    print(f"Event: {event.event_type} - {event.data}")

# Stream specific event types
for event in client.stream_job_events():
    if event.event_type == pandafuzz.PandaFuzzEventTypes.JOB_COMPLETED:
        print(f"Job {event.data['job_id']} completed!")

# Async event streaming
async def stream_events():
    async for event in client.async_client.stream_events(['crash.discovered']):
        print(f"New crash discovered: {event.data}")

asyncio.run(stream_events())
```

## Authentication

The client supports multiple authentication methods:

### API Key Authentication

```python
client = pandafuzz.PandaFuzzClient(
    host="http://localhost:8080",
    api_key="your-api-key"
)
```

### Bearer Token Authentication

```python
client = pandafuzz.PandaFuzzClient(
    host="http://localhost:8080",
    bearer_token="your-jwt-token"
)
```

### Environment Variables

Set environment variables to avoid hardcoding credentials:

```bash
export PANDAFUZZ_HOST="http://localhost:8080"
export PANDAFUZZ_API_KEY="your-api-key"
# or
export PANDAFUZZ_BEARER_TOKEN="your-jwt-token"
```

```python
import os
import pandafuzz

client = pandafuzz.PandaFuzzClient(
    host=os.getenv("PANDAFUZZ_HOST"),
    api_key=os.getenv("PANDAFUZZ_API_KEY"),
    bearer_token=os.getenv("PANDAFUZZ_BEARER_TOKEN")
)
```

## Core Operations

### Bot Management

```python
# List all bots
bots = client.bots.list_bots()
print(f"Active bots: {len(bots.items)}")

# Register a new bot
bot = client.bots.create_bot({
    "name": "fuzzer-bot-1",
    "capabilities": ["libfuzzer", "afl++"],
    "max_concurrent_jobs": 4
})

# Update bot configuration
client.bots.update_bot(bot.id, {
    "max_concurrent_jobs": 8
})

# Get bot details
bot_details = client.bots.get_bot(bot.id)
print(f"Bot status: {bot_details.status}")
```

### Job Management

```python
# Create a fuzzing job
job = client.jobs.create_job({
    "name": "security-audit-job",
    "fuzzer_type": "libfuzzer",
    "target_binary": "/usr/bin/target",
    "arguments": ["-arg1", "value1"],
    "timeout_seconds": 7200,
    "corpus_path": "/path/to/initial/corpus"
})

# Monitor job progress
for progress in client.monitor_campaign_progress(job.campaign_id):
    print(f"Progress: {progress['completion_percentage']}%")
    if progress['status'] in ['completed', 'failed']:
        break

# Get job logs
logs = client.jobs.get_job_logs(job.id, limit=100)
for log_entry in logs.logs:
    print(f"[{log_entry.timestamp}] {log_entry.level}: {log_entry.message}")

# Cancel a job
client.jobs.cancel_job(job.id)
```

### Campaign Management

```python
# Create a fuzzing campaign
campaign = client.campaigns.create_campaign({
    "name": "comprehensive-security-audit",
    "job_template": {
        "fuzzer_type": "afl++",
        "target_binary": "/usr/bin/target",
        "timeout_seconds": 86400  # 24 hours
    },
    "max_parallel_jobs": 10,
    "timeout_seconds": 604800  # 1 week
})

# Start the campaign
client.campaigns.start_campaign(campaign.id)

# Monitor campaign progress
async def monitor_campaign():
    async for stats in client.async_monitor_campaign_progress(campaign.id):
        print(f"Jobs: {stats['active_jobs']}/{stats['total_jobs']}")
        print(f"Crashes found: {stats['total_crashes']}")
        print(f"Coverage: {stats['coverage_percentage']}%")

asyncio.run(monitor_campaign())

# Stop campaign
client.campaigns.stop_campaign(campaign.id, reason="Sufficient coverage achieved")
```

## API Reference

### High-Level Clients

- `PandaFuzzClient` - Main synchronous client
- `AsyncPandaFuzzClient` - Asynchronous client  
- `SSEClient` - Server-sent events client

### Generated API Clients

- `AnalyticsApi` - System analytics and metrics
- `BatchApi` - Batch operations
- `BotsApi` - Bot management
- `CampaignsApi` - Campaign orchestration
- `CorpusApi` - Corpus management
- `CrashesApi` - Crash analysis
- `EventsApi` - Event streaming
- `HealthApi` - System health
- `JobsApi` - Job management

## Examples

See the `examples/` directory for complete working examples.

## License

This project is licensed under the Apache 2.0 License.

## Support

- GitHub Issues: https://github.com/ethpandaops/pandafuzz/issues
- Email: support@ethpandaops.io