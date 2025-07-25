package v2

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// RouterConfig contains configuration for the V2 API router
type RouterConfig struct {
	Logger logrus.FieldLogger
}

// Router manages the V2 API routes
type Router struct {
	config *RouterConfig
	logger logrus.FieldLogger
}

// NewRouter creates a new V2 API router
func NewRouter(config *RouterConfig) *Router {
	if config.Logger == nil {
		config.Logger = logrus.New()
	}
	return &Router{
		config: config,
		logger: config.Logger,
	}
}

// SetupRoutes configures all V2 API routes on the provided router
func (r *Router) SetupRoutes(router *mux.Router) {
	// V2 API is temporarily disabled while being refactored
	router.HandleFunc("/{path:.*}", r.notImplemented).Methods("GET", "POST", "PUT", "DELETE", "PATCH")
}

// notImplemented handler for V2 routes
func (r *Router) notImplemented(w http.ResponseWriter, req *http.Request) {
	response := map[string]interface{}{
		"error":     "Not Implemented",
		"message":   "V2 API is temporarily disabled while being refactored",
		"timestamp": time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(response)
}
