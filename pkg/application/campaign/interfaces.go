package campaign

import (
	"context"
)

// Command represents a write operation that changes state
type Command interface {
	Execute(ctx context.Context) error
}

// Query represents a read operation that doesn't change state
type Query interface {
	Execute(ctx context.Context) (interface{}, error)
}

// CommandHandler handles command execution
type CommandHandler interface {
	Handle(ctx context.Context, command Command) error
}

// QueryHandler handles query execution
type QueryHandler interface {
	Handle(ctx context.Context, query Query) (interface{}, error)
}

// EventBus publishes domain events
type EventBus interface {
	Publish(ctx context.Context, event interface{}) error
	Subscribe(eventType string, handler func(ctx context.Context, event interface{}) error) error
}

// CQRSHandler provides unified handling for commands and queries
type CQRSHandler struct {
	commandHandlers map[string]CommandHandler
	queryHandlers   map[string]QueryHandler
}

// NewCQRSHandler creates a new CQRS handler
func NewCQRSHandler() *CQRSHandler {
	return &CQRSHandler{
		commandHandlers: make(map[string]CommandHandler),
		queryHandlers:   make(map[string]QueryHandler),
	}
}

// RegisterCommandHandler registers a command handler
func (h *CQRSHandler) RegisterCommandHandler(commandType string, handler CommandHandler) {
	h.commandHandlers[commandType] = handler
}

// RegisterQueryHandler registers a query handler
func (h *CQRSHandler) RegisterQueryHandler(queryType string, handler QueryHandler) {
	h.queryHandlers[queryType] = handler
}

// HandleCommand executes a command
func (h *CQRSHandler) HandleCommand(ctx context.Context, commandType string, command Command) error {
	handler, exists := h.commandHandlers[commandType]
	if !exists {
		return NewApplicationError(ErrCodeCommandNotFound, "Command handler not found", nil).
			WithDetails("command_type", commandType)
	}
	return handler.Handle(ctx, command)
}

// HandleQuery executes a query
func (h *CQRSHandler) HandleQuery(ctx context.Context, queryType string, query Query) (interface{}, error) {
	handler, exists := h.queryHandlers[queryType]
	if !exists {
		return nil, NewApplicationError(ErrCodeQueryNotFound, "Query handler not found", nil).
			WithDetails("query_type", queryType)
	}
	return handler.Handle(ctx, query)
}

// Common application error codes
const (
	ErrCodeCommandNotFound  = "COMMAND_NOT_FOUND"
	ErrCodeQueryNotFound    = "QUERY_NOT_FOUND"
	ErrCodeValidationFailed = "VALIDATION_FAILED"
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeConflict         = "CONFLICT"
	ErrCodeOperationFailed  = "OPERATION_FAILED"
)

// ApplicationError represents an application layer error
type ApplicationError struct {
	Code    string
	Message string
	Details map[string]interface{}
	Cause   error
}

// Error implements the error interface
func (e ApplicationError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// WithDetails adds details to the error
func (e ApplicationError) WithDetails(key string, value interface{}) ApplicationError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// NewApplicationError creates a new application error
func NewApplicationError(code, message string, cause error) ApplicationError {
	return ApplicationError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
		Cause:   cause,
	}
}
