package middleware

import (
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/interfaces/api/rest/v1"
	"github.com/sirupsen/logrus"
)

// RecoveryConfig holds configuration for recovery middleware
type RecoveryConfig struct {
	// Logger for panic messages
	Logger logrus.FieldLogger

	// StackSize is the maximum size of stack trace to capture
	StackSize int

	// SkipFrames is the number of stack frames to skip when logging
	SkipFrames int

	// DisableStackAll disables capturing stack traces of all goroutines
	DisableStackAll bool

	// PrintStack enables printing stack trace to stderr
	PrintStack bool

	// Recovery handler allows custom recovery behavior
	RecoveryHandler func(http.ResponseWriter, *http.Request, interface{})

	// EnableDetailedErrors includes stack traces in HTTP responses (dev only)
	EnableDetailedErrors bool

	// NotifyChannels is a list of channels to notify on panic
	NotifyChannels []chan<- PanicInfo

	// CustomFields allows adding custom fields to panic logs
	CustomFields map[string]interface{}
}

// PanicInfo contains information about a panic
type PanicInfo struct {
	Timestamp   time.Time
	RequestID   string
	Method      string
	Path        string
	RemoteAddr  string
	UserAgent   string
	Panic       interface{}
	Stack       string
	Goroutines  string
	Headers     map[string][]string
	QueryParams string
}

// DefaultRecoveryConfig returns a default recovery configuration
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		Logger:               logrus.NewEntry(logrus.StandardLogger()),
		StackSize:            4096,
		SkipFrames:           3,
		DisableStackAll:      false,
		PrintStack:           false,
		EnableDetailedErrors: false,
		CustomFields:         make(map[string]interface{}),
	}
}

// Recovery creates a panic recovery middleware with default configuration
func Recovery() func(http.Handler) http.Handler {
	return RecoveryWithConfig(DefaultRecoveryConfig())
}

// RecoveryWithConfig creates a panic recovery middleware with custom configuration
func RecoveryWithConfig(config RecoveryConfig) func(http.Handler) http.Handler {
	if config.Logger == nil {
		config.Logger = logrus.NewEntry(logrus.StandardLogger())
	}

	if config.StackSize <= 0 {
		config.StackSize = 4096
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					handlePanic(w, r, err, config)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// handlePanic handles a recovered panic
func handlePanic(w http.ResponseWriter, r *http.Request, panicValue interface{}, config RecoveryConfig) {
	timestamp := time.Now()

	// Capture stack trace
	stack := captureStack(config.StackSize, config.SkipFrames)

	// Capture all goroutines stack trace if enabled
	var allGoroutines string
	if !config.DisableStackAll {
		allGoroutines = string(debug.Stack())
	}

	// Extract request information
	correlationID := GetCorrelationID(r)
	if correlationID == "" {
		correlationID = generateCorrelationID()
	}

	panicInfo := PanicInfo{
		Timestamp:   timestamp,
		RequestID:   correlationID,
		Method:      r.Method,
		Path:        r.URL.Path,
		RemoteAddr:  getClientIP(r),
		UserAgent:   r.Header.Get("User-Agent"),
		Panic:       panicValue,
		Stack:       stack,
		Goroutines:  allGoroutines,
		Headers:     r.Header,
		QueryParams: r.URL.RawQuery,
	}

	// Log the panic
	logPanic(panicInfo, config)

	// Print stack to stderr if enabled
	if config.PrintStack {
		fmt.Printf("PANIC: %v\n%s\n", panicValue, stack)
	}

	// Notify channels
	notifyChannels(panicInfo, config.NotifyChannels)

	// Handle recovery
	if config.RecoveryHandler != nil {
		config.RecoveryHandler(w, r, panicValue)
	} else {
		defaultRecoveryHandler(w, r, panicInfo, config)
	}
}

// logPanic logs the panic with detailed information
func logPanic(info PanicInfo, config RecoveryConfig) {
	fields := logrus.Fields{
		"event":        "panic_recovered",
		"panic_value":  info.Panic,
		"request_id":   info.RequestID,
		"method":       info.Method,
		"path":         info.Path,
		"remote_addr":  info.RemoteAddr,
		"user_agent":   info.UserAgent,
		"timestamp":    info.Timestamp,
		"stack_trace":  info.Stack,
		"query_params": info.QueryParams,
	}

	// Add goroutines info if available
	if info.Goroutines != "" {
		fields["all_goroutines"] = info.Goroutines
	}

	// Add custom fields
	for key, value := range config.CustomFields {
		fields[key] = value
	}

	// Mask sensitive headers
	maskedHeaders := make(map[string]interface{})
	sensitiveHeaders := []string{"Authorization", "X-API-Key", "Cookie", "Set-Cookie"}
	for name, values := range info.Headers {
		if isSensitiveHeader(name, sensitiveHeaders) {
			maskedHeaders[name] = "[MASKED]"
		} else {
			maskedHeaders[name] = values
		}
	}
	fields["headers"] = maskedHeaders

	config.Logger.WithFields(fields).Error("Panic recovered in HTTP handler")
}

// defaultRecoveryHandler provides default panic recovery response
func defaultRecoveryHandler(w http.ResponseWriter, r *http.Request, info PanicInfo, config RecoveryConfig) {
	// Ensure headers can still be written
	if !isHeadersWritten(w) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Correlation-ID", info.RequestID)
	}

	// Prepare error details
	details := map[string]interface{}{
		"request_id": info.RequestID,
		"timestamp":  info.Timestamp.Format(time.RFC3339),
	}

	// Include stack trace in response for development
	if config.EnableDetailedErrors {
		details["panic_value"] = fmt.Sprintf("%v", info.Panic)
		details["stack_trace"] = strings.Split(info.Stack, "\n")
	}

	// Write error response using RFC 7807 format
	v1.WriteErrorWithDetails(w, http.StatusInternalServerError, "Internal server error", details)
}

// captureStack captures the current stack trace
func captureStack(size, skip int) string {
	buf := make([]byte, size)
	n := runtime.Stack(buf, false)
	stack := string(buf[:n])

	// Skip specified number of frames
	lines := strings.Split(stack, "\n")
	if skip > 0 && len(lines) > skip*2 {
		// Each frame typically takes 2 lines (function + file:line)
		lines = lines[skip*2:]
		stack = strings.Join(lines, "\n")
	}

	return stack
}

// notifyChannels sends panic information to notification channels
func notifyChannels(info PanicInfo, channels []chan<- PanicInfo) {
	for _, ch := range channels {
		select {
		case ch <- info:
			// Successfully sent
		default:
			// Channel is full or closed, skip
		}
	}
}

// isHeadersWritten checks if HTTP headers have been written
func isHeadersWritten(w http.ResponseWriter) bool {
	// This is a heuristic - in practice, you might want to use a custom ResponseWriter
	// that tracks header writing state
	return false
}

// PanicRecoveryWithHandler creates a recovery middleware with custom handler
func PanicRecoveryWithHandler(handler func(http.ResponseWriter, *http.Request, interface{})) func(http.Handler) http.Handler {
	config := DefaultRecoveryConfig()
	config.RecoveryHandler = handler
	return RecoveryWithConfig(config)
}

// DevelopmentRecovery returns a recovery configuration suitable for development
func DevelopmentRecovery() RecoveryConfig {
	return RecoveryConfig{
		Logger:               logrus.NewEntry(logrus.StandardLogger()),
		StackSize:            8192,
		SkipFrames:           3,
		DisableStackAll:      false,
		PrintStack:           true,
		EnableDetailedErrors: true,
		CustomFields: map[string]interface{}{
			"environment": "development",
		},
	}
}

// ProductionRecovery returns a recovery configuration suitable for production
func ProductionRecovery() RecoveryConfig {
	return RecoveryConfig{
		Logger:               logrus.NewEntry(logrus.StandardLogger()),
		StackSize:            4096,
		SkipFrames:           3,
		DisableStackAll:      true,
		PrintStack:           false,
		EnableDetailedErrors: false,
		CustomFields: map[string]interface{}{
			"environment": "production",
		},
	}
}

// PanicNotifier creates a channel that receives panic notifications
func PanicNotifier(bufferSize int) chan PanicInfo {
	return make(chan PanicInfo, bufferSize)
}

// MonitorPanics starts a goroutine that monitors panic notifications
func MonitorPanics(ch <-chan PanicInfo, handler func(PanicInfo)) {
	go func() {
		for info := range ch {
			handler(info)
		}
	}()
}

// AdvancedRecovery creates a recovery middleware with monitoring and alerting
func AdvancedRecovery(config RecoveryConfig, alertHandler func(PanicInfo)) func(http.Handler) http.Handler {
	if alertHandler != nil {
		notificationCh := PanicNotifier(100)
		config.NotifyChannels = append(config.NotifyChannels, notificationCh)
		MonitorPanics(notificationCh, alertHandler)
	}

	return RecoveryWithConfig(config)
}

// PanicCounter tracks panic statistics
type PanicCounter struct {
	total  int64
	byPath map[string]int64
	byType map[string]int64
}

// NewPanicCounter creates a new panic counter
func NewPanicCounter() *PanicCounter {
	return &PanicCounter{
		byPath: make(map[string]int64),
		byType: make(map[string]int64),
	}
}

// Record records a panic occurrence
func (pc *PanicCounter) Record(info PanicInfo) {
	pc.total++
	pc.byPath[info.Path]++

	panicType := fmt.Sprintf("%T", info.Panic)
	pc.byType[panicType]++
}

// GetStats returns panic statistics
func (pc *PanicCounter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total":   pc.total,
		"by_path": pc.byPath,
		"by_type": pc.byType,
	}
}

// RecoveryWithMetrics creates a recovery middleware that tracks panic metrics
func RecoveryWithMetrics(config RecoveryConfig, counter *PanicCounter) func(http.Handler) http.Handler {
	if counter != nil {
		notificationCh := PanicNotifier(100)
		config.NotifyChannels = append(config.NotifyChannels, notificationCh)
		MonitorPanics(notificationCh, counter.Record)
	}

	return RecoveryWithConfig(config)
}
