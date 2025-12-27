package master

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// setupRouter configures the HTTP router with all routes and middleware
func (s *Server) setupRouter() error {
	// Always use Chi router with unified API v1
	s.setupChiRouter()
	s.logger.Info("HTTP router configured with unified API v1")
	return nil
}

// setupChiRouter configures the Chi router with unified API v1
func (s *Server) setupChiRouter() error {
	s.chiRouter = chi.NewRouter()

	// Add Chi middleware
	s.chiRouter.Use(middleware.Logger)
	s.chiRouter.Use(middleware.Recoverer)
	s.chiRouter.Use(middleware.RequestID)
	s.chiRouter.Use(middleware.RealIP)

	// Add custom middleware
	if s.config.Server.EnableCORS {
		s.chiRouter.Use(s.corsMiddlewareForChi())
	}

	// Add rate limiting if configured
	if s.config.Server.RateLimitRPS > 0 {
		s.chiRouter.Use(s.rateLimitMiddlewareForChi())
	}

	// Health and metrics endpoints (direct on Chi router)
	s.chiRouter.Get("/health", s.handleHealth)
	s.chiRouter.Get("/status", s.handleStatus)

	// Prometheus metrics endpoint
	if s.config.Monitoring.Enabled {
		s.chiRouter.Handle("/metrics", promhttp.Handler())
	} else {
		s.chiRouter.Get("/metrics", s.handleMetrics)
	}

	// WebSocket endpoint for real-time updates
	s.chiRouter.Get("/ws", s.handleWebSocket)

	// Mount unified API routes
	if s.apiV1 != nil {
		s.chiRouter.Mount("/api/v1", s.apiV1.GetRouter())
		s.logger.Info("API routes mounted at /api/v1")
	}

	// Serve static files for web UI
	s.setupStaticFileServingOnChi()

	return nil
}

// Legacy router setup functions have been removed.
// All API endpoints are now unified under pkg/api/v1 using Chi router.

// spaFileHandler serves static files and handles SPA routing
type spaFileHandler struct {
	staticPath string
	fileServer http.Handler
}

func (h *spaFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only skip paths that are handled by explicit routes on the Chi router
	// API routes are mounted at /api/v1
	// Other explicit routes: /metrics, /health, /status, /ws
	// Static assets: /static/, /css/, /js/
	if strings.HasPrefix(r.URL.Path, "/api/") ||
		strings.HasPrefix(r.URL.Path, "/metrics") ||
		strings.HasPrefix(r.URL.Path, "/health") ||
		strings.HasPrefix(r.URL.Path, "/status") ||
		strings.HasPrefix(r.URL.Path, "/ws") ||
		strings.HasPrefix(r.URL.Path, "/css/") ||
		strings.HasPrefix(r.URL.Path, "/js/") {
		// These are handled by other Chi routes - don't serve index.html
		http.NotFound(w, r)
		return
	}

	// Get the absolute path to prevent directory traversal
	path := filepath.Join(h.staticPath, r.URL.Path)

	// Check if file exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) || r.URL.Path == "/" {
		// File doesn't exist or root path, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, "index.html"))
		return
	}

	// Serve the requested file
	h.fileServer.ServeHTTP(w, r)
}

// corsMiddlewareForChi returns CORS middleware for Chi router
func (s *Server) corsMiddlewareForChi() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitMiddlewareForChi returns rate limiting middleware for Chi router
// This delegates to the main rateLimitMiddleware implementation
func (s *Server) rateLimitMiddlewareForChi() func(http.Handler) http.Handler {
	return s.rateLimitMiddleware
}

// setupStaticFileServingOnChi configures static file serving for Chi router
func (s *Server) setupStaticFileServingOnChi() {
	// Check if web UI directory exists
	webStaticDir := "./web/static"
	if _, err := os.Stat(webStaticDir); os.IsNotExist(err) {
		s.logger.WithField("dir", webStaticDir).Warn("Web UI static directory not found, skipping static file serving")
		return
	}

	// Serve CSS files
	cssDir := "./web/css"
	if _, err := os.Stat(cssDir); err == nil {
		cssFileServer := http.FileServer(http.Dir(cssDir))
		s.chiRouter.Mount("/css", http.StripPrefix("/css", cssFileServer))
	}

	// Serve JS files
	jsDir := "./web/js"
	if _, err := os.Stat(jsDir); err == nil {
		jsFileServer := http.FileServer(http.Dir(jsDir))
		s.chiRouter.Mount("/js", http.StripPrefix("/js", jsFileServer))
	}

	// Create file server for static HTML files
	fileServer := http.FileServer(http.Dir(webStaticDir))

	// SPA handler - serves HTML files
	spaHandler := &spaFileHandler{
		staticPath: webStaticDir,
		fileServer: fileServer,
	}

	// Serve static HTML files - catch-all for everything not matched above
	s.chiRouter.Get("/*", spaHandler.ServeHTTP)

	s.logger.WithField("dir", webStaticDir).Info("Static file serving configured for web UI on Chi router")
}
