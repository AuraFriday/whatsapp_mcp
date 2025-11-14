package main

import (
  "os"
  "path/filepath"
)

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
  // Default to user data directory
  userDataPath := filepath.Join(os.Getenv("APPDATA"), "AuraFriday", "user_data")
  
  return &Config{
    database_path:         filepath.Join(userDataPath, "whatsapp_session.db"),
    handlers_database_path: filepath.Join(userDataPath, "whatsapp_handlers.db"),
    media_download_path:   filepath.Join(userDataPath, "whatsapp_media"),
    log_level:             "info",
    log_file:              filepath.Join(userDataPath, "whatsapp_mcp.log"),
    auto_reconnect:        true,
    auto_read_receipts:    false,
    auto_presence:         true,
    handler_timeout:       30,
    max_parallel_handlers: 10,
  }
}

// GetDatabasePath returns the database path
func (c *Config) GetDatabasePath() string {
  c.mu.RLock()
  defer c.mu.RUnlock()
  return c.database_path
}

// SetDatabasePath sets the database path
func (c *Config) SetDatabasePath(path string) {
  c.mu.Lock()
  defer c.mu.Unlock()
  c.database_path = path
}

// GetHandlersDatabasePath returns the handlers database path
func (c *Config) GetHandlersDatabasePath() string {
  c.mu.RLock()
  defer c.mu.RUnlock()
  return c.handlers_database_path
}

// SetHandlersDatabasePath sets the handlers database path
func (c *Config) SetHandlersDatabasePath(path string) {
  c.mu.Lock()
  defer c.mu.Unlock()
  c.handlers_database_path = path
}

// GetMediaDownloadPath returns the media download path
func (c *Config) GetMediaDownloadPath() string {
  c.mu.RLock()
  defer c.mu.RUnlock()
  return c.media_download_path
}

// SetMediaDownloadPath sets the media download path
func (c *Config) SetMediaDownloadPath(path string) {
  c.mu.Lock()
  defer c.mu.Unlock()
  c.media_download_path = path
}

// GetLogLevel returns the log level
func (c *Config) GetLogLevel() string {
  c.mu.RLock()
  defer c.mu.RUnlock()
  return c.log_level
}

// SetLogLevel sets the log level
func (c *Config) SetLogLevel(level string) {
  c.mu.Lock()
  defer c.mu.Unlock()
  c.log_level = level
}

// GetAutoReconnect returns the auto reconnect setting
func (c *Config) GetAutoReconnect() bool {
  c.mu.RLock()
  defer c.mu.RUnlock()
  return c.auto_reconnect
}

// SetAutoReconnect sets the auto reconnect setting
func (c *Config) SetAutoReconnect(enabled bool) {
  c.mu.Lock()
  defer c.mu.Unlock()
  c.auto_reconnect = enabled
}

// ToMap converts the config to a map for JSON serialization
func (c *Config) ToMap() map[string]interface{} {
  c.mu.RLock()
  defer c.mu.RUnlock()

  return map[string]interface{}{
    "database_path":         c.database_path,
    "handlers_database_path": c.handlers_database_path,
    "media_download_path":   c.media_download_path,
    "log_level":             c.log_level,
    "log_file":              c.log_file,
    "auto_reconnect":        c.auto_reconnect,
    "auto_read_receipts":    c.auto_read_receipts,
    "auto_presence":         c.auto_presence,
    "handler_timeout":       c.handler_timeout,
    "max_parallel_handlers": c.max_parallel_handlers,
  }
}

// UpdateFromMap updates the config from a map
func (c *Config) UpdateFromMap(data map[string]interface{}) {
  c.mu.Lock()
  defer c.mu.Unlock()

  if val, ok := data["database_path"].(string); ok {
    c.database_path = val
  }
  if val, ok := data["handlers_database_path"].(string); ok {
    c.handlers_database_path = val
  }
  if val, ok := data["media_download_path"].(string); ok {
    c.media_download_path = val
  }
  if val, ok := data["log_level"].(string); ok {
    c.log_level = val
  }
  if val, ok := data["log_file"].(string); ok {
    c.log_file = val
  }
  if val, ok := data["auto_reconnect"].(bool); ok {
    c.auto_reconnect = val
  }
  if val, ok := data["auto_read_receipts"].(bool); ok {
    c.auto_read_receipts = val
  }
  if val, ok := data["auto_presence"].(bool); ok {
    c.auto_presence = val
  }
  if val, ok := data["handler_timeout"].(float64); ok {
    c.handler_timeout = int(val)
  }
  if val, ok := data["max_parallel_handlers"].(float64); ok {
    c.max_parallel_handlers = int(val)
  }
}

