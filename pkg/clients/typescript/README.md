# PandaFuzz TypeScript Client SDK

A comprehensive TypeScript client SDK for the PandaFuzz distributed fuzzing orchestration platform. This SDK provides full API access with built-in support for Server-Sent Events (SSE) for real-time monitoring.

## Features

- 🚀 **Full TypeScript Support** - Complete type safety with auto-generated types from OpenAPI specification
- 🔄 **Real-time Events** - Built-in Server-Sent Events (SSE) client for live updates
- 🔁 **Automatic Retry Logic** - Configurable retry mechanisms for robust error handling
- 🔒 **Authentication Support** - JWT Bearer tokens and API key authentication
- 📦 **Tree-shakeable** - Modular design allows importing only what you need
- 🧪 **Testing Ready** - Comprehensive test coverage and mock-friendly design
- 📚 **Well Documented** - Extensive examples and API documentation

## Installation

```bash
npm install @pandafuzz/client
```

## Quick Start

### Basic API Usage

```typescript
import { PandaFuzzClient } from '@pandafuzz/client';

// Initialize client
const client = new PandaFuzzClient({
  baseUrl: 'https://pandafuzz.example.com/api/v1',
  apiKey: 'your-api-key-here'
});

// Check system health
const isHealthy = await client.healthCheck();
console.log('System healthy:', isHealthy);

// List all bots
const bots = await client.bots.listBots();
console.log('Active bots:', bots.data);

// Create a new campaign
const campaign = await client.campaigns.createCampaign({
  name: 'My Fuzzing Campaign',
  description: 'Testing my application',
  jobTemplate: {
    fuzzerType: 'libfuzzer',
    targetBinary: '/path/to/binary',
    duration: 3600 // 1 hour
  }
});
```

### Real-time Event Monitoring

```typescript
import { PandaFuzzClient, EventTypes } from '@pandafuzz/client';

const client = new PandaFuzzClient({
  baseUrl: 'https://pandafuzz.example.com/api/v1',
  accessToken: 'your-jwt-token'
});

// Connect to event stream
client.connectEvents();

// Subscribe to specific events
client.onJobCompleted((jobData) => {
  console.log('Job completed:', jobData);
});

client.onCrashDiscovered((crashData) => {
  console.log('New crash found:', crashData);
  // Handle crash notification
});

client.onBotStatusChange((botData) => {
  console.log('Bot status changed:', botData);
});

// Subscribe to all events
client.onAnyEvent((event) => {
  console.log('Event received:', event);
});
```

## Authentication

### API Key Authentication

```typescript
const client = new PandaFuzzClient({
  baseUrl: 'https://pandafuzz.example.com/api/v1',
  apiKey: 'your-api-key'
});
```

### JWT Bearer Token Authentication

```typescript
const client = new PandaFuzzClient({
  baseUrl: 'https://pandafuzz.example.com/api/v1',
  accessToken: 'your-jwt-token'
});
```

## Core APIs

### Health API
```typescript
// System health check
await client.health.getHealth();

// Readiness check
await client.health.getReady();
```

### Bots API
```typescript
// List all bots
const bots = await client.bots.listBots();

// Get specific bot
const bot = await client.bots.getBot({ id: 'bot-123' });

// Update bot
await client.bots.updateBot({ 
  id: 'bot-123', 
  botUpdateRequest: { name: 'Updated Bot Name' }
});
```

### Jobs API
```typescript
// Create job
const job = await client.jobs.createJob({
  name: 'My Fuzzing Job',
  campaignId: 'campaign-123',
  fuzzerType: 'libfuzzer',
  targetBinary: '/path/to/target',
  duration: 3600
});

// List jobs
const jobs = await client.jobs.listJobs({
  status: 'running',
  limit: 50
});

// Get job logs
const logs = await client.jobs.getJobLogs({ 
  id: 'job-123',
  lines: 100 
});
```

### Campaigns API
```typescript
// Create campaign
const campaign = await client.campaigns.createCampaign({
  name: 'Security Test Campaign',
  description: 'Comprehensive security testing',
  jobTemplate: {
    fuzzerType: 'afl++',
    targetBinary: '/usr/bin/target',
    duration: 7200
  }
});

// Start campaign
await client.campaigns.startCampaign({ id: 'campaign-123' });

// Get campaign statistics
const stats = await client.campaigns.getCampaignStats({ id: 'campaign-123' });
```

## Error Handling

### Automatic Retry with Exponential Backoff

```typescript
// Use built-in retry logic
const result = await client.withRetry(
  () => client.jobs.getJob({ id: 'job-123' }),
  {
    maxAttempts: 5,
    initialDelay: 1000,
    maxDelay: 30000,
    backoffFactor: 2
  }
);
```

### Manual Error Handling

```typescript
try {
  const job = await client.jobs.createJob(jobRequest);
  console.log('Job created:', job);
} catch (error) {
  if (error.status === 400) {
    console.error('Invalid job request:', error.body);
  } else if (error.status === 429) {
    console.error('Rate limit exceeded');
  } else {
    console.error('Unexpected error:', error);
  }
}
```

## Event Types

The SDK provides strongly-typed event constants:

```typescript
import { EventTypes } from '@pandafuzz/client';

// Bot events
EventTypes.BOT_STATUS_CHANGED
EventTypes.BOT_REGISTERED
EventTypes.BOT_HEARTBEAT

// Job events
EventTypes.JOB_CREATED
EventTypes.JOB_COMPLETED
EventTypes.JOB_FAILED
EventTypes.JOB_PROGRESS

// Campaign events
EventTypes.CAMPAIGN_STARTED
EventTypes.CAMPAIGN_STOPPED
EventTypes.CAMPAIGN_COMPLETED

// Crash events
EventTypes.CRASH_DISCOVERED
EventTypes.CRASH_MINIMIZED
EventTypes.CRASH_REPRODUCED
```

## Advanced Configuration

### SSE Client Options

```typescript
const client = new PandaFuzzClient({
  baseUrl: 'https://pandafuzz.example.com/api/v1',
  sse: {
    reconnectInterval: 5000,
    maxReconnectAttempts: 10,
    debug: true
  }
});
```

## Examples

See the [examples directory](./examples/) for more comprehensive usage examples:

- [Basic CRUD Operations](./examples/basic-operations.ts)
- [Real-time Event Monitoring](./examples/event-monitoring.ts)
- [Campaign Management](./examples/campaign-management.ts)
- [Error Handling Patterns](./examples/error-handling.ts)
- [Authentication Flows](./examples/authentication.ts)

## Building

To build and compile the TypeScript sources to JavaScript use:
```bash
npm install
npm run build
```

## Publishing

First build the package then run:
```bash
npm publish
```

## Testing

The SDK is designed to be test-friendly:

```typescript
// Mock the client for testing
jest.mock('@pandafuzz/client', () => ({
  PandaFuzzClient: jest.fn().mockImplementation(() => ({
    health: {
      getHealth: jest.fn().mockResolvedValue({ status: 'healthy' })
    },
    bots: {
      listBots: jest.fn().mockResolvedValue({ data: [] })
    }
  }))
}));
```

## Browser Support

This SDK works in both Node.js and browser environments. Built with:

- ES6+ features with TypeScript compilation
- Fetch API for HTTP requests (with polyfill support)
- EventSource for Server-Sent Events
- Compatible with bundlers like Webpack, Rollup, and Vite

## License

This project is licensed under the Apache 2.0 License.

## Support

- 📖 [API Documentation](https://pandafuzz.ethpandaops.io/docs)
- 🐛 [Report Issues](https://github.com/ethpandaops/pandafuzz/issues)
- 💬 [Community Discussions](https://github.com/ethpandaops/pandafuzz/discussions)
- 📧 [Email Support](mailto:support@ethpandaops.io)