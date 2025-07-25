package monitoring

import (
	"fmt"
	"net/http"

	"github.com/hibiken/asynq"
	"github.com/hibiken/asynqmon"
	"github.com/sirupsen/logrus"
)

// AsynqDashboard provides a web interface for monitoring asynq queues
type AsynqDashboard struct {
	handler http.Handler
	logger  *logrus.Logger
	port    int
}

// NewAsynqDashboard creates a new asynq monitoring dashboard
func NewAsynqDashboard(redisOpt asynq.RedisClientOpt, port int, logger *logrus.Logger) *AsynqDashboard {
	// Create asynqmon handler
	h := asynqmon.New(asynqmon.Options{
		RootPath:     "/asynq",
		RedisConnOpt: redisOpt,
	})

	return &AsynqDashboard{
		handler: h,
		logger:  logger,
		port:    port,
	}
}

// GetHandler returns the HTTP handler for the dashboard
func (d *AsynqDashboard) GetHandler() http.Handler {
	return d.handler
}

// Serve starts the dashboard on the configured port
func (d *AsynqDashboard) Serve() error {
	addr := fmt.Sprintf(":%d", d.port)
	d.logger.WithField("address", addr).Info("Starting asynq dashboard")

	// Create a mux and mount the dashboard
	mux := http.NewServeMux()
	mux.Handle("/", d.handler)

	// Add a simple health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return http.ListenAndServe(addr, mux)
}

// ServeOnMux mounts the dashboard on an existing mux
func (d *AsynqDashboard) ServeOnMux(mux *http.ServeMux, path string) {
	d.logger.WithField("path", path).Info("Mounting asynq dashboard")
	mux.Handle(path, http.StripPrefix(path, d.handler))
}
