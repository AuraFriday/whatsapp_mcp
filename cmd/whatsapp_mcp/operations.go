package main

import (
  "encoding/json"
  "fmt"
  "os"
  "strings"
  "time"
)

// OperationHandler handles all MCP operations
type OperationHandler struct {
  error_state  *ErrorState
  config       *Config
  whatsapp_state *WhatsAppState
  database     *Database
}

// NewOperationHandler creates a new operation handler
func NewOperationHandler(errorState *ErrorState, config *Config, whatsappState *WhatsAppState, database *Database) *OperationHandler {
  return &OperationHandler{
    error_state:  errorState,
    config:       config,
    whatsapp_state: whatsappState,
    database:     database,
  }
}

// HandleOperation handles an operation and returns the result
func (oh *OperationHandler) HandleOperation(input *OperationInput) *OperationResult {
  // Check for critical errors first (except for error management operations)
  if input.Operation != "get_error_log" && 
     input.Operation != "get_health_status" && 
     input.Operation != "clear_error_state" {
    if errorResult := oh.error_state.CheckErrorState(input.Operation); errorResult != nil {
      return errorResult
    }
  }

  // Route to specific operation handler
  switch input.Operation {
  case "get_error_log":
    return oh.handleGetErrorLog(input)
  case "get_health_status":
    return oh.handleGetHealthStatus(input)
  case "clear_error_state":
    return oh.handleClearErrorState(input)
  case "get_config":
    return oh.handleGetConfig(input)
  case "set_config":
    return oh.handleSetConfig(input)
  case "get_connection_info":
    return oh.handleGetConnectionInfo(input)
  case "get_qr_code":
    return oh.handleGetQRCode(input)
  case "check_login_status":
    return oh.handleCheckLoginStatus(input)
  case "logout":
    return oh.handleLogout(input)
  case "shutdown":
    return oh.handleShutdown(input)
  case "call_whatsmeow":
    return oh.handleCallWhatsmeow(input)
  case "get_method_registry":
    return oh.handleGetMethodRegistry(input)
  case "get_version":
    return oh.handleGetVersion(input)
  case "get_messages":
    return oh.handleGetMessages(input)

  // Handler operations
  case "register_handler":
    return oh.handleRegisterHandler(input)
  case "list_handlers":
    return oh.handleListHandlers(input)
  case "get_handler":
    return oh.handleGetHandler(input)
  case "update_handler":
    return oh.handleUpdateHandler(input)
  case "delete_handler":
    return oh.handleDeleteHandler(input)
  case "enable_handler":
    return oh.handleEnableHandler(input)
  case "disable_handler":
    return oh.handleDisableHandler(input)
  case "get_handler_executions":
    return oh.handleGetHandlerExecutions(input)
  case "reload_handlers":
    return oh.handleReloadHandlers(input)

  default:
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Unknown operation: %s", input.Operation),
    }
  }
}

// handleGetErrorLog handles the get_error_log operation
func (oh *OperationHandler) handleGetErrorLog(input *OperationInput) *OperationResult {
  // Parse parameters
  limit := 50 // default
  if limitVal, ok := input.Data["limit"].(float64); ok {
    limit = int(limitVal)
  }

  var severity *ErrorSeverity
  if severityStr, ok := input.Data["severity"].(string); ok {
    sev := ErrorSeverity(severityStr)
    severity = &sev
  }

  // Get errors from memory
  memoryErrors := oh.error_state.GetRecentErrors(severity, limit)

  // Get errors from database
  dbErrors, err := oh.database.GetRecentErrors(severity, limit)
  if err != nil {
    oh.error_state.LogError(ErrorSeverityWarning, "get_error_log", "Failed to retrieve errors from database", err.Error())
  }

  // Convert to JSON-friendly format
  var errorList []map[string]interface{}
  
  // Add memory errors first (most recent)
  for _, e := range memoryErrors {
    errorList = append(errorList, map[string]interface{}{
      "id":         e.ID,
      "timestamp":  e.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
      "severity":   e.Severity,
      "operation":  e.Operation,
      "message":    e.Message,
      "details":    e.Details,
      "source":     "memory",
    })
  }

  // Add database errors (if not already in memory)
  memoryIDs := make(map[string]bool)
  for _, e := range memoryErrors {
    memoryIDs[e.ID] = true
  }

  for _, e := range dbErrors {
    if !memoryIDs[e.ID] {
      errorList = append(errorList, map[string]interface{}{
        "id":        e.ID,
        "timestamp": e.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
        "severity":  e.Severity,
        "operation": e.Operation,
        "message":   e.Message,
        "details":   e.Details,
        "source":    "database",
      })
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Retrieved %d error(s)", len(errorList)),
    Data: map[string]interface{}{
      "errors": errorList,
      "count":  len(errorList),
    },
  }
}

// handleGetHealthStatus handles the get_health_status operation
func (oh *OperationHandler) handleGetHealthStatus(input *OperationInput) *OperationResult {
  criticalError := oh.error_state.GetCriticalError()
  recentErrors := oh.error_state.GetRecentErrors(nil, 10)

  // Count errors by severity
  errorCounts := map[ErrorSeverity]int{
    ErrorSeverityInfo:     0,
    ErrorSeverityWarning:  0,
    ErrorSeverityError:    0,
    ErrorSeverityCritical: 0,
  }

  for _, e := range recentErrors {
    errorCounts[e.Severity]++
  }

  health := "healthy"
  if criticalError != nil {
    health = "critical"
  } else if errorCounts[ErrorSeverityError] > 0 {
    health = "degraded"
  } else if errorCounts[ErrorSeverityWarning] > 3 {
    health = "warning"
  }

  data := map[string]interface{}{
    "health":        health,
    "has_critical_error": criticalError != nil,
    "error_counts": map[string]int{
      "info":     errorCounts[ErrorSeverityInfo],
      "warning":  errorCounts[ErrorSeverityWarning],
      "error":    errorCounts[ErrorSeverityError],
      "critical": errorCounts[ErrorSeverityCritical],
    },
    "connection_state": oh.whatsapp_state.GetConnectionState(),
  }

  if criticalError != nil {
    data["critical_error"] = map[string]interface{}{
      "id":        criticalError.ID,
      "timestamp": criticalError.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
      "operation": criticalError.Operation,
      "message":   criticalError.Message,
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("System health: %s", health),
    Data:    data,
  }
}

// handleClearErrorState handles the clear_error_state operation
func (oh *OperationHandler) handleClearErrorState(input *OperationInput) *OperationResult {
  clearCritical := false
  if val, ok := input.Data["clear_critical"].(bool); ok {
    clearCritical = val
  }

  if clearCritical {
    oh.error_state.ClearCriticalError()
  }

  oh.error_state.ClearRecentErrors()

  return &OperationResult{
    Success: true,
    Message: "Error state cleared",
    Data: map[string]interface{}{
      "cleared_critical": clearCritical,
    },
  }
}

// handleGetConfig handles the get_config operation
func (oh *OperationHandler) handleGetConfig(input *OperationInput) *OperationResult {
  return &OperationResult{
    Success: true,
    Message: "Current configuration",
    Data:    oh.config.ToMap(),
  }
}

// handleSetConfig handles the set_config operation
func (oh *OperationHandler) handleSetConfig(input *OperationInput) *OperationResult {
  if input.Data == nil {
    return &OperationResult{
      Success: false,
      Error:   "No configuration data provided",
    }
  }

  oh.config.UpdateFromMap(input.Data)

  // Save to database
  if err := oh.database.SaveConfig("app_config", oh.config.ToMap()); err != nil {
    oh.error_state.LogError(ErrorSeverityWarning, "set_config", "Failed to save config to database", err.Error())
  }

  return &OperationResult{
    Success: true,
    Message: "Configuration updated",
    Data:    oh.config.ToMap(),
  }
}

// handleGetConnectionInfo handles the get_connection_info operation
func (oh *OperationHandler) handleGetConnectionInfo(input *OperationInput) *OperationResult {
  state := oh.whatsapp_state.GetState()

  return &OperationResult{
    Success: true,
    Message: "Connection information",
    Data:    state,
  }
}

// Helper methods for WhatsAppState
func (ws *WhatsAppState) GetConnectionState() string {
  ws.mu.RLock()
  defer ws.mu.RUnlock()
  return string(ws.connection_state)
}

func (ws *WhatsAppState) GetState() map[string]interface{} {
  ws.mu.RLock()
  defer ws.mu.RUnlock()

  return map[string]interface{}{
    "connection_state":  string(ws.connection_state),
    "phone_number":      ws.phone_number,
    "device_id":         ws.device_id,
    "last_connected":    ws.last_connected.Format("2006-01-02T15:04:05Z07:00"),
    "last_disconnected": ws.last_disconnected.Format("2006-01-02T15:04:05Z07:00"),
    "reconnect_attempts": ws.reconnect_attempts,
  }
}

// handleGetQRCode handles the get_qr_code operation
func (oh *OperationHandler) handleGetQRCode(input *OperationInput) *OperationResult {
  if global_whatsapp_client == nil {
    return &OperationResult{
      Success: false,
      Error:   "WhatsApp client not initialized",
    }
  }

  if global_whatsapp_client.IsLoggedIn() {
    return &OperationResult{
      Success: false,
      Error:   "Already logged in. Use logout first if you want to pair a new device.",
    }
  }

  // Get timeout from parameters (default 60 seconds)
  timeout := 60
  if timeoutVal, ok := input.Data["timeout"].(float64); ok {
    timeout = int(timeoutVal)
  }

  // Get QR code
  qrText, qrBase64, err := global_whatsapp_client.GetQRCode(timeout)
  if err != nil {
    oh.error_state.LogError(ErrorSeverityError, "get_qr_code", "Failed to get QR code", err.Error())
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to get QR code: %v", err),
    }
  }

  // Generate ASCII QR for terminal
  asciiQR := generateASCIIQR(qrText)
  
  // Print ASCII QR to console
  fmt.Fprintln(os.Stderr, "\n"+strings.Repeat("=", 60))
  fmt.Fprintln(os.Stderr, "QR CODE - Scan with WhatsApp mobile app")
  fmt.Fprintln(os.Stderr, strings.Repeat("=", 60))
  fmt.Fprintln(os.Stderr, asciiQR)
  fmt.Fprintln(os.Stderr, strings.Repeat("=", 60))
  fmt.Fprintln(os.Stderr, "Instructions: Open WhatsApp > Settings > Linked Devices > Link a Device")
  fmt.Fprintf(os.Stderr, "Timeout: %d seconds\n", timeout)
  fmt.Fprintln(os.Stderr, strings.Repeat("=", 60)+"\n")
  
  // Show QR code popup using user MCP tool
  go showQRPopup(qrBase64, timeout)
  
  return &OperationResult{
    Success: true,
    Message: "QR code generated. Scan with WhatsApp mobile app.",
    Data: map[string]interface{}{
      "qr_code_text":   qrText,
      "qr_code_base64": qrBase64,
      "qr_code_ascii":  asciiQR,
      "timeout":        timeout,
      "instructions":   "Scan this QR code with your WhatsApp mobile app (Settings > Linked Devices > Link a Device)",
    },
  }
}

// handleCheckLoginStatus handles the check_login_status operation
func (oh *OperationHandler) handleCheckLoginStatus(input *OperationInput) *OperationResult {
  if global_whatsapp_client == nil {
    return &OperationResult{
      Success: false,
      Error:   "WhatsApp client not initialized",
    }
  }

  isLoggedIn := global_whatsapp_client.IsLoggedIn()
  isConnected := global_whatsapp_client.IsConnected()

  var phoneNumber, deviceID string
  if isLoggedIn {
    jid := global_whatsapp_client.GetJID()
    phoneNumber = jid.User
    deviceID = fmt.Sprintf("%d", jid.Device)
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Login status: %v, Connected: %v", isLoggedIn, isConnected),
    Data: map[string]interface{}{
      "is_logged_in": isLoggedIn,
      "is_connected": isConnected,
      "phone_number": phoneNumber,
      "device_id":    deviceID,
    },
  }
}

// handleLogout handles the logout operation
func (oh *OperationHandler) handleLogout(input *OperationInput) *OperationResult {
  if global_whatsapp_client == nil {
    return &OperationResult{
      Success: false,
      Error:   "WhatsApp client not initialized",
    }
  }

  if !global_whatsapp_client.IsLoggedIn() {
    return &OperationResult{
      Success: false,
      Error:   "Not logged in",
    }
  }

  err := global_whatsapp_client.Logout()
  if err != nil {
    oh.error_state.LogError(ErrorSeverityError, "logout", "Failed to logout", err.Error())
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to logout: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: "Logged out successfully",
  }
}

// handleShutdown handles the shutdown operation - gracefully shuts down the tool
func (oh *OperationHandler) handleShutdown(input *OperationInput) *OperationResult {
  fmt.Fprintln(os.Stderr, "[INFO] Shutdown requested by AI agent")
  
  // Log the shutdown
  oh.error_state.LogError(ErrorSeverityInfo, "shutdown", "Graceful shutdown initiated", "")
  
  // Disconnect WhatsApp client if connected
  if global_whatsapp_client != nil && global_whatsapp_client.IsConnected() {
    fmt.Fprintln(os.Stderr, "[INFO] Disconnecting WhatsApp client...")
    global_whatsapp_client.client.Disconnect()
  }
  
  // Close database
  if global_database != nil {
    fmt.Fprintln(os.Stderr, "[INFO] Closing database...")
    global_database.Close()
  }
  
  // Exit the process
  fmt.Fprintln(os.Stderr, "[INFO] Shutdown complete. Exiting.")
  go func() {
    time.Sleep(500 * time.Millisecond) // Give time for response to be sent
    os.Exit(0)
  }()
  
  return &OperationResult{
    Success: true,
    Message: "Shutdown initiated. Tool will exit in 500ms.",
  }
}

// showQRPopup shows a popup window with the QR code using the user MCP tool
func showQRPopup(qrBase64 string, timeout int) {
  if global_sse_connection == nil {
    fmt.Fprintln(os.Stderr, "[WARN] Cannot show QR popup: no SSE connection")
    return
  }

  html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>WhatsApp QR Code</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            padding: 15px 30px;
            background: linear-gradient(135deg, #25D366 0%%, #128C7E 100%%);
            margin: 0;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
        }
        .container {
            background: white;
            padding: 40px;
            border-radius: 16px;
            box-shadow: 0 8px 32px rgba(0,0,0,0.3);
            text-align: center;
            max-width: 500px;
        }
        h1 {
            color: #128C7E;
            margin-top: 0;
            margin-bottom: 10px;
            font-size: 28px;
        }
        .subtitle {
            color: #666;
            margin-bottom: 30px;
            font-size: 14px;
        }
        .qr-container {
            background: white;
            padding: 20px;
            border-radius: 12px;
            display: inline-block;
            box-shadow: 0 4px 12px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        .qr-code {
            width: 300px;
            height: 300px;
            image-rendering: pixelated;
        }
        .instructions {
            background: #f0f9ff;
            border-left: 4px solid #25D366;
            padding: 15px;
            margin: 20px 0;
            text-align: left;
            border-radius: 4px;
        }
        .instructions h3 {
            margin-top: 0;
            color: #128C7E;
            font-size: 16px;
        }
        .instructions ol {
            margin: 10px 0;
            padding-left: 20px;
        }
        .instructions li {
            margin: 8px 0;
            color: #333;
        }
        .timeout {
            color: #999;
            font-size: 12px;
            margin-top: 20px;
        }
        .close-btn {
            background: #25D366;
            color: white;
            border: none;
            padding: 12px 30px;
            border-radius: 8px;
            cursor: pointer;
            font-size: 16px;
            margin-top: 20px;
            transition: background 0.3s;
        }
        .close-btn:hover {
            background: #128C7E;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>📱 WhatsApp Pairing</h1>
        <p class="subtitle">Scan this QR code with your WhatsApp mobile app</p>
        
        <div class="qr-container">
            <img src="data:image/png;base64,%s" class="qr-code" alt="WhatsApp QR Code">
        </div>
        
        <div class="instructions">
            <h3>How to scan:</h3>
            <ol>
                <li>Open <strong>WhatsApp</strong> on your phone</li>
                <li>Tap <strong>Menu</strong> or <strong>Settings</strong></li>
                <li>Tap <strong>Linked Devices</strong></li>
                <li>Tap <strong>Link a Device</strong></li>
                <li>Point your phone at this screen to scan the QR code</li>
            </ol>
        </div>
        
        <p class="timeout">⏱️ This QR code will expire in %d seconds</p>
        
        <button class="close-btn" onclick="window.close()">Close</button>
    </div>
</body>
</html>`, qrBase64, timeout)

  arguments := map[string]interface{}{
    "input": map[string]interface{}{
      "operation":        "show_popup",
      "html":             html,
      "title":            "WhatsApp QR Code - Scan to Connect",
      "width":            600,
      "height":           954,
      "modal":            false,
      "wait_for_response": false,
      "center_on_screen": true,
      "always_on_top":    true,
      "bring_to_front":   false,
      "tool_unlock_token": "b3fa8eb3",
    },
  }

  fmt.Fprintln(os.Stderr, "[INFO] Showing QR code popup window...")
  _, err := callMCPTool(global_sse_connection, "user", arguments)
  if err != nil {
    fmt.Fprintf(os.Stderr, "[WARN] Failed to show QR popup: %v\n", err)
  } else {
    fmt.Fprintln(os.Stderr, "[OK] QR code popup displayed")
  }
}

// handleCallWhatsmeow handles the call_whatsmeow operation - generic dispatcher
func (oh *OperationHandler) handleCallWhatsmeow(input *OperationInput) *OperationResult {
  // Extract method name
  methodName, ok := input.Data["method"].(string)
  if !ok {
    return &OperationResult{
      Success: false,
      Error:   "method name required (string)",
    }
  }

  // Extract params
  params, ok := input.Data["params"].(map[string]interface{})
  if !ok {
    params = make(map[string]interface{})
  }

  // Call via dispatcher
  fmt.Fprintf(os.Stderr, "[INFO] Calling whatsmeow method: %s\n", methodName)
  result := CallWhatsmeowMethod(methodName, params)

  if !result.Success {
    oh.error_state.LogError(ErrorSeverityError, "call_whatsmeow", fmt.Sprintf("Method %s failed", methodName), result.Error)
  }

  return result
}

// handleGetMethodRegistry handles the get_method_registry operation
func (oh *OperationHandler) handleGetMethodRegistry(input *OperationInput) *OperationResult {
  if globalMethodRegistry == nil {
    return &OperationResult{
      Success: false,
      Error:   "Method registry not loaded",
    }
  }

  return &OperationResult{
    Success: true,
    Message: "Method registry retrieved",
    Data: map[string]interface{}{
      "methods":           globalMethodRegistry.Methods,
      "message_templates": globalMethodRegistry.MessageTemplates,
      "type_notes":        globalMethodRegistry.TypeNotes,
    },
  }
}

// handleGetVersion handles the get_version operation
func (oh *OperationHandler) handleGetVersion(input *OperationInput) *OperationResult {
  return &OperationResult{
    Success: true,
    Message: "Version information retrieved",
    Data:    GetVersionInfo(),
  }
}

// handleGetMessages handles the get_messages operation
func (oh *OperationHandler) handleGetMessages(input *OperationInput) *OperationResult {
  // Parse parameters
  limit := 50 // Default limit
  if input.Data != nil {
    if l, ok := input.Data["limit"].(float64); ok {
      limit = int(l)
    }
  }

  var fromJID *string
  if input.Data != nil {
    if f, ok := input.Data["from"].(string); ok && f != "" {
      fromJID = &f
    }
  }

  var chatJID *string
  if input.Data != nil {
    if c, ok := input.Data["chat"].(string); ok && c != "" {
      chatJID = &c
    }
  }

  var sinceTime *time.Time
  if input.Data != nil {
    if s, ok := input.Data["since"].(string); ok && s != "" {
      t, err := time.Parse(time.RFC3339, s)
      if err == nil {
        sinceTime = &t
      }
    }
  }

  // Get messages from database
  messages, err := oh.database.GetMessages(limit, fromJID, chatJID, sinceTime)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to retrieve messages: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Retrieved %d messages", len(messages)),
    Data: map[string]interface{}{
      "messages": messages,
      "count":    len(messages),
    },
  }
}

// handleRegisterHandler handles the register_handler operation
func (oh *OperationHandler) handleRegisterHandler(input *OperationInput) *OperationResult {
  if input.Data == nil {
    return &OperationResult{
      Success: false,
      Error:   "Missing handler data",
    }
  }

  // Validate required fields
  handlerID, ok := input.Data["handler_id"].(string)
  if !ok || handlerID == "" {
    return &OperationResult{
      Success: false,
      Error:   "Missing or invalid handler_id",
    }
  }

  if _, ok := input.Data["event_filter"]; !ok {
    return &OperationResult{
      Success: false,
      Error:   "Missing event_filter",
    }
  }

  if _, ok := input.Data["action"]; !ok {
    return &OperationResult{
      Success: false,
      Error:   "Missing action",
    }
  }

  // Set defaults
  if _, ok := input.Data["enabled"]; !ok {
    input.Data["enabled"] = true
  }
  if _, ok := input.Data["priority"]; !ok {
    input.Data["priority"] = 0
  }
  if _, ok := input.Data["timeout_seconds"]; !ok {
    input.Data["timeout_seconds"] = 30
  }

  // Save to database
  err := oh.database.SaveHandler(input.Data)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to save handler: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Handler '%s' registered successfully", handlerID),
    Data: map[string]interface{}{
      "handler_id": handlerID,
    },
  }
}

// handleListHandlers handles the list_handlers operation
func (oh *OperationHandler) handleListHandlers(input *OperationInput) *OperationResult {
  enabledOnly := false
  if input.Data != nil {
    if e, ok := input.Data["enabled_only"].(bool); ok {
      enabledOnly = e
    }
  }

  handlers, err := oh.database.ListHandlers(enabledOnly)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to retrieve handlers: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Retrieved %d handlers", len(handlers)),
    Data: map[string]interface{}{
      "handlers": handlers,
      "count":    len(handlers),
    },
  }
}

// handleGetHandler handles the get_handler operation
func (oh *OperationHandler) handleGetHandler(input *OperationInput) *OperationResult {
  if input.Data == nil {
    return &OperationResult{
      Success: false,
      Error:   "Missing handler_id",
    }
  }

  handlerID, ok := input.Data["handler_id"].(string)
  if !ok || handlerID == "" {
    return &OperationResult{
      Success: false,
      Error:   "Missing or invalid handler_id",
    }
  }

  handler, err := oh.database.GetHandler(handlerID)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to retrieve handler: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Retrieved handler '%s'", handlerID),
    Data:    handler,
  }
}

// handleUpdateHandler handles the update_handler operation
func (oh *OperationHandler) handleUpdateHandler(input *OperationInput) *OperationResult {
  if input.Data == nil {
    return &OperationResult{
      Success: false,
      Error:   "Missing handler data",
    }
  }

  handlerID, ok := input.Data["handler_id"].(string)
  if !ok || handlerID == "" {
    return &OperationResult{
      Success: false,
      Error:   "Missing or invalid handler_id",
    }
  }

  // Check if handler exists
  existing, err := oh.database.GetHandler(handlerID)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Handler not found: %v", err),
    }
  }

  // Merge updates into existing handler
  for key, value := range input.Data {
    existing[key] = value
  }

  // Save updated handler
  err = oh.database.SaveHandler(existing)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to update handler: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Handler '%s' updated successfully", handlerID),
    Data: map[string]interface{}{
      "handler_id": handlerID,
    },
  }
}

// handleDeleteHandler handles the delete_handler operation
func (oh *OperationHandler) handleDeleteHandler(input *OperationInput) *OperationResult {
  if input.Data == nil {
    return &OperationResult{
      Success: false,
      Error:   "Missing handler_id",
    }
  }

  handlerID, ok := input.Data["handler_id"].(string)
  if !ok || handlerID == "" {
    return &OperationResult{
      Success: false,
      Error:   "Missing or invalid handler_id",
    }
  }

  err := oh.database.DeleteHandler(handlerID)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to delete handler: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Handler '%s' deleted successfully", handlerID),
    Data: map[string]interface{}{
      "handler_id": handlerID,
    },
  }
}

// handleEnableHandler handles the enable_handler operation
func (oh *OperationHandler) handleEnableHandler(input *OperationInput) *OperationResult {
  if input.Data == nil {
    return &OperationResult{
      Success: false,
      Error:   "Missing handler_id",
    }
  }

  handlerID, ok := input.Data["handler_id"].(string)
  if !ok || handlerID == "" {
    return &OperationResult{
      Success: false,
      Error:   "Missing or invalid handler_id",
    }
  }

  err := oh.database.UpdateHandlerEnabled(handlerID, true)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to enable handler: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Handler '%s' enabled", handlerID),
    Data: map[string]interface{}{
      "handler_id": handlerID,
      "enabled":    true,
    },
  }
}

// handleDisableHandler handles the disable_handler operation
func (oh *OperationHandler) handleDisableHandler(input *OperationInput) *OperationResult {
  if input.Data == nil {
    return &OperationResult{
      Success: false,
      Error:   "Missing handler_id",
    }
  }

  handlerID, ok := input.Data["handler_id"].(string)
  if !ok || handlerID == "" {
    return &OperationResult{
      Success: false,
      Error:   "Missing or invalid handler_id",
    }
  }

  err := oh.database.UpdateHandlerEnabled(handlerID, false)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to disable handler: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Handler '%s' disabled", handlerID),
    Data: map[string]interface{}{
      "handler_id": handlerID,
      "enabled":    false,
    },
  }
}

// handleGetHandlerExecutions handles the get_handler_executions operation
func (oh *OperationHandler) handleGetHandlerExecutions(input *OperationInput) *OperationResult {
  limit := 50 // default
  var handlerID *string

  if input.Data != nil {
    if l, ok := input.Data["limit"].(float64); ok {
      limit = int(l)
    }
    if h, ok := input.Data["handler_id"].(string); ok && h != "" {
      handlerID = &h
    }
  }

  executions, err := oh.database.GetHandlerExecutions(handlerID, limit)
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to retrieve executions: %v", err),
    }
  }

  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Retrieved %d executions", len(executions)),
    Data: map[string]interface{}{
      "executions": executions,
      "count":      len(executions),
    },
  }
}

// handleReloadHandlers handles the reload_handlers operation
func (oh *OperationHandler) handleReloadHandlers(input *OperationInput) *OperationResult {
  if global_event_matcher == nil {
    return &OperationResult{
      Success: false,
      Error:   "Event matcher not initialized",
    }
  }

  err := global_event_matcher.LoadHandlers()
  if err != nil {
    return &OperationResult{
      Success: false,
      Error:   fmt.Sprintf("Failed to reload handlers: %v", err),
    }
  }

  count := len(global_event_matcher.handlers)
  return &OperationResult{
    Success: true,
    Message: fmt.Sprintf("Reloaded %d handlers", count),
    Data: map[string]interface{}{
      "count": count,
    },
  }
}

// FormatOperationResult formats an operation result as JSON
func FormatOperationResult(result *OperationResult) (string, error) {
  jsonBytes, err := json.MarshalIndent(result, "", "  ")
  if err != nil {
    return "", err
  }
  return string(jsonBytes), nil
}

