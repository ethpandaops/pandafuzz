# PandaFuzz TypeScript Client Examples

This directory contains comprehensive examples demonstrating how to use the PandaFuzz TypeScript client SDK in various scenarios.

## Available Examples

### 1. Basic Operations (`basic-operations.ts`)

Demonstrates fundamental CRUD operations and basic client usage:

- System health checks
- Bot management (list, create, update, delete)
- Campaign lifecycle management
- Job operations and monitoring
- Corpus management
- Crash analysis
- Analytics and metrics
- Error handling patterns
- Retry logic with exponential backoff

**Run the example:**
```bash
npx ts-node examples/basic-operations.ts
```

### 2. Event Monitoring (`event-monitoring.ts`)

Shows how to use Server-Sent Events (SSE) for real-time monitoring:

- Basic event monitoring for all event types
- Filtered event monitoring for specific campaigns/bots
- Custom SSE client usage
- Event-based dashboard implementation
- Statistics tracking and reporting
- Connection management and recovery

**Run the examples:**
```bash
# Basic event monitoring
npx ts-node examples/event-monitoring.ts basic

# Filtered event monitoring
npx ts-node examples/event-monitoring.ts filtered

# Custom SSE client
npx ts-node examples/event-monitoring.ts custom

# Comprehensive monitoring with statistics
npx ts-node examples/event-monitoring.ts comprehensive
```

### 3. Campaign Management (`campaign-management.ts`)

Advanced campaign orchestration and management:

- Security testing campaign creation
- Performance testing campaign setup
- Campaign monitoring and optimization
- Intelligent insights and recommendations
- Multi-campaign management
- Resource utilization tracking
- Comprehensive reporting

**Run the examples:**
```bash
# Complete campaign lifecycle
npx ts-node examples/campaign-management.ts lifecycle

# Multi-campaign management
npx ts-node examples/campaign-management.ts multi
```

### 4. Error Handling (`error-handling.ts`)

Robust error handling patterns and resilience strategies:

- Custom error classes and hierarchies
- Circuit breaker pattern implementation
- Automatic retry with exponential backoff
- Graceful degradation strategies
- Bulk operations with partial failures
- Connection recovery mechanisms
- Rate limiting handling

**Run the examples:**
```bash
# Comprehensive error handling
npx ts-node examples/error-handling.ts error-handling

# Rate limiting strategies
npx ts-node examples/error-handling.ts rate-limiting
```

### 5. Authentication (`authentication.ts`)

Various authentication methods and token management:

- Password-based authentication
- API key authentication
- OAuth2 flow implementation
- JWT token management with automatic refresh
- Multi-tenant authentication
- Custom authentication middleware
- Secure token storage strategies

**Run the examples:**
```bash
# Password authentication
npx ts-node examples/authentication.ts password

# API key authentication
npx ts-node examples/authentication.ts apikey

# OAuth2 flow
npx ts-node examples/authentication.ts oauth2

# Multi-tenant setup
npx ts-node examples/authentication.ts multi-tenant

# Custom authentication
npx ts-node examples/authentication.ts custom
```

## Prerequisites

Before running the examples, ensure you have:

1. **PandaFuzz Server Running**: The examples expect a PandaFuzz server running at `http://localhost:8080`
2. **Valid Authentication**: Configure appropriate API keys or credentials
3. **Node.js and TypeScript**: Install required dependencies

```bash
# Install dependencies
npm install

# Install TypeScript and ts-node globally (if not already installed)
npm install -g typescript ts-node

# Build the client library
npm run build
```

## Configuration

### Environment Variables

Set these environment variables for the examples:

```bash
export PANDAFUZZ_API_URL="http://localhost:8080/api/v1"
export PANDAFUZZ_API_KEY="your-api-key-here"
export PANDAFUZZ_USERNAME="your-username"
export PANDAFUZZ_PASSWORD="your-password"
```

### Configuration Files

Create configuration files for different environments:

```typescript
// config/development.ts
export const config = {
  baseUrl: 'http://localhost:8080/api/v1',
  apiKey: process.env.PANDAFUZZ_API_KEY,
  debug: true,
  timeout: 30000,
  retries: 3
};

// config/production.ts
export const config = {
  baseUrl: 'https://pandafuzz.yourdomain.com/api/v1',
  apiKey: process.env.PANDAFUZZ_API_KEY,
  debug: false,
  timeout: 10000,
  retries: 5
};
```

## Common Patterns

### Client Initialization

```typescript
import { PandaFuzzClient } from '@pandafuzz/client';

// Basic initialization
const client = new PandaFuzzClient({
  baseUrl: 'http://localhost:8080/api/v1',
  apiKey: 'your-api-key'
});

// With custom configuration
const client = new PandaFuzzClient({
  baseUrl: 'http://localhost:8080/api/v1',
  accessToken: 'jwt-token',
  sse: {
    reconnectInterval: 5000,
    maxReconnectAttempts: 10,
    debug: true
  }
});
```

### Error Handling

```typescript
try {
  const result = await client.campaigns.createCampaign(campaignData);
  console.log('Success:', result);
} catch (error) {
  if (error.status === 400) {
    console.error('Validation error:', error.body);
  } else if (error.status === 401) {
    console.error('Authentication failed');
  } else {
    console.error('Unexpected error:', error);
  }
}
```

### Event Monitoring

```typescript
// Connect to events
client.connectEvents();

// Subscribe to specific events
client.onCrashDiscovered((data) => {
  console.log('New crash:', data);
});

// Subscribe to all events
client.onAnyEvent((event) => {
  console.log('Event:', event.type, event.data);
});
```

### Retry Logic

```typescript
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

## Testing Examples

The examples include comprehensive error scenarios and edge cases. To test different failure modes:

1. **Network Failures**: Stop the PandaFuzz server while examples are running
2. **Authentication Failures**: Use invalid API keys or tokens
3. **Rate Limiting**: Make rapid API calls to trigger rate limits
4. **Validation Errors**: Send invalid data to trigger validation failures

## Integration Patterns

### With Express.js

```typescript
import express from 'express';
import { PandaFuzzClient } from '@pandafuzz/client';

const app = express();
const pandafuzz = new PandaFuzzClient({
  baseUrl: process.env.PANDAFUZZ_URL,
  apiKey: process.env.PANDAFUZZ_API_KEY
});

app.get('/campaigns', async (req, res) => {
  try {
    const campaigns = await pandafuzz.campaigns.listCampaigns();
    res.json(campaigns);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});
```

### With React

```typescript
import React, { useEffect, useState } from 'react';
import { PandaFuzzClient, EventTypes } from '@pandafuzz/client';

const DashboardComponent: React.FC = () => {
  const [client] = useState(() => new PandaFuzzClient({
    baseUrl: process.env.REACT_APP_PANDAFUZZ_URL,
    apiKey: process.env.REACT_APP_PANDAFUZZ_API_KEY
  }));
  
  const [events, setEvents] = useState<any[]>([]);

  useEffect(() => {
    client.connectEvents();
    
    client.onAnyEvent((event) => {
      setEvents(prev => [event, ...prev.slice(0, 99)]); // Keep last 100 events
    });
    
    return () => client.close();
  }, [client]);

  return (
    <div>
      <h1>PandaFuzz Dashboard</h1>
      <div>
        {events.map((event, index) => (
          <div key={index}>
            <strong>{event.type}</strong>: {JSON.stringify(event.data)}
          </div>
        ))}
      </div>
    </div>
  );
};
```

## Performance Tips

1. **Reuse Client Instances**: Create one client instance and reuse it
2. **Connection Pooling**: The client automatically manages HTTP connections
3. **Event Filtering**: Use event filters to reduce network traffic
4. **Batch Operations**: Use bulk operations when processing multiple items
5. **Circuit Breakers**: Implement circuit breakers for external service calls

## Troubleshooting

### Common Issues

1. **Connection Refused**: Ensure PandaFuzz server is running
2. **401 Unauthorized**: Check API key or token validity
3. **429 Rate Limited**: Implement proper retry logic with backoff
4. **SSE Connection Issues**: Check firewall and proxy settings
5. **Token Expiry**: Implement automatic token refresh

### Debug Mode

Enable debug mode for detailed logging:

```typescript
const client = new PandaFuzzClient({
  baseUrl: 'http://localhost:8080/api/v1',
  sse: { debug: true }
});
```

### Logging

Add logging to track API calls:

```typescript
const client = new PandaFuzzClient({
  baseUrl: 'http://localhost:8080/api/v1'
});

// Override fetch to add logging
const originalFetch = global.fetch;
global.fetch = async (...args) => {
  console.log('API Call:', args[0]);
  const response = await originalFetch(...args);
  console.log('API Response:', response.status, response.statusText);
  return response;
};
```

## Contributing

To add new examples:

1. Create a new `.ts` file in the `examples/` directory
2. Follow the existing pattern with CLI interface
3. Include comprehensive error handling
4. Add documentation comments
5. Update this README with the new example
6. Test with different failure scenarios

## Support

If you encounter issues with the examples:

1. Check the console output for detailed error messages
2. Verify your PandaFuzz server is running and accessible
3. Ensure you have valid authentication credentials
4. Review the troubleshooting section above
5. Open an issue on the GitHub repository with example output