package main

import (
  "sync"
  "time"
)

// ErrorSeverity represents the severity level of an error
type ErrorSeverity string

const (
  ErrorSeverityInfo     ErrorSeverity = "info"
  ErrorSeverityWarning  ErrorSeverity = "warning"
  ErrorSeverityError    ErrorSeverity = "error"
  ErrorSeverityCritical ErrorSeverity = "critical"
)

// ErrorEntry represents a single error in the error log
type ErrorEntry struct {
  ID        string        `json:"id"`
  Timestamp time.Time     `json:"timestamp"`
  Severity  ErrorSeverity `json:"severity"`
  Operation string        `json:"operation"`
  Message   string        `json:"message"`
  Details   string        `json:"details,omitempty"`
  StackTrace string       `json:"stack_trace,omitempty"`
}

// ErrorState represents the current error state of the application
type ErrorState struct {
  mu                    sync.RWMutex
  current_critical_error *ErrorEntry
  recent_errors         []*ErrorEntry
  max_recent_errors     int
}

// Config represents the application configuration
type Config struct {
  mu                    sync.RWMutex
  database_path         string
  handlers_database_path string
  media_download_path   string
  log_level             string
  log_file              string
  auto_reconnect        bool
  auto_read_receipts    bool
  auto_presence         bool
  handler_timeout       int
  max_parallel_handlers int
}

// ConnectionState represents the WhatsApp connection state
type ConnectionState string

const (
  StateDisconnected ConnectionState = "disconnected"
  StateConnecting   ConnectionState = "connecting"
  StateConnected    ConnectionState = "connected"
  StateReconnecting ConnectionState = "reconnecting"
  StateError        ConnectionState = "error"
)

// WhatsAppState represents the current state of the WhatsApp client
type WhatsAppState struct {
  mu                sync.RWMutex
  connection_state  ConnectionState
  phone_number      string
  device_id         string
  last_connected    time.Time
  last_disconnected time.Time
  reconnect_attempts int
}

// OperationInput represents the input for all operations
type OperationInput struct {
  Operation string                 `json:"operation"`
  Data      map[string]interface{} `json:"data,omitempty"`
}

// OperationResult represents the result of an operation
type OperationResult struct {
  Success bool                   `json:"success"`
  Message string                 `json:"message,omitempty"`
  Data    map[string]interface{} `json:"data,omitempty"`
  Error   string                 `json:"error,omitempty"`
}

