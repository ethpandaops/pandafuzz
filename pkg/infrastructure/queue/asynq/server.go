package asynq

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/queue"
	"github.com/hibiken/asynq"
	"github.com/sirupsen/logrus"
)

// Server wraps asynq server with pandafuzz-specific functionality
type Server struct {
	server   *asynq.Server
	mux      *asynq.ServeMux
	logger   logrus.FieldLogger
	config   *config.QueueConfig
	handlers map[string]queue.HandlerFunc
	mu       sync.RWMutex
	running  bool
}

// NewServer creates a new asynq server wrapper
func NewServer(redisCfg *config.RedisConfig, queueCfg *config.QueueConfig, logger logrus.FieldLogger) (*Server, error) {
	redisOpt, err := NewRedisOpt(redisCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis options: %w", err)
	}

	serverCfg, err := NewServerConfig(queueCfg, redisCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create server config: %w", err)
	}

	// Override error handler to use our logger
	serverCfg.ErrorHandler = asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
		logger.WithError(err).WithFields(logrus.Fields{
			"task_type": task.Type(),
			"task_id":   func() string { id, _ := asynq.GetTaskID(ctx); return id }(),
			"retry":     func() int { n, _ := asynq.GetRetryCount(ctx); return n }(),
			"max_retry": func() int { n, _ := asynq.GetMaxRetry(ctx); return n }(),
		}).Error("Task processing failed")
	})

	server := asynq.NewServer(redisOpt, *serverCfg)
	mux := asynq.NewServeMux()

	// Set up middleware
	mux.Use(loggingMiddleware(logger))
	mux.Use(recoveryMiddleware(logger))

	return &Server{
		server:   server,
		mux:      mux,
		logger:   logger.WithField("component", "asynq-server"),
		config:   queueCfg,
		handlers: make(map[string]queue.HandlerFunc),
	}, nil
}

// Start begins processing tasks from queues
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	// Register all handlers with the mux
	for taskType, handler := range s.handlers {
		s.registerHandler(taskType, handler)
	}

	// Start the server in a goroutine
	go func() {
		s.logger.WithFields(logrus.Fields{
			"concurrency": s.config.Concurrency,
			"queues":      s.config.Queues,
		}).Info("Starting asynq server")

		if err := s.server.Run(s.mux); err != nil {
			s.logger.WithError(err).Error("Asynq server error")
		}
	}()

	s.running = true
	s.logger.Info("Server started")
	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("server not running")
	}

	s.logger.Info("Stopping server...")
	s.server.Stop()
	s.server.Shutdown()

	s.running = false
	s.logger.Info("Server stopped")
	return nil
}

// HandleFunc registers a handler function for a task type
func (s *Server) HandleFunc(taskType string, handler queue.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.handlers[taskType] = handler

	// If server is already running, register the handler immediately
	if s.running {
		s.registerHandler(taskType, handler)
	}
}

// registerHandler registers a handler with the asynq mux
func (s *Server) registerHandler(taskType string, handler queue.HandlerFunc) {
	// Wrap the handler to convert between asynq and our interface
	asynqHandler := func(ctx context.Context, task *asynq.Task) error {
		// Create a task wrapper that implements our Task interface
		wrappedTask := &taskWrapper{
			taskType: task.Type(),
			payload:  task.Payload(),
		}

		// Call our handler
		return handler(ctx, wrappedTask)
	}

	s.mux.HandleFunc(taskType, asynqHandler)
	s.logger.WithField("task_type", taskType).Info("Handler registered")
}

// IsRunning returns true if the server is running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// taskWrapper implements the queue.Task interface for asynq tasks
type taskWrapper struct {
	taskType string
	payload  []byte
}

func (t *taskWrapper) Type() string {
	return t.taskType
}

func (t *taskWrapper) Payload() []byte {
	return t.payload
}

// Middleware functions

// loggingMiddleware logs task processing
func loggingMiddleware(logger logrus.FieldLogger) asynq.MiddlewareFunc {
	return func(h asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			taskID, _ := asynq.GetTaskID(ctx)
			queueName, _ := asynq.GetQueueName(ctx)
			retryCount, _ := asynq.GetRetryCount(ctx)

			logger.WithFields(logrus.Fields{
				"task_type": task.Type(),
				"task_id":   taskID,
				"queue":     queueName,
				"retry":     retryCount,
			}).Debug("Processing task")

			err := h.ProcessTask(ctx, task)

			if err != nil {
				logger.WithError(err).WithFields(logrus.Fields{
					"task_type": task.Type(),
					"task_id":   taskID,
				}).Warn("Task processing failed")
			} else {
				logger.WithFields(logrus.Fields{
					"task_type": task.Type(),
					"task_id":   taskID,
				}).Debug("Task processed successfully")
			}

			return err
		})
	}
}

// recoveryMiddleware recovers from panics
func recoveryMiddleware(logger logrus.FieldLogger) asynq.MiddlewareFunc {
	return func(h asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) (err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.WithFields(logrus.Fields{
						"task_type": task.Type(),
						"panic":     r,
					}).Error("Task handler panicked")

					err = fmt.Errorf("panic recovered: %v", r)
				}
			}()

			return h.ProcessTask(ctx, task)
		})
	}
}

// ServerStats represents server statistics
type ServerStats struct {
	// Active is the number of workers currently processing tasks
	Active int `json:"active"`

	// Idle is the number of idle workers
	Idle int `json:"idle"`

	// Total is the total number of workers
	Total int `json:"total"`

	// QueueSizes maps queue names to their current sizes
	QueueSizes map[string]int `json:"queue_sizes"`

	// ProcessedTotal is the total number of tasks processed
	ProcessedTotal int64 `json:"processed_total"`

	// FailedTotal is the total number of failed tasks
	FailedTotal int64 `json:"failed_total"`
}

// GetStats returns server statistics
func (s *Server) GetStats() (*ServerStats, error) {
	// Note: asynq doesn't expose detailed server stats directly
	// This would need to be tracked separately or use the inspector
	stats := &ServerStats{
		Total:      s.config.Concurrency,
		QueueSizes: make(map[string]int),
	}

	// Would need to use inspector to get queue sizes
	// This is a simplified version
	return stats, nil
}
