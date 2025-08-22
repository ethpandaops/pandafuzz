# Server-Sent Events (SSE) Infrastructure

This package provides a comprehensive Server-Sent Events implementation for PandaFuzz's real-time event streaming capabilities.

## Overview

The SSE infrastructure replaces WebSocket implementation with a more efficient, HTTP-based real-time event system that provides:

- Real-time event broadcasting to multiple clients
- Topic-based subscriptions
- Advanced event filtering
- Automatic reconnection with event replay
- Rate limiting and buffer management
- Comprehensive metrics and monitoring
- Production-ready scalability

## Architecture

### Core Components

1. **Manager** (`manager.go`) - Central orchestrator for SSE connections and event broadcasting
2. **Client** (`client.go`) - Individual SSE client connection handler
3. **Events** (`events.go`) - Structured event types for all PandaFuzz resources
4. **Filters** (`filters.go`) - Advanced event filtering system
5. **Types** (`types.go`) - Supporting types, metrics, and error definitions

## Quick Start

```go
package main

import (
    "context"
    "net/http"
    
    "github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
    "github.com/sirupsen/logrus"
)

func main() {
    logger := logrus.New()
    
    // Create and start SSE manager
    config := sse.DefaultConfig()
    manager := sse.NewManager(config, logger)
    
    ctx := context.Background()
    manager.Start(ctx)
    defer manager.Stop()
    
    // Set up SSE endpoint
    http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
        clientID := "client_" + generateID()
        client := sse.NewClient(clientID, w, r, sse.DefaultClientConfig(), logger)
        
        manager.Register(client)
        defer manager.Unregister(client)
        
        client.ServeSSE() // Blocks until connection closes
    })
    
    // Broadcast events
    botEvent := sse.CreateBotRegisteredEvent(botID, "example-bot", []string{"fuzzing"})
    manager.Broadcast(botEvent)
    
    http.ListenAndServe(":8080", nil)
}
```

## Event Types

The SSE system supports structured events for all PandaFuzz resources:

### Bot Events
- `bot.registered` - Bot agent registration
- `bot.unregistered` - Bot agent disconnection
- `bot.heartbeat` - Periodic bot status updates
- `bot.status_changed` - Bot status transitions
- `bot.job_assigned` - Job assignment to bot
- `bot.job_completed` - Job completion by bot
- `bot.error` - Bot error conditions

### Job Events
- `job.created` - New job creation
- `job.started` - Job execution start
- `job.progress` - Job progress updates
- `job.completed` - Job completion
- `job.failed` - Job failure
- `job.cancelled` - Job cancellation
- `job.timeout` - Job timeout
- `job.restarted` - Job restart

### Campaign Events
- `campaign.created` - Campaign creation
- `campaign.started` - Campaign start
- `campaign.paused` - Campaign pause
- `campaign.resumed` - Campaign resume
- `campaign.completed` - Campaign completion
- `campaign.failed` - Campaign failure
- `campaign.deleted` - Campaign deletion
- `campaign.updated` - Campaign updates

### Crash Events
- `crash.detected` - New crash discovery
- `crash.analyzed` - Crash analysis completion
- `crash.duplicate` - Duplicate crash detection
- `crash.minimized` - Crash minimization
- `crash.reproduced` - Crash reproduction
- `crash.classified` - Crash classification

### Corpus Events
- `corpus.sync` - Corpus synchronization
- `corpus.updated` - Corpus updates
- `corpus.quarantined` - Corpus quarantine
- `corpus.restored` - Corpus restoration
- `corpus.deleted` - Corpus deletion
- `corpus.created` - Corpus creation

### System Events
- `system.heartbeat` - System health checks
- `system.alert` - System alerts
- `system.maintenance` - Maintenance notifications
- `client.connected` - Client connections
- `client.disconnected` - Client disconnections
- `client.reconnected` - Client reconnections

## Event Filtering

The SSE system provides powerful filtering capabilities:

### Type Filtering
Filter events by specific types:
```
/events?types=job.started,job.completed,crash.detected
```

### Resource Filtering
Filter events by specific resources:
```
/events?bot_id=550e8400-e29b-41d4-a716-446655440000
/events?campaign_id=fedcba98-7654-3210-fedc-ba9876543210
/events?job_id=01234567-89ab-cdef-0123-456789abcdef
```

### Pattern Filtering
Filter events using regex patterns:
```
/events?pattern=^(job|crash)\.
```

### Severity Filtering
Filter events by minimum severity level:
```
/events?min_severity=warning
```

### Compound Filtering
Combine multiple filters:
```
/events?types=job.progress&campaign_id=550e8400-e29b-41d4-a716-446655440000&min_severity=info
```

## Client Usage

### JavaScript/Browser
```javascript
const eventSource = new EventSource('/events?types=job.progress,crash.detected');

eventSource.addEventListener('job.progress', function(event) {
    const data = JSON.parse(event.data);
    console.log('Job progress:', data.progress + '%');
    updateProgressBar(data.job_id, data.progress);
});

eventSource.addEventListener('crash.detected', function(event) {
    const data = JSON.parse(event.data);
    console.log('Crash detected:', data.crash_type);
    showCrashAlert(data);
});

eventSource.onerror = function(event) {
    console.error('SSE connection error:', event);
};
```

### cURL
```bash
# Subscribe to all events
curl -N -H "Accept: text/event-stream" http://localhost:8080/events

# Subscribe to specific event types
curl -N -H "Accept: text/event-stream" "http://localhost:8080/events?types=job.started,job.completed"

# Subscribe to events for specific campaign
curl -N -H "Accept: text/event-stream" "http://localhost:8080/events?campaign_id=550e8400-e29b-41d4-a716-446655440000"
```

## Advanced Features

### Automatic Reconnection
Clients can reconnect and receive missed events using the `Last-Event-ID` header:

```javascript
const eventSource = new EventSource('/events');
// Browser automatically handles reconnection with Last-Event-ID
```

### Topic Subscriptions
Subscribe clients to specific topics for targeted event delivery:

```go
// Subscribe client to bot events topic
manager.Subscribe("client_123", "bot-events")

// Broadcast only to subscribers of bot-events topic
manager.BroadcastToTopic("bot-events", botEvent)
```

### Rate Limiting
Configure per-client rate limiting:

```go
clientConfig := sse.ClientConfig{
    MaxEventsPerSec: 10,    // Max 10 events per second
    BurstSize:      20,     // Allow bursts up to 20 events
    BufferSize:     100,    // Client event buffer size
}
```

### Metrics and Monitoring
Access comprehensive metrics:

```go
metrics := manager.GetMetrics()
fmt.Printf("Active clients: %d\n", metrics.GetActiveClients())
fmt.Printf("Events sent: %d\n", metrics.GetEventsSent())
fmt.Printf("Events per second: %.2f\n", metrics.GetEventsPerSecond())

// Get detailed statistics
stats := manager.GetManagerStats()
```

## Configuration

### Manager Configuration
```go
config := sse.Config{
    MaxClients:          1000,                // Maximum concurrent clients
    ClientTimeout:       5 * time.Minute,    // Client connection timeout
    WriteTimeout:        10 * time.Second,   // Write operation timeout
    HeartbeatInterval:   30 * time.Second,   // Heartbeat frequency
    EventBufferSize:     1000,               // Manager event buffer
    ClientBufferSize:    100,                // Per-client event buffer
    BroadcastBufferSize: 10000,              // Broadcast queue size
    MaxEventsPerSecond:  100,                // Global rate limit
    BurstSize:           200,                // Global burst size
    CleanupInterval:     1 * time.Minute,    // Cleanup frequency
    MaxEventHistory:     10000,              // Event history size
}
```

### Client Configuration
```go
clientConfig := sse.ClientConfig{
    BufferSize:        100,               // Client event buffer
    WriteTimeout:      10 * time.Second, // Write timeout
    MaxEventsPerSec:   10,               // Client rate limit
    BurstSize:         20,               // Client burst size
    EnableCompression: false,            // Enable gzip compression
}
```

## Error Handling

The SSE system provides comprehensive error handling:

```go
// Manager errors
if err := manager.Start(ctx); err != nil {
    switch err {
    case sse.ErrManagerAlreadyRunning:
        // Handle already running
    default:
        // Handle other errors
    }
}

// Client errors
if err := client.SendEvent(event); err != nil {
    switch err {
    case sse.ErrClientClosed:
        // Client disconnected
    case sse.ErrRateLimited:
        // Rate limit exceeded
    case sse.ErrBufferFull:
        // Client buffer full
    }
}
```

## Testing

Run the comprehensive test suite:

```bash
# Run all tests
go test ./pkg/api/v1/sse/

# Run tests with race detection
go test -race ./pkg/api/v1/sse/

# Run benchmarks
go test -bench=. ./pkg/api/v1/sse/

# Run specific test
go test -run TestManager_Lifecycle ./pkg/api/v1/sse/
```

## Performance Considerations

- **Memory Usage**: Each client uses ~1KB of memory plus buffer size
- **CPU Usage**: Minimal overhead with efficient channel operations
- **Network**: Events are JSON-encoded, consider compression for large payloads
- **Scaling**: Tested with 1000+ concurrent clients per instance
- **Latency**: Sub-millisecond event delivery under normal conditions

## Production Deployment

### Load Balancing
For multiple instances, use sticky sessions:
```nginx
upstream sse_backend {
    ip_hash;  # Sticky sessions
    server sse1.example.com:8080;
    server sse2.example.com:8080;
}

location /events {
    proxy_pass http://sse_backend;
    proxy_set_header Connection '';
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_cache off;
}
```

### Monitoring
Monitor key metrics:
- Active client count
- Events per second
- Client connection/disconnection rate
- Event buffer utilization
- Error rates

### Security
- Use HTTPS in production
- Implement authentication/authorization
- Rate limit based on client identity
- Monitor for abuse patterns

## Integration with PandaFuzz

The SSE infrastructure integrates seamlessly with PandaFuzz's existing architecture:

1. **API v1 Integration**: Implements the `/events` endpoint from the OpenAPI specification
2. **Event Generation**: Master and bot components generate events automatically
3. **Web UI**: React frontend consumes real-time events for live updates
4. **Monitoring**: Provides real-time visibility into system state

## Migration from WebSockets

The SSE system provides several advantages over WebSockets:

1. **Simpler Protocol**: Standard HTTP-based, easier to debug and monitor
2. **Better Caching**: Works with HTTP caches and CDNs
3. **Automatic Reconnection**: Built-in browser support for reconnection
4. **Firewall Friendly**: Uses standard HTTP, fewer network issues
5. **Event Replay**: Automatic handling of missed events on reconnection

## Examples

See `example_usage.go` for complete working examples including:
- Basic SSE server setup
- Event generation and broadcasting
- Client connection handling
- Filter usage demonstrations
- JavaScript client examples