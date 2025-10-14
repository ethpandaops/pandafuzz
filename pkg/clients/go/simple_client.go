package pandafuzz

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/clients/go/generated"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// SimpleClient provides a simple interface to the PandaFuzz API using the generated client
type SimpleClient struct {
	client     *generated.Client
	baseURL    string
	httpClient *http.Client
	logger     logrus.FieldLogger
}

// SimpleClientOption configures the SimpleClient
type SimpleClientOption func(*SimpleClient)

// WithSimpleAPIKey sets the API key for authentication
func WithSimpleAPIKey(apiKey string) SimpleClientOption {
	return func(c *SimpleClient) {
		// We'll add authentication in request editors
	}
}

// WithSimpleBearerToken sets the Bearer token for authentication
func WithSimpleBearerToken(token string) SimpleClientOption {
	return func(c *SimpleClient) {
		// We'll add authentication in request editors
	}
}

// WithSimpleLogger sets a custom logger
func WithSimpleLogger(logger logrus.FieldLogger) SimpleClientOption {
	return func(c *SimpleClient) {
		c.logger = logger
	}
}

// WithSimpleHTTPClient sets a custom HTTP client
func WithSimpleHTTPClient(httpClient *http.Client) SimpleClientOption {
	return func(c *SimpleClient) {
		c.httpClient = httpClient
	}
}

// NewSimpleClient creates a new simple PandaFuzz API client
func NewSimpleClient(baseURL string, options ...SimpleClientOption) (*SimpleClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}

	// Default configuration
	client := &SimpleClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logrus.WithField("component", "pandafuzz-simple-client"),
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	// Create the generated client
	generatedClient, err := generated.NewClient(baseURL, generated.WithHTTPClient(client.httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create generated client: %w", err)
	}

	client.client = generatedClient

	return client, nil
}

// Close releases any resources held by the client
func (c *SimpleClient) Close() error {
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}

// GetClient returns the underlying generated client for direct access
func (c *SimpleClient) GetClient() *generated.Client {
	return c.client
}

// Health performs a health check on the PandaFuzz system
func (c *SimpleClient) Health(ctx context.Context) (*http.Response, error) {
	return c.client.GetHealth(ctx)
}

// CreateBot creates a new bot
func (c *SimpleClient) CreateBot(ctx context.Context, req generated.BotCreateRequest) (*http.Response, error) {
	return c.client.CreateBot(ctx, req)
}

// GetBot retrieves a bot by ID
func (c *SimpleClient) GetBot(ctx context.Context, botID string) (*http.Response, error) {
	// Parse string as UUID
	botUUID, err := uuid.Parse(botID)
	if err != nil {
		return nil, fmt.Errorf("invalid bot ID format: %w", err)
	}
	return c.client.GetBot(ctx, botUUID, &generated.GetBotParams{})
}

// ListBots retrieves all bots
func (c *SimpleClient) ListBots(ctx context.Context, params *generated.ListBotsParams) (*http.Response, error) {
	if params == nil {
		params = &generated.ListBotsParams{}
	}
	return c.client.ListBots(ctx, params)
}

// CreateCampaign creates a new campaign
func (c *SimpleClient) CreateCampaign(ctx context.Context, req generated.CampaignCreateRequest) (*http.Response, error) {
	return c.client.CreateCampaign(ctx, req)
}

// GetCampaign retrieves a campaign by ID
func (c *SimpleClient) GetCampaign(ctx context.Context, campaignID string) (*http.Response, error) {
	// Parse string as UUID
	campaignUUID, err := uuid.Parse(campaignID)
	if err != nil {
		return nil, fmt.Errorf("invalid campaign ID format: %w", err)
	}
	return c.client.GetCampaign(ctx, campaignUUID, &generated.GetCampaignParams{})
}

// ListCampaigns retrieves all campaigns
func (c *SimpleClient) ListCampaigns(ctx context.Context, params *generated.ListCampaignsParams) (*http.Response, error) {
	if params == nil {
		params = &generated.ListCampaignsParams{}
	}
	return c.client.ListCampaigns(ctx, params)
}
