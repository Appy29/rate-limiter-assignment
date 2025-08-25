package utils

import (
	"context"
	"log"
	"time"
)

// Context key type to avoid collisions
type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	LoggerKey    contextKey = "logger"
	StartTimeKey contextKey = "start_time"
)

// Logger interface for our context logger
type ContextLogger struct {
	RequestID string
}

// NewContextLogger creates a new context logger
func NewContextLogger(requestID string) *ContextLogger {
	return &ContextLogger{
		RequestID: requestID,
	}
}

// Info logs info level messages with request ID
func (l *ContextLogger) Info(msg string, args ...interface{}) {
	log.Printf("[INFO] [%s] %s", l.RequestID, msg)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			log.Printf("  %v: %v", args[i], args[i+1])
		}
	}
}

// Error logs error level messages with request ID
func (l *ContextLogger) Error(msg string, err error, args ...interface{}) {
	log.Printf("[ERROR] [%s] %s: %v", l.RequestID, msg, err)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			log.Printf("  %v: %v", args[i], args[i+1])
		}
	}
}

// Warn logs warning level messages with request ID
func (l *ContextLogger) Warn(msg string, args ...interface{}) {
	log.Printf("[WARN] [%s] %s", l.RequestID, msg)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			log.Printf("  %v: %v", args[i], args[i+1])
		}
	}
}

// GetLoggerFromContext extracts logger from context
func GetLoggerFromContext(ctx context.Context) *ContextLogger {
	if logger, ok := ctx.Value(LoggerKey).(*ContextLogger); ok {
		return logger
	}
	// Return default logger if not found
	return NewContextLogger("unknown")
}

// GetRequestIDFromContext extracts request ID from context
func GetRequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return "unknown"
}

// GetStartTimeFromContext extracts start time from context
func GetStartTimeFromContext(ctx context.Context) time.Time {
	if startTime, ok := ctx.Value(StartTimeKey).(time.Time); ok {
		return startTime
	}
	return time.Now()
}
