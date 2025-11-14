package main

import (
  "database/sql"
  "encoding/json"
  "fmt"
  "os"
  "path/filepath"
  "time"

  _ "github.com/mattn/go-sqlite3"
)

// Database represents the error logging database
type Database struct {
  db *sql.DB
}

// NewDatabase creates a new database connection
func NewDatabase(dbPath string) (*Database, error) {
  // Ensure directory exists
  dir := filepath.Dir(dbPath)
  if err := os.MkdirAll(dir, 0755); err != nil {
    return nil, fmt.Errorf("failed to create database directory: %w", err)
  }

  db, err := sql.Open("sqlite3", dbPath)
  if err != nil {
    return nil, fmt.Errorf("failed to open database: %w", err)
  }

  database := &Database{db: db}
  if err := database.initSchema(); err != nil {
    db.Close()
    return nil, fmt.Errorf("failed to initialize schema: %w", err)
  }

  return database, nil
}

// initSchema initializes the database schema
func (d *Database) initSchema() error {
  schema := `
  CREATE TABLE IF NOT EXISTS error_log (
    id TEXT PRIMARY KEY,
    timestamp TIMESTAMP NOT NULL,
    severity TEXT NOT NULL,
    operation TEXT NOT NULL,
    message TEXT NOT NULL,
    details TEXT,
    stack_trace TEXT
  );

  CREATE INDEX IF NOT EXISTS idx_error_log_timestamp ON error_log(timestamp DESC);
  CREATE INDEX IF NOT EXISTS idx_error_log_severity ON error_log(severity);
  CREATE INDEX IF NOT EXISTS idx_error_log_operation ON error_log(operation);

  CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
  );

  CREATE TABLE IF NOT EXISTS connection_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TIMESTAMP NOT NULL,
    event_type TEXT NOT NULL,
    details TEXT
  );

  CREATE INDEX IF NOT EXISTS idx_connection_log_timestamp ON connection_log(timestamp DESC);

  CREATE TABLE IF NOT EXISTS messages (
    message_id TEXT PRIMARY KEY,
    timestamp TIMESTAMP NOT NULL,
    from_jid TEXT NOT NULL,
    chat_jid TEXT NOT NULL,
    sender_name TEXT,
    is_group INTEGER NOT NULL DEFAULT 0,
    is_from_me INTEGER NOT NULL DEFAULT 0,
    message_type TEXT NOT NULL,
    text_content TEXT,
    media_type TEXT,
    media_mime_type TEXT,
    media_size INTEGER,
    quoted_message_id TEXT,
    raw_message TEXT
  );

  CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp DESC);
  CREATE INDEX IF NOT EXISTS idx_messages_from ON messages(from_jid);
  CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_jid);
  CREATE INDEX IF NOT EXISTS idx_messages_type ON messages(message_type);

  CREATE TABLE IF NOT EXISTS event_handlers (
    handler_id TEXT PRIMARY KEY,
    description TEXT,
    event_filter TEXT NOT NULL,
    action TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    priority INTEGER DEFAULT 0,
    max_executions_per_minute INTEGER,
    max_executions_per_hour INTEGER,
    max_executions_per_sender_per_hour INTEGER,
    cooldown_seconds INTEGER DEFAULT 0,
    timeout_seconds INTEGER DEFAULT 30,
    circuit_breaker_enabled INTEGER DEFAULT 1,
    circuit_breaker_threshold INTEGER DEFAULT 5,
    circuit_breaker_reset_seconds INTEGER DEFAULT 300,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    execution_count INTEGER DEFAULT 0,
    last_executed TIMESTAMP,
    last_error TEXT,
    last_error_time TIMESTAMP,
    total_errors INTEGER DEFAULT 0,
    circuit_breaker_state TEXT DEFAULT 'closed'
  );

  CREATE INDEX IF NOT EXISTS idx_handlers_enabled ON event_handlers(enabled);
  CREATE INDEX IF NOT EXISTS idx_handlers_priority ON event_handlers(priority DESC);

  CREATE TABLE IF NOT EXISTS handler_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    handler_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    from_jid TEXT,
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    duration_ms INTEGER,
    success INTEGER,
    error TEXT,
    actions_executed INTEGER,
    FOREIGN KEY (handler_id) REFERENCES event_handlers(handler_id)
  );

  CREATE INDEX IF NOT EXISTS idx_executions_handler ON handler_executions(handler_id);
  CREATE INDEX IF NOT EXISTS idx_executions_from ON handler_executions(from_jid);
  CREATE INDEX IF NOT EXISTS idx_executions_time ON handler_executions(started_at DESC);
  `

  _, err := d.db.Exec(schema)
  return err
}

// LogError logs an error to the database
func (d *Database) LogError(entry *ErrorEntry) error {
  query := `
  INSERT INTO error_log (id, timestamp, severity, operation, message, details, stack_trace)
  VALUES (?, ?, ?, ?, ?, ?, ?)
  `

  _, err := d.db.Exec(query,
    entry.ID,
    entry.Timestamp,
    entry.Severity,
    entry.Operation,
    entry.Message,
    entry.Details,
    entry.StackTrace,
  )

  return err
}

// GetRecentErrors retrieves recent errors from the database
func (d *Database) GetRecentErrors(severity *ErrorSeverity, limit int) ([]*ErrorEntry, error) {
  var query string
  var args []interface{}

  if severity != nil {
    query = `
    SELECT id, timestamp, severity, operation, message, details, stack_trace
    FROM error_log
    WHERE severity = ?
    ORDER BY timestamp DESC
    LIMIT ?
    `
    args = []interface{}{*severity, limit}
  } else {
    query = `
    SELECT id, timestamp, severity, operation, message, details, stack_trace
    FROM error_log
    ORDER BY timestamp DESC
    LIMIT ?
    `
    args = []interface{}{limit}
  }

  rows, err := d.db.Query(query, args...)
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  var errors []*ErrorEntry
  for rows.Next() {
    entry := &ErrorEntry{}
    var stackTrace sql.NullString
    var details sql.NullString

    err := rows.Scan(
      &entry.ID,
      &entry.Timestamp,
      &entry.Severity,
      &entry.Operation,
      &entry.Message,
      &details,
      &stackTrace,
    )
    if err != nil {
      return nil, err
    }

    if details.Valid {
      entry.Details = details.String
    }
    if stackTrace.Valid {
      entry.StackTrace = stackTrace.String
    }

    errors = append(errors, entry)
  }

  return errors, rows.Err()
}

// ClearOldErrors clears errors older than the specified duration
func (d *Database) ClearOldErrors(olderThan time.Duration) error {
  cutoff := time.Now().Add(-olderThan)
  query := `DELETE FROM error_log WHERE timestamp < ?`
  _, err := d.db.Exec(query, cutoff)
  return err
}

// SaveConfig saves a configuration value
func (d *Database) SaveConfig(key string, value interface{}) error {
  jsonValue, err := json.Marshal(value)
  if err != nil {
    return err
  }

  query := `
  INSERT OR REPLACE INTO config (key, value, updated_at)
  VALUES (?, ?, ?)
  `

  _, err = d.db.Exec(query, key, string(jsonValue), time.Now())
  return err
}

// LoadConfig loads a configuration value
func (d *Database) LoadConfig(key string, dest interface{}) error {
  query := `SELECT value FROM config WHERE key = ?`

  var jsonValue string
  err := d.db.QueryRow(query, key).Scan(&jsonValue)
  if err != nil {
    if err == sql.ErrNoRows {
      return nil // Not found, not an error
    }
    return err
  }

  return json.Unmarshal([]byte(jsonValue), dest)
}

// LogConnectionEvent logs a connection event
func (d *Database) LogConnectionEvent(eventType string, details string) error {
  query := `
  INSERT INTO connection_log (timestamp, event_type, details)
  VALUES (?, ?, ?)
  `

  _, err := d.db.Exec(query, time.Now(), eventType, details)
  return err
}

// SaveMessage saves a received message to the database
func (d *Database) SaveMessage(msg map[string]interface{}) error {
  query := `
  INSERT OR REPLACE INTO messages (
    message_id, timestamp, from_jid, chat_jid, sender_name,
    is_group, is_from_me, message_type, text_content,
    media_type, media_mime_type, media_size, quoted_message_id, raw_message
  ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `

  rawJSON, _ := json.Marshal(msg)

  _, err := d.db.Exec(query,
    msg["message_id"],
    msg["timestamp"],
    msg["from"],
    msg["chat"],
    msg["sender_name"],
    msg["is_group"],
    msg["is_from_me"],
    msg["message_type"],
    msg["text_content"],
    msg["media_type"],
    msg["media_mime_type"],
    msg["media_size"],
    msg["quoted_message_id"],
    string(rawJSON),
  )

  return err
}

// GetMessages retrieves messages from the database
func (d *Database) GetMessages(limit int, fromJID *string, chatJID *string, sinceTime *time.Time) ([]map[string]interface{}, error) {
  query := `
  SELECT message_id, timestamp, from_jid, chat_jid, sender_name,
         is_group, is_from_me, message_type, text_content,
         media_type, media_mime_type, media_size, quoted_message_id
  FROM messages
  WHERE 1=1
  `
  args := []interface{}{}

  if fromJID != nil {
    query += ` AND from_jid = ?`
    args = append(args, *fromJID)
  }

  if chatJID != nil {
    query += ` AND chat_jid = ?`
    args = append(args, *chatJID)
  }

  if sinceTime != nil {
    query += ` AND timestamp > ?`
    args = append(args, *sinceTime)
  }

  query += ` ORDER BY timestamp DESC LIMIT ?`
  args = append(args, limit)

  rows, err := d.db.Query(query, args...)
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  var messages []map[string]interface{}
  for rows.Next() {
    var messageID, fromJID, chatJID, senderName, messageType string
    var textContent, mediaType, mediaMimeType, quotedMessageID sql.NullString
    var mediaSize sql.NullInt64
    var timestamp time.Time
    var isGroup, isFromMe bool

    err := rows.Scan(
      &messageID, &timestamp, &fromJID, &chatJID, &senderName,
      &isGroup, &isFromMe, &messageType, &textContent,
      &mediaType, &mediaMimeType, &mediaSize, &quotedMessageID,
    )
    if err != nil {
      return nil, err
    }

    msg := map[string]interface{}{
      "message_id":  messageID,
      "timestamp":   timestamp.Format(time.RFC3339),
      "from":        fromJID,
      "chat":        chatJID,
      "sender_name": senderName,
      "is_group":    isGroup,
      "is_from_me":  isFromMe,
      "message_type": messageType,
    }

    if textContent.Valid {
      msg["text_content"] = textContent.String
    }
    if mediaType.Valid {
      msg["media_type"] = mediaType.String
    }
    if mediaMimeType.Valid {
      msg["media_mime_type"] = mediaMimeType.String
    }
    if mediaSize.Valid {
      msg["media_size"] = mediaSize.Int64
    }
    if quotedMessageID.Valid {
      msg["quoted_message_id"] = quotedMessageID.String
    }

    messages = append(messages, msg)
  }

  return messages, rows.Err()
}

// SaveHandler saves an event handler to the database
func (d *Database) SaveHandler(handler map[string]interface{}) error {
  query := `
  INSERT OR REPLACE INTO event_handlers (
    handler_id, description, event_filter, action, enabled, priority,
    max_executions_per_minute, max_executions_per_hour, max_executions_per_sender_per_hour,
    cooldown_seconds, timeout_seconds,
    circuit_breaker_enabled, circuit_breaker_threshold, circuit_breaker_reset_seconds,
    updated_at
  ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `

  filterJSON, _ := json.Marshal(handler["event_filter"])
  actionJSON, _ := json.Marshal(handler["action"])

  enabled := 1
  if e, ok := handler["enabled"].(bool); ok && !e {
    enabled = 0
  }

  // Handle circuit breaker fields with defaults
  cbEnabled := 1
  if cb, ok := handler["circuit_breaker_enabled"].(bool); ok && !cb {
    cbEnabled = 0
  } else if cb, ok := handler["circuit_breaker_enabled"].(int); ok {
    cbEnabled = cb
  } else if cb, ok := handler["circuit_breaker_enabled"].(float64); ok {
    if cb == 0 {
      cbEnabled = 0
    }
  }

  cbThreshold := 5 // default
  if t, ok := handler["circuit_breaker_threshold"].(int); ok {
    cbThreshold = t
  } else if t, ok := handler["circuit_breaker_threshold"].(int64); ok {
    cbThreshold = int(t)
  } else if t, ok := handler["circuit_breaker_threshold"].(float64); ok {
    cbThreshold = int(t)
  }

  cbReset := 300 // default 5 minutes
  if r, ok := handler["circuit_breaker_reset_seconds"].(int); ok {
    cbReset = r
  } else if r, ok := handler["circuit_breaker_reset_seconds"].(int64); ok {
    cbReset = int(r)
  } else if r, ok := handler["circuit_breaker_reset_seconds"].(float64); ok {
    cbReset = int(r)
  }

  _, err := d.db.Exec(query,
    handler["handler_id"],
    handler["description"],
    string(filterJSON),
    string(actionJSON),
    enabled,
    handler["priority"],
    handler["max_executions_per_minute"],
    handler["max_executions_per_hour"],
    handler["max_executions_per_sender_per_hour"],
    handler["cooldown_seconds"],
    handler["timeout_seconds"],
    cbEnabled,
    cbThreshold,
    cbReset,
    time.Now(),
  )

  return err
}

// GetHandler retrieves a specific event handler
func (d *Database) GetHandler(handlerID string) (map[string]interface{}, error) {
  query := `
  SELECT handler_id, description, event_filter, action, enabled, priority,
         max_executions_per_minute, max_executions_per_hour, max_executions_per_sender_per_hour,
         cooldown_seconds, timeout_seconds,
         circuit_breaker_enabled, circuit_breaker_threshold, circuit_breaker_reset_seconds,
         created_at, updated_at, execution_count, last_executed,
         last_error, last_error_time, total_errors, circuit_breaker_state
  FROM event_handlers
  WHERE handler_id = ?
  `

  var handler map[string]interface{}
  var filterJSON, actionJSON string
  var enabled, priority, cbEnabled int
  var maxPerMin, maxPerHour, maxPerSenderHour, cooldown, timeout, cbThreshold, cbReset sql.NullInt64
  var createdAt, updatedAt time.Time
  var executionCount, totalErrors int
  var lastExecuted, lastErrorTime sql.NullTime
  var lastError, cbState sql.NullString
  var description sql.NullString

  err := d.db.QueryRow(query, handlerID).Scan(
    &handlerID, &description, &filterJSON, &actionJSON, &enabled, &priority,
    &maxPerMin, &maxPerHour, &maxPerSenderHour, &cooldown, &timeout,
    &cbEnabled, &cbThreshold, &cbReset,
    &createdAt, &updatedAt, &executionCount, &lastExecuted,
    &lastError, &lastErrorTime, &totalErrors, &cbState,
  )

  if err != nil {
    return nil, err
  }

  var eventFilter map[string]interface{}
  var action map[string]interface{}
  json.Unmarshal([]byte(filterJSON), &eventFilter)
  json.Unmarshal([]byte(actionJSON), &action)

  handler = map[string]interface{}{
    "handler_id":    handlerID,
    "event_filter":  eventFilter,
    "action":        action,
    "enabled":       enabled == 1,
    "priority":      priority,
    "created_at":    createdAt.Format(time.RFC3339),
    "updated_at":    updatedAt.Format(time.RFC3339),
    "execution_count": executionCount,
    "total_errors":   totalErrors,
    "circuit_breaker_state": "closed",
  }

  if description.Valid {
    handler["description"] = description.String
  }
  if maxPerMin.Valid {
    handler["max_executions_per_minute"] = maxPerMin.Int64
  }
  if maxPerHour.Valid {
    handler["max_executions_per_hour"] = maxPerHour.Int64
  }
  if maxPerSenderHour.Valid {
    handler["max_executions_per_sender_per_hour"] = maxPerSenderHour.Int64
  }
  if cooldown.Valid {
    handler["cooldown_seconds"] = cooldown.Int64
  }
  if timeout.Valid {
    handler["timeout_seconds"] = timeout.Int64
  }
  if cbEnabled == 1 {
    handler["circuit_breaker_enabled"] = true
    if cbThreshold.Valid {
      handler["circuit_breaker_threshold"] = cbThreshold.Int64
    }
    if cbReset.Valid {
      handler["circuit_breaker_reset_seconds"] = cbReset.Int64
    }
  }
  if lastExecuted.Valid {
    handler["last_executed"] = lastExecuted.Time.Format(time.RFC3339)
  }
  if lastError.Valid {
    handler["last_error"] = lastError.String
  }
  if lastErrorTime.Valid {
    handler["last_error_time"] = lastErrorTime.Time.Format(time.RFC3339)
  }
  if cbState.Valid {
    handler["circuit_breaker_state"] = cbState.String
  }

  return handler, nil
}

// ListHandlers retrieves all event handlers
func (d *Database) ListHandlers(enabledOnly bool) ([]map[string]interface{}, error) {
  query := `
  SELECT handler_id, description, enabled, priority, execution_count, last_executed, circuit_breaker_state
  FROM event_handlers
  `
  args := []interface{}{}

  if enabledOnly {
    query += ` WHERE enabled = 1`
  }

  query += ` ORDER BY priority DESC, handler_id`

  rows, err := d.db.Query(query, args...)
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  var handlers []map[string]interface{}
  for rows.Next() {
    var handlerID string
    var description sql.NullString
    var enabled, priority, executionCount int
    var lastExecuted sql.NullTime
    var cbState sql.NullString

    err := rows.Scan(&handlerID, &description, &enabled, &priority, &executionCount, &lastExecuted, &cbState)
    if err != nil {
      return nil, err
    }

    handler := map[string]interface{}{
      "handler_id":      handlerID,
      "enabled":         enabled == 1,
      "priority":        priority,
      "execution_count": executionCount,
    }

    if description.Valid {
      handler["description"] = description.String
    }
    if lastExecuted.Valid {
      handler["last_executed"] = lastExecuted.Time.Format(time.RFC3339)
    }
    if cbState.Valid {
      handler["circuit_breaker_state"] = cbState.String
    }

    handlers = append(handlers, handler)
  }

  return handlers, rows.Err()
}

// DeleteHandler deletes an event handler
func (d *Database) DeleteHandler(handlerID string) error {
  query := `DELETE FROM event_handlers WHERE handler_id = ?`
  _, err := d.db.Exec(query, handlerID)
  return err
}

// UpdateHandlerEnabled enables or disables a handler
func (d *Database) UpdateHandlerEnabled(handlerID string, enabled bool) error {
  enabledInt := 0
  if enabled {
    enabledInt = 1
  }

  query := `UPDATE event_handlers SET enabled = ?, updated_at = ? WHERE handler_id = ?`
  _, err := d.db.Exec(query, enabledInt, time.Now(), handlerID)
  return err
}

// UpdateHandlerStats updates handler execution statistics
func (d *Database) UpdateHandlerStats(handlerID string, success bool, errorMsg string) error {
  if success {
    query := `
    UPDATE event_handlers 
    SET execution_count = execution_count + 1,
        last_executed = ?,
        updated_at = ?
    WHERE handler_id = ?
    `
    _, err := d.db.Exec(query, time.Now(), time.Now(), handlerID)
    return err
  } else {
    query := `
    UPDATE event_handlers 
    SET execution_count = execution_count + 1,
        total_errors = total_errors + 1,
        last_executed = ?,
        last_error = ?,
        last_error_time = ?,
        updated_at = ?
    WHERE handler_id = ?
    `
    now := time.Now()
    _, err := d.db.Exec(query, now, errorMsg, now, now, handlerID)
    return err
  }
}

// LogHandlerExecution logs a handler execution
func (d *Database) LogHandlerExecution(execution map[string]interface{}) error {
  query := `
  INSERT INTO handler_executions (
    handler_id, event_id, event_type, from_jid,
    started_at, completed_at, duration_ms, success, error, actions_executed
  ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `

  success := 0
  if s, ok := execution["success"].(bool); ok && s {
    success = 1
  }

  _, err := d.db.Exec(query,
    execution["handler_id"],
    execution["event_id"],
    execution["event_type"],
    execution["from_jid"],
    execution["started_at"],
    execution["completed_at"],
    execution["duration_ms"],
    success,
    execution["error"],
    execution["actions_executed"],
  )

  return err
}

// GetHandlerExecutions retrieves recent handler executions
func (d *Database) GetHandlerExecutions(handlerID *string, limit int) ([]map[string]interface{}, error) {
  query := `
  SELECT id, handler_id, event_id, event_type, from_jid,
         started_at, completed_at, duration_ms, success, error, actions_executed
  FROM handler_executions
  WHERE 1=1
  `
  args := []interface{}{}

  if handlerID != nil {
    query += ` AND handler_id = ?`
    args = append(args, *handlerID)
  }

  query += ` ORDER BY started_at DESC LIMIT ?`
  args = append(args, limit)

  rows, err := d.db.Query(query, args...)
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  var executions []map[string]interface{}
  for rows.Next() {
    var id int
    var handlerID, eventID, eventType string
    var fromJID sql.NullString
    var startedAt, completedAt time.Time
    var durationMs int
    var success int
    var errorMsg sql.NullString
    var actionsExecuted sql.NullInt64

    err := rows.Scan(&id, &handlerID, &eventID, &eventType, &fromJID,
      &startedAt, &completedAt, &durationMs, &success, &errorMsg, &actionsExecuted)
    if err != nil {
      return nil, err
    }

    exec := map[string]interface{}{
      "id":          id,
      "handler_id":  handlerID,
      "event_id":    eventID,
      "event_type":  eventType,
      "started_at":  startedAt.Format(time.RFC3339),
      "completed_at": completedAt.Format(time.RFC3339),
      "duration_ms": durationMs,
      "success":     success == 1,
    }

    if fromJID.Valid {
      exec["from_jid"] = fromJID.String
    }
    if errorMsg.Valid {
      exec["error"] = errorMsg.String
    }
    if actionsExecuted.Valid {
      exec["actions_executed"] = actionsExecuted.Int64
    }

    executions = append(executions, exec)
  }

  return executions, rows.Err()
}

// Close closes the database connection
func (d *Database) Close() error {
  return d.db.Close()
}

