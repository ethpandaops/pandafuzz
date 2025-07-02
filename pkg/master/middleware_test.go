package master

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeoutMiddleware(t *testing.T) {
	// Create a test server
	logger := logrus.New()
	config := &common.MasterConfig{
		Timeouts: common.TimeoutConfig{
			HTTPRequest: 100 * time.Millisecond,
		},
	}

	server := &Server{
		config: config,
		logger: logger,
		stats:  ServerStats{},
	}

	t.Run("Request completes before timeout", func(t *testing.T) {
		// Create a handler that responds quickly
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		})

		// Wrap with timeout middleware
		wrapped := server.timeoutMiddleware(200 * time.Millisecond)(handler)

		// Make request
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// Should succeed
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "success", rec.Body.String())
	})

	t.Run("Request exceeds timeout", func(t *testing.T) {
		// Create a handler that takes too long
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("should not see this"))
		})

		// Wrap with timeout middleware
		wrapped := server.timeoutMiddleware(100 * time.Millisecond)(handler)

		// Make request
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// Should timeout with 504
		assert.Equal(t, http.StatusGatewayTimeout, rec.Code)

		// Check response body
		var response map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "Gateway Timeout", response["error"])
		assert.Equal(t, float64(http.StatusGatewayTimeout), response["code"])
		assert.Contains(t, response["message"], "Request exceeded timeout")

		// Check error count was incremented
		assert.Equal(t, int64(1), server.stats.ErrorCount)
	})

	t.Run("Uses default timeout when zero", func(t *testing.T) {
		// Create a handler that responds after default timeout
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		// Wrap with timeout middleware (0 means use default)
		wrapped := server.timeoutMiddleware(0)(handler)

		// Make request
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// Should succeed (default is from config: 100ms)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
