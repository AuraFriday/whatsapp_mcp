package main

import (
  "fmt"
  "runtime/debug"
  "time"

  "github.com/google/uuid"
)

// NewErrorState creates a new error state manager
func NewErrorState(max_recent_errors int) *ErrorState {
  return &ErrorState{
    current_critical_error: nil,
    recent_errors:         make([]*ErrorEntry, 0, max_recent_errors),
    max_recent_errors:     max_recent_errors,
  }
}

// LogError logs an error to the error state
func (es *ErrorState) LogError(severity ErrorSeverity, operation string, message string, details string) *ErrorEntry {
  es.mu.Lock()
  defer es.mu.Unlock()

  entry := &ErrorEntry{
    ID:        uuid.New().String(),
    Timestamp: time.Now(),
    Severity:  severity,
    Operation: operation,
    Message:   message,
    Details:   details,
  }

  // Capture stack trace for errors and critical errors
  if severity == ErrorSeverityError || severity == ErrorSeverityCritical {
    entry.StackTrace = string(debug.Stack())
  }

  // If critical, set as current critical error
  if severity == ErrorSeverityCritical {
    es.current_critical_error = entry
  }

  // Add to recent errors
  es.recent_errors = append(es.recent_errors, entry)

  // Trim if exceeds max
  if len(es.recent_errors) > es.max_recent_errors {
    es.recent_errors = es.recent_errors[1:]
  }

  return entry
}

// GetCriticalError returns the current critical error, if any
func (es *ErrorState) GetCriticalError() *ErrorEntry {
  es.mu.RLock()
  defer es.mu.RUnlock()
  return es.current_critical_error
}

// HasCriticalError checks if there is a current critical error
func (es *ErrorState) HasCriticalError() bool {
  es.mu.RLock()
  defer es.mu.RUnlock()
  return es.current_critical_error != nil
}

// ClearCriticalError clears the current critical error
func (es *ErrorState) ClearCriticalError() {
  es.mu.Lock()
  defer es.mu.Unlock()
  es.current_critical_error = nil
}

// GetRecentErrors returns recent errors, optionally filtered by severity
func (es *ErrorState) GetRecentErrors(severity *ErrorSeverity, limit int) []*ErrorEntry {
  es.mu.RLock()
  defer es.mu.RUnlock()

  var filtered []*ErrorEntry
  for i := len(es.recent_errors) - 1; i >= 0; i-- {
    entry := es.recent_errors[i]
    if severity == nil || entry.Severity == *severity {
      filtered = append(filtered, entry)
      if limit > 0 && len(filtered) >= limit {
        break
      }
    }
  }

  return filtered
}

// ClearRecentErrors clears all non-critical recent errors
func (es *ErrorState) ClearRecentErrors() {
  es.mu.Lock()
  defer es.mu.Unlock()

  // Keep only critical errors
  var kept []*ErrorEntry
  for _, entry := range es.recent_errors {
    if entry.Severity == ErrorSeverityCritical {
      kept = append(kept, entry)
    }
  }

  es.recent_errors = kept
}

// CheckErrorState checks if there's a critical error and returns an error result if so
func (es *ErrorState) CheckErrorState(operation string) *OperationResult {
  if es.HasCriticalError() {
    criticalErr := es.GetCriticalError()
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Operation '%s' blocked due to critical error: %s (occurred at %s during '%s')", operation, criticalErr.Message, criticalErr.Timestamp.Format(time.RFC3339), criticalErr.Operation),
      Data: map[string]interface{}{
        "blocked_by_critical_error": true,
        "critical_error": map[string]interface{}{
          "id":        criticalErr.ID,
          "timestamp": criticalErr.Timestamp.Format(time.RFC3339),
          "operation": criticalErr.Operation,
          "message":   criticalErr.Message,
          "details":   criticalErr.Details,
        },
      },
    }
  }
  return nil
}

