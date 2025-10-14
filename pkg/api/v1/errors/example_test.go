package errors_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	apierrors "github.com/ethpandaops/pandafuzz/pkg/api/v1/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Example demonstrates how to use the RFC 7807 error handling system
func ExampleProblem() {
	// Create a validation error
	fieldErrors := []apierrors.FieldError{
		{
			Field:   "fuzzer",
			Code:    apierrors.CodeInvalidEnum,
			Message: "Must be one of: afl++, libfuzzer, honggfuzz",
			Value:   "invalid-fuzzer",
		},
		{
			Field:   "timeout",
			Code:    apierrors.CodeInvalidRange,
			Message: "Must be between 1 and 3600 seconds",
			Value:   -1,
		},
	}

	problem := apierrors.ValidationError(fieldErrors)
	problem.AddRequestID("req-123")
	problem.AddTraceID("trace-456")

	fmt.Printf("Type: %s\n", problem.Type)
	fmt.Printf("Title: %s\n", problem.Title)
	fmt.Printf("Status: %d\n", problem.Status)
	fmt.Printf("Detail: %s\n", problem.Detail)

	// Output:
	// Type: https://pandafuzz.io/problems/validation-error
	// Title: Validation Error
	// Status: 400
	// Detail: One or more validation errors occurred
}

// Example of using custom PandaFuzzError types
func ExamplePandaFuzzError() {
	// Create a resource not found error
	err := apierrors.NewResourceNotFoundError("campaign", "550e8400-e29b-41d4-a716-446655440001")
	problem := err.ToProblem()

	fmt.Printf("Error: %s\n", err.Error())
	fmt.Printf("Problem Type: %s\n", problem.Type)
	fmt.Printf("Problem Status: %d\n", problem.Status)

	// Output:
	// Error: campaign with identifier '550e8400-e29b-41d4-a716-446655440001' not found
	// Problem Type: https://pandafuzz.io/problems/not-found
	// Problem Status: 404
}

// Example of using the error handler in an HTTP handler
func ExampleHandler_ErrorHandler() {
	// Create a buffer to capture logs
	var logBuffer bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&logBuffer)

	// Create handler with custom config
	config := &apierrors.HandlerConfig{
		Logger:           logger,
		DebugMode:        true,
		IncludeRequestID: true,
		CORSEnabled:      true,
		ServiceName:      "test-service",
	}
	handler := apierrors.NewHandler(config)

	// Create a test HTTP request
	req := httptest.NewRequest("GET", "/api/v1/campaigns/nonexistent", nil)
	req.Header.Set("X-Request-ID", "test-request-123")
	req.Header.Set("Accept", "application/json")

	// Create a response recorder
	w := httptest.NewRecorder()

	// Simulate an error
	err := apierrors.NewResourceNotFoundError("campaign", "nonexistent")
	handler.ErrorHandler(w, req, err)

	// Check the response
	fmt.Printf("Status Code: %d\n", w.Code)
	fmt.Printf("Content-Type: %s\n", w.Header().Get("Content-Type"))
	fmt.Printf("Response includes request ID: %t\n",
		bytes.Contains(w.Body.Bytes(), []byte("test-request-123")))

	// Output:
	// Status Code: 404
	// Content-Type: application/json; charset=utf-8
	// Response includes request ID: true
}

func TestProblemJSONSerialization(t *testing.T) {
	// Test that Problem details serialize correctly to JSON
	problem := apierrors.NewNotFoundProblem("campaign")
	problem.AddRequestID("req-123")
	problem.AddExtension("custom_field", "custom_value")

	data, err := problem.MarshalJSON()
	require.NoError(t, err)

	// Verify JSON contains all required fields
	assert.Contains(t, string(data), `"type":"https://pandafuzz.io/problems/not-found"`)
	assert.Contains(t, string(data), `"title":"Not Found"`)
	assert.Contains(t, string(data), `"status":404`)
	assert.Contains(t, string(data), `"instance":"req-123"`)
	assert.Contains(t, string(data), `"custom_field":"custom_value"`)
	assert.Contains(t, string(data), `"timestamp"`)
}

func TestValidationErrors(t *testing.T) {
	// Test validation error creation and conversion
	validationErr := apierrors.NewValidationErrors("Multiple fields are invalid", []apierrors.ValidationErrorDetail{
		{
			Field:   "name",
			Code:    apierrors.CodeRequired,
			Message: "Name is required",
		},
		{
			Field:   "timeout",
			Code:    apierrors.CodeInvalidRange,
			Message: "Timeout must be positive",
			Value:   -5,
		},
	})

	problem := validationErr.ToProblem()

	assert.Equal(t, apierrors.TypeValidationError, problem.Type)
	assert.Equal(t, http.StatusBadRequest, problem.Status)
	assert.Equal(t, "Multiple fields are invalid", problem.Detail)

	// Check that validation errors are included in extensions
	errors, exists := problem.Extensions["errors"]
	assert.True(t, exists)
	assert.IsType(t, []apierrors.FieldError{}, errors)
}

func TestRateLimitError(t *testing.T) {
	// Test rate limit error with retry-after
	rateLimitErr := apierrors.NewRateLimitExceededError(100, "1h", 3600)
	problem := rateLimitErr.ToProblem()

	assert.Equal(t, apierrors.TypeRateLimit, problem.Type)
	assert.Equal(t, http.StatusTooManyRequests, problem.Status)

	// Check extensions
	limit, exists := problem.Extensions["limit"]
	assert.True(t, exists)
	assert.Equal(t, 100, limit)

	window, exists := problem.Extensions["window"]
	assert.True(t, exists)
	assert.Equal(t, "1h", window)

	retryAfter, exists := problem.Extensions["retry_after"]
	assert.True(t, exists)
	assert.Equal(t, 3600, retryAfter)
}

func TestErrorWrapping(t *testing.T) {
	// Test error wrapping preserves PandaFuzzError interface
	originalErr := apierrors.NewResourceNotFoundError("job", "123")
	wrappedErr := apierrors.WrapError(originalErr, "failed to process request")

	// Verify it's still a PandaFuzzError
	pandaErr, ok := wrappedErr.(apierrors.PandaFuzzError)
	require.True(t, ok)

	// Verify the problem details are preserved
	problem := pandaErr.ToProblem()
	assert.Equal(t, apierrors.TypeNotFound, problem.Type)
	assert.Equal(t, http.StatusNotFound, problem.Status)

	// Verify context is added
	context, exists := problem.Extensions["context"]
	assert.True(t, exists)
	assert.Equal(t, "failed to process request", context)
}

func TestContentNegotiation(t *testing.T) {
	// Create handler
	handler := apierrors.NewHandler(apierrors.DefaultHandlerConfig())

	testCases := []struct {
		acceptHeader string
		expectedType string
	}{
		{"application/json", "application/json; charset=utf-8"},
		{"text/plain", "text/plain; charset=utf-8"},
		{"*/*", "application/json; charset=utf-8"},
		{"", "application/json; charset=utf-8"},
	}

	for _, tc := range testCases {
		t.Run(tc.acceptHeader, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tc.acceptHeader != "" {
				req.Header.Set("Accept", tc.acceptHeader)
			}

			w := httptest.NewRecorder()

			err := apierrors.NewResourceNotFoundError("test", "123")
			handler.ErrorHandler(w, req, err)

			assert.Equal(t, tc.expectedType, w.Header().Get("Content-Type"))
		})
	}
}

func TestErrorUtilities(t *testing.T) {
	// Test error utility functions
	retryableErr := apierrors.NewExternalServiceError("storage", "upload", 503, "Service unavailable", true)
	clientErr := apierrors.NewResourceNotFoundError("job", "123")
	serverErr := apierrors.NewTimeoutError("database query", "30s")

	assert.True(t, apierrors.IsRetryableError(retryableErr))
	assert.False(t, apierrors.IsRetryableError(clientErr))
	assert.True(t, apierrors.IsRetryableError(serverErr))

	assert.True(t, apierrors.IsClientError(clientErr))
	assert.False(t, apierrors.IsClientError(serverErr))

	assert.True(t, apierrors.IsServerError(serverErr))
	assert.False(t, apierrors.IsServerError(clientErr))

	assert.Equal(t, http.StatusNotFound, apierrors.GetStatusCode(clientErr))
	assert.Equal(t, http.StatusInternalServerError, apierrors.GetStatusCode(serverErr))
}
