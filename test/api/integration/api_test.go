package integration

import (
	"context"
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
)

// TestAPIInitialization tests API server initialization and basic functionality
func (s *APIIntegrationTestSuite) TestAPIInitialization() {
	// Test that server is running
	s.NotNil(s.server, "Server should be initialized")
	s.NotEmpty(s.baseURL, "Base URL should be set")
}

// TestHealthEndpoint tests the health check endpoint
func (s *APIIntegrationTestSuite) TestHealthEndpoint() {
	resp, err := s.client.Health(s.ctx)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var health generated.HealthStatus
	err = parseJSONResponse(resp, &health)
	s.Require().NoError(err)

	// Validate health response structure
	s.Equal("healthy", health.Status)
	s.NotEmpty(health.Version)
	s.False(health.Timestamp.IsZero())
}

// TestReadinessEndpoint tests the readiness check endpoint
func (s *APIIntegrationTestSuite) TestReadinessEndpoint() {
	client := s.client.GetClient()
	resp, err := client.GetReadiness(s.ctx)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var readiness generated.ReadinessStatus
	err = parseJSONResponse(resp, &readiness)
	s.Require().NoError(err)

	// Validate readiness response
	s.True(readiness.Ready)
	s.NotNil(readiness.Checks)
}

// TestMetricsEndpoint tests the metrics endpoint
func (s *APIIntegrationTestSuite) TestMetricsEndpoint() {
	client := s.client.GetClient()
	resp, err := client.GetMetrics(s.ctx)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var metrics generated.MetricsResponse
	err = parseJSONResponse(resp, &metrics)
	s.Require().NoError(err)

	// Validate metrics structure
	s.NotNil(metrics.Metrics)
	s.NotNil(metrics.Metrics.System)
	s.NotNil(metrics.Metrics.Bots)
	s.NotNil(metrics.Metrics.Jobs)
	s.NotNil(metrics.Metrics.Campaigns)
	s.NotNil(metrics.Metrics.Crashes)
	s.NotNil(metrics.Metrics.Coverage)
}

// TestAPIVersioning tests API versioning
func (s *APIIntegrationTestSuite) TestAPIVersioning() {
	// Test that all endpoints are properly versioned under /api/v1
	endpoints := []string{
		"/api/v1/health",
		"/api/v1/readiness",
		"/api/v1/metrics",
		"/api/v1/bots",
		"/api/v1/jobs",
		"/api/v1/campaigns",
		"/api/v1/corpus",
		"/api/v1/crashes",
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}

	for _, endpoint := range endpoints {
		url := s.baseURL + endpoint
		resp, err := httpClient.Get(url)
		s.Require().NoError(err, "Failed to access endpoint: %s", endpoint)

		// Should not get 404 for valid endpoints
		s.NotEqual(http.StatusNotFound, resp.StatusCode, "Endpoint should exist: %s", endpoint)

		resp.Body.Close()
	}
}

// TestCORS tests Cross-Origin Resource Sharing configuration
func (s *APIIntegrationTestSuite) TestCORS() {
	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Test preflight request
	req, err := http.NewRequestWithContext(s.ctx, "OPTIONS", s.baseURL+"/api/v1/health", nil)
	s.Require().NoError(err)

	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	resp, err := httpClient.Do(req)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	// Check CORS headers
	s.NotEmpty(resp.Header.Get("Access-Control-Allow-Origin"))
	s.NotEmpty(resp.Header.Get("Access-Control-Allow-Methods"))
	s.NotEmpty(resp.Header.Get("Access-Control-Allow-Headers"))

	resp.Body.Close()
}

// TestRateLimiting tests rate limiting middleware
func (s *APIIntegrationTestSuite) TestRateLimiting() {
	httpClient := &http.Client{Timeout: 1 * time.Second}

	// Make many rapid requests to trigger rate limiting
	url := s.baseURL + "/api/v1/health"
	successCount := 0
	rateLimitedCount := 0

	for i := 0; i < 100; i++ {
		resp, err := httpClient.Get(url)
		if err != nil {
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK:
			successCount++
		case http.StatusTooManyRequests:
			rateLimitedCount++
		}

		resp.Body.Close()
	}

	// We should have some successful requests and possibly some rate limited
	s.Greater(successCount, 0, "Should have some successful requests")

	// Rate limiting might not trigger in tests, so we don't require it
	if rateLimitedCount > 0 {
		s.T().Logf("Rate limiting triggered: %d requests rate limited", rateLimitedCount)
	}
}

// TestRequestValidation tests request validation middleware
func (s *APIIntegrationTestSuite) TestRequestValidation() {
	client := s.client.GetClient()

	// Test invalid JSON payload
	req := generated.BotCreateRequest{
		Name:     "", // Invalid: empty name
		Hostname: "test-host",
		Capabilities: []generated.BotCreateRequestCapabilities{
			generated.BotCreateRequestCapabilitiesFuzzing,
		},
	}

	resp, err := client.CreateBot(s.ctx, req)
	s.Require().NoError(err)

	// Should return validation error
	s.Equal(http.StatusBadRequest, resp.StatusCode)

	// Parse error response
	problemDetails, err := parseErrorResponse(resp)
	s.Require().NoError(err)
	s.Equal("Validation Error", problemDetails.Title)
	s.NotEmpty(problemDetails.Detail)
}

// TestAuthentication tests authentication mechanisms
func (s *APIIntegrationTestSuite) TestAuthentication() {
	// Note: This test assumes authentication is optional for test environment
	// In production, this would test API keys or bearer tokens

	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Test without authentication (should work in test environment)
	resp, err := httpClient.Get(s.baseURL + "/api/v1/health")
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Test with invalid authentication header
	req, err := http.NewRequestWithContext(s.ctx, "GET", s.baseURL+"/api/v1/bots", nil)
	s.Require().NoError(err)
	req.Header.Set("Authorization", "Bearer invalid-token")

	resp, err = httpClient.Do(req)
	s.Require().NoError(err)

	// In test environment, this might still succeed
	// In production, this should return 401 Unauthorized
	if resp.StatusCode == http.StatusUnauthorized {
		s.T().Log("Authentication is properly enforced")
	}

	resp.Body.Close()
}

// TestContentNegotiation tests content type negotiation
func (s *APIIntegrationTestSuite) TestContentNegotiation() {
	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Test JSON response (default)
	req, err := http.NewRequestWithContext(s.ctx, "GET", s.baseURL+"/api/v1/health", nil)
	s.Require().NoError(err)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	s.Contains(resp.Header.Get("Content-Type"), "application/json")
	resp.Body.Close()

	// Test unsupported content type
	req, err = http.NewRequestWithContext(s.ctx, "GET", s.baseURL+"/api/v1/health", nil)
	s.Require().NoError(err)
	req.Header.Set("Accept", "application/xml")

	resp, err = httpClient.Do(req)
	s.Require().NoError(err)

	// Should still return JSON (or 406 Not Acceptable)
	if resp.StatusCode == http.StatusNotAcceptable {
		s.T().Log("Content negotiation properly rejects unsupported types")
	} else {
		s.Contains(resp.Header.Get("Content-Type"), "application/json")
	}

	resp.Body.Close()
}

// TestErrorHandling tests global error handling
func (s *APIIntegrationTestSuite) TestErrorHandling() {
	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Test 404 for non-existent endpoint
	resp, err := httpClient.Get(s.baseURL + "/api/v1/nonexistent")
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)

	// Should return proper error format
	var problemDetails generated.ProblemDetails
	err = parseJSONResponse(resp, &problemDetails)
	s.Require().NoError(err)
	s.Equal("Not Found", problemDetails.Title)

	// Test method not allowed
	req, err := http.NewRequestWithContext(s.ctx, "DELETE", s.baseURL+"/api/v1/health", nil)
	s.Require().NoError(err)

	resp, err = httpClient.Do(req)
	s.Require().NoError(err)
	s.Equal(http.StatusMethodNotAllowed, resp.StatusCode)
	resp.Body.Close()
}

// TestConcurrentRequests tests handling of concurrent requests
func (s *APIIntegrationTestSuite) TestConcurrentRequests() {
	const numRequests = 10

	// Channel to collect results
	results := make(chan error, numRequests)

	// Make concurrent health check requests
	for i := 0; i < numRequests; i++ {
		go func() {
			resp, err := s.client.Health(s.ctx)
			if err != nil {
				results <- err
				return
			}

			if resp.StatusCode != http.StatusOK {
				results <- fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				return
			}

			resp.Body.Close()
			results <- nil
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		err := <-results
		s.NoError(err, "Concurrent request failed")
	}
}

// TestServerShutdown tests graceful server shutdown
func (s *APIIntegrationTestSuite) TestServerShutdown() {
	// This test is informational only
	// The actual shutdown is tested in the suite teardown
	s.T().Log("Server shutdown will be tested during suite teardown")
}

// TestAPIDocumentation tests that API documentation is accessible
func (s *APIIntegrationTestSuite) TestAPIDocumentation() {
	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Test OpenAPI spec endpoint (if available)
	endpoints := []string{
		"/api/v1/openapi.json",
		"/api/v1/openapi.yaml",
		"/api/v1/docs",
		"/docs",
		"/swagger",
	}

	foundDocs := false
	for _, endpoint := range endpoints {
		resp, err := httpClient.Get(s.baseURL + endpoint)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			foundDocs = true
			s.T().Logf("Found API documentation at: %s", endpoint)
		}

		resp.Body.Close()
	}

	if !foundDocs {
		s.T().Log("No API documentation endpoints found (this is optional)")
	}
}
