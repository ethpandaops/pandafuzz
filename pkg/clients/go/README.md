# PandaFuzz Go Client SDK

A Go client library for the PandaFuzz distributed fuzzing orchestration platform.

## Features

- 🚀 **Generated Client**: Auto-generated from OpenAPI specification for complete API coverage  
- 🔐 **Authentication**: Support for both API keys and Bearer tokens
- ⚡ **Context Support**: Full context.Context integration for cancellation and timeouts
- 🛡️ **Type Safety**: Strongly-typed request/response structures
- 🔍 **Logging**: Structured logging with configurable levels
- 📊 **Connection Pooling**: Efficient HTTP connection management
- 🧪 **Testing**: Complete example programs

## Installation

```bash
go get github.com/ethpandaops/pandafuzz/clients/go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    pandafuzz "github.com/ethpandaops/pandafuzz/clients/go"
    "github.com/ethpandaops/pandafuzz/clients/go/generated"
)

func main() {
    // Create a new simple client
    client, err := pandafuzz.NewSimpleClient(
        "http://localhost:8080",
        pandafuzz.WithSimpleAPIKey("your-api-key"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx := context.Background()

    // Check system health
    healthResp, err := client.Health(ctx)
    if err != nil {
        log.Fatal(err)
    }
    defer healthResp.Body.Close()
    
    fmt.Printf("Health check status: %d\n", healthResp.StatusCode)

    // List all bots
    botsResp, err := client.ListBots(ctx, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer botsResp.Body.Close()
    
    fmt.Printf("List bots status: %d\n", botsResp.StatusCode)
}
```

## Client Types

### SimpleClient

The `SimpleClient` provides a convenient wrapper around the generated client with helper methods:

```go
client, err := pandafuzz.NewSimpleClient(
    "http://localhost:8080",
    pandafuzz.WithSimpleAPIKey("your-api-key"),
    pandafuzz.WithSimpleLogger(logger),
)
```

Available methods:
- `Health(ctx) (*http.Response, error)` - System health check
- `CreateBot(ctx, req) (*http.Response, error)` - Create a new bot
- `GetBot(ctx, botID) (*http.Response, error)` - Get bot by ID
- `ListBots(ctx, params) (*http.Response, error)` - List all bots
- `CreateCampaign(ctx, req) (*http.Response, error)` - Create a new campaign
- `GetCampaign(ctx, campaignID) (*http.Response, error)` - Get campaign by ID
- `ListCampaigns(ctx, params) (*http.Response, error)` - List all campaigns

### Generated Client

For full API access, use the generated client directly:

```go
client, err := pandafuzz.NewSimpleClient(baseURL, options...)
generatedClient := client.GetClient()

// Use any API endpoint
resp, err := generatedClient.ListJobs(ctx, &generated.ListJobsParams{
    Limit: &limit,
    Status: &status,
})
```

## Configuration

### Authentication

```go
// API Key Authentication
client, err := pandafuzz.NewSimpleClient(
    baseURL, 
    pandafuzz.WithSimpleAPIKey("your-api-key"),
)

// Bearer Token Authentication (JWT)
client, err := pandafuzz.NewSimpleClient(
    baseURL, 
    pandafuzz.WithSimpleBearerToken("your-jwt-token"),
)
```

### Custom HTTP Client

```go
httpClient := &http.Client{
    Timeout: 60 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns: 100,
    },
}

client, err := pandafuzz.NewSimpleClient(
    baseURL,
    pandafuzz.WithSimpleHTTPClient(httpClient),
)
```

### Logging

```go
logger := logrus.New()
logger.SetLevel(logrus.DebugLevel)

client, err := pandafuzz.NewSimpleClient(
    baseURL,
    pandafuzz.WithSimpleLogger(logger),
)
```

## Usage Examples

### Bot Management

```go
// Create a bot
createReq := generated.BotCreateRequest{
    ApiEndpoint: "http://bot.example.com:8080",
    Hostname:    "worker-01",
    Capabilities: []generated.BotCreateRequestCapabilities{
        "libfuzzer", "aflplusplus",
    },
}

createResp, err := client.CreateBot(ctx, createReq)
if err != nil {
    log.Fatal(err)
}
defer createResp.Body.Close()

// List bots with parameters
params := &generated.ListBotsParams{
    Limit: &[]int{10}[0],
    Status: &[]string{"active"}[0],
}

botsResp, err := client.ListBots(ctx, params)
if err != nil {
    log.Fatal(err)
}
defer botsResp.Body.Close()
```

### Campaign Management

```go
// Create a campaign
createReq := generated.CampaignCreateRequest{
    Name:        "security-testing",
    Description: "Security vulnerability testing campaign",
    Config: generated.CampaignCreateRequestConfig{
        MaxJobs:             20,
        JobTimeoutMinutes:   60,
        CorpusRetentionDays: 30,
    },
    Tags: map[string]string{
        "project": "security",
        "env":     "staging",
    },
}

campaignResp, err := client.CreateCampaign(ctx, createReq)
if err != nil {
    log.Fatal(err)
}
defer campaignResp.Body.Close()
```

### Error Handling

```go
resp, err := client.GetBot(ctx, "invalid-uuid")
if err != nil {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        log.Printf("Request timed out: %v", err)
    case errors.Is(err, context.Canceled):
        log.Printf("Request was canceled: %v", err)
    default:
        log.Printf("Request failed: %v", err)
    }
    return
}
defer resp.Body.Close()

if resp.StatusCode != http.StatusOK {
    body, _ := io.ReadAll(resp.Body)
    log.Printf("API error %d: %s", resp.StatusCode, string(body))
}
```

### Context Usage

```go
// With timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

resp, err := client.Health(ctx)

// With cancellation
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Cancel after 5 seconds
go func() {
    time.Sleep(5 * time.Second)
    cancel()
}()

resp, err := client.ListBots(ctx, nil)
```

## Generated Types

The client includes comprehensive generated types from the OpenAPI specification:

### Core Types
- `Bot`, `BotCreateRequest`, `BotUpdateRequest`
- `Campaign`, `CampaignCreateRequest`, `CampaignUpdateRequest` 
- `Job`, `JobCreateRequest`, `JobUpdateRequest`
- `HealthStatus`, `ErrorResponse`

### Parameter Types
- `ListBotsParams`, `GetBotParams`
- `ListCampaignsParams`, `GetCampaignParams`
- `ListJobsParams`, `GetJobParams`

### Enum Types
- `BotStatus`, `CampaignStatus`, `JobStatus`
- `FuzzerType`, `ArtifactType`

See the `generated/` package for the complete type definitions.

## Development

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Generating Client

To regenerate the client from the OpenAPI specification:

```bash
make generate
```

### Examples

Run the example programs:

```bash
cd examples/
go run simple_example.go
```

## Contributing

See the main [PandaFuzz repository](https://github.com/ethpandaops/pandafuzz) for contribution guidelines.

## License

Apache 2.0 License. See [LICENSE](https://github.com/ethpandaops/pandafuzz/blob/master/LICENSE) for details.