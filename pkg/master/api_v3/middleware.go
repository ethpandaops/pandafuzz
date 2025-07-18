package api_v3

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// BackwardsCompatibilityMiddleware provides backwards compatibility for v2 clients
type BackwardsCompatibilityMiddleware struct {
	next http.Handler
}

// NewBackwardsCompatibilityMiddleware creates a new backwards compatibility middleware
func NewBackwardsCompatibilityMiddleware(next http.Handler) *BackwardsCompatibilityMiddleware {
	return &BackwardsCompatibilityMiddleware{next: next}
}

// ServeHTTP implements http.Handler
func (m *BackwardsCompatibilityMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check for v2 API calls
	if strings.HasPrefix(r.URL.Path, "/api/v2/") {
		// Transform v2 to v3 path
		r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/api/v3/", 1)

		// Add compatibility header
		w.Header().Set("X-API-Compatibility", "v2-to-v3")

		// Transform request body if needed
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
			m.transformV2Request(r)
		}

		// Wrap response writer to transform response
		wrapped := &v2ResponseWriter{
			ResponseWriter: w,
			transform:      true,
		}

		m.next.ServeHTTP(wrapped, r)
		return
	}

	// Pass through v3 requests
	m.next.ServeHTTP(w, r)
}

// transformV2Request transforms v2 request format to v3
func (m *BackwardsCompatibilityMiddleware) transformV2Request(r *http.Request) {
	// Read body
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return
	}

	// Transform based on endpoint
	switch {
	case strings.Contains(r.URL.Path, "/jobs"):
		// v2 uses "corpus_id", v3 uses "campaign_id" for some operations
		if corpusID, ok := body["corpus_id"].(string); ok {
			body["campaign_id"] = corpusID
			delete(body, "corpus_id")
		}

	case strings.Contains(r.URL.Path, "/bots"):
		// v2 uses "capabilities" as string, v3 uses array
		if caps, ok := body["capabilities"].(string); ok {
			body["capabilities"] = strings.Split(caps, ",")
		}
	}

	// Re-encode body
	newBody, _ := json.Marshal(body)
	r.Body = io.NopCloser(strings.NewReader(string(newBody)))
	r.ContentLength = int64(len(newBody))
}

// v2ResponseWriter wraps http.ResponseWriter to transform v3 responses to v2 format
type v2ResponseWriter struct {
	http.ResponseWriter
	transform  bool
	statusCode int
	body       []byte
}

func (w *v2ResponseWriter) WriteHeader(code int) {
	w.statusCode = code
}

func (w *v2ResponseWriter) Write(data []byte) (int, error) {
	if w.transform {
		w.body = append(w.body, data...)
		return len(data), nil
	}
	return w.ResponseWriter.Write(data)
}

func (w *v2ResponseWriter) Flush() {
	if !w.transform {
		if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}

	// Transform response
	var response map[string]interface{}
	if err := json.Unmarshal(w.body, &response); err == nil {
		// Transform based on content
		if jobs, ok := response["jobs"].([]interface{}); ok {
			// Transform job responses
			for _, job := range jobs {
				if j, ok := job.(map[string]interface{}); ok {
					// v2 uses "corpus_id" instead of "campaign_id"
					if campaignID, ok := j["campaign_id"]; ok {
						j["corpus_id"] = campaignID
					}
				}
			}
		}

		// Re-encode
		transformed, _ := json.Marshal(response)
		w.body = transformed
	}

	// Write actual response
	if w.statusCode != 0 {
		w.ResponseWriter.WriteHeader(w.statusCode)
	}
	w.ResponseWriter.Write(w.body)
}

// DeprecationMiddleware adds deprecation warnings for old API versions
type DeprecationMiddleware struct {
	next http.Handler
}

// NewDeprecationMiddleware creates a new deprecation middleware
func NewDeprecationMiddleware(next http.Handler) *DeprecationMiddleware {
	return &DeprecationMiddleware{next: next}
}

// ServeHTTP implements http.Handler
func (m *DeprecationMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check API version
	if strings.HasPrefix(r.URL.Path, "/api/v1/") || strings.HasPrefix(r.URL.Path, "/api/v2/") {
		// Add deprecation headers
		w.Header().Set("X-API-Deprecated", "true")
		w.Header().Set("X-API-Deprecation-Date", "2024-12-31")
		w.Header().Set("X-API-Sunset-Date", "2025-06-30")
		w.Header().Set("X-API-Migration-Guide", "https://pandafuzz.example.com/docs/api/migration")

		// Add warning header (RFC 7234)
		version := "v1"
		if strings.HasPrefix(r.URL.Path, "/api/v2/") {
			version = "v2"
		}
		w.Header().Set("Warning", `299 - "API version `+version+` is deprecated. Please migrate to v3."`)
	}

	m.next.ServeHTTP(w, r)
}

// CORSMiddleware handles CORS headers
type CORSMiddleware struct {
	next           http.Handler
	allowedOrigins []string
}

// NewCORSMiddleware creates a new CORS middleware
func NewCORSMiddleware(next http.Handler, allowedOrigins []string) *CORSMiddleware {
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}
	return &CORSMiddleware{
		next:           next,
		allowedOrigins: allowedOrigins,
	}
}

// ServeHTTP implements http.Handler
func (m *CORSMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowed := false

	// Check if origin is allowed
	for _, allowedOrigin := range m.allowedOrigins {
		if allowedOrigin == "*" || allowedOrigin == origin {
			allowed = true
			break
		}
	}

	if allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")
	}

	// Handle preflight
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	m.next.ServeHTTP(w, r)
}
