package main

import (
  "encoding/json"
  "fmt"
  "os"
  "path/filepath"
  "time"
)

// ActionExecutor handles execution of handler actions
type ActionExecutor struct {
  database     *Database
  errorState   *ErrorState
  eventMatcher *EventMatcher
}

// NewActionExecutor creates a new action executor
func NewActionExecutor(database *Database, errorState *ErrorState, eventMatcher *EventMatcher) *ActionExecutor {
  return &ActionExecutor{
    database:     database,
    errorState:   errorState,
    eventMatcher: eventMatcher,
  }
}

// ExecuteHandlersForEvent finds and executes all matching handlers for an event
func (ae *ActionExecutor) ExecuteHandlersForEvent(event map[string]interface{}) {
  // Find matching handlers
  matchingHandlers := ae.eventMatcher.MatchEvent(event)

  if len(matchingHandlers) == 0 {
    return // No handlers match
  }

  // Log matched handlers
  ae.errorState.LogError(ErrorSeverityInfo, "event_executor", 
    fmt.Sprintf("Event matched %d handlers", len(matchingHandlers)), "")

  // Execute each handler in a goroutine (non-blocking)
  for _, handler := range matchingHandlers {
    go ae.executeHandler(handler, event)
  }
}

// executeHandler executes a single handler for an event
func (ae *ActionExecutor) executeHandler(handler map[string]interface{}, event map[string]interface{}) {
  handlerID := handler["handler_id"].(string)
  startTime := time.Now()

  // Record execution start
  ae.eventMatcher.RecordExecution(handlerID, event)

  // Get timeout
  timeout := 30 // default
  if t, ok := handler["timeout_seconds"].(int64); ok && t > 0 {
    timeout = int(t)
  }

  // Prepare event data for handler
  eventData := ae.prepareEventData(event)

  // Get action definition
  action, ok := handler["action"].(map[string]interface{})
  if !ok {
    ae.logExecutionError(handlerID, event, startTime, "Invalid action definition")
    return
  }

  // Execute based on action type
  actionType, _ := action["type"].(string)
  var result map[string]interface{}
  var err error

  switch actionType {
  case "python":
    result, err = ae.executePythonAction(action, eventData, timeout)
  case "actions":
    result, err = ae.executeDirectActions(action, eventData)
  default:
    err = fmt.Errorf("unknown action type: %s", actionType)
  }

  if err != nil {
    ae.logExecutionError(handlerID, event, startTime, err.Error())
    ae.eventMatcher.UpdateCircuitBreaker(handlerID, false)
    ae.database.UpdateHandlerStats(handlerID, false, err.Error())
    return
  }

  // Check if handler returned success
  success, _ := result["success"].(bool)
  if !success {
    errorMsg, _ := result["error"].(string)
    ae.logExecutionError(handlerID, event, startTime, errorMsg)
    ae.eventMatcher.UpdateCircuitBreaker(handlerID, false)
    ae.database.UpdateHandlerStats(handlerID, false, errorMsg)
    return
  }

  // Execute returned actions
  actionsExecuted := 0
  if actions, ok := result["actions"].([]interface{}); ok {
    actionsExecuted = ae.executeReturnedActions(actions, eventData)
  }

  // Log success
  duration := time.Since(startTime).Milliseconds()
  ae.logExecutionSuccess(handlerID, event, startTime, duration, actionsExecuted)
  ae.eventMatcher.UpdateCircuitBreaker(handlerID, true)
  ae.database.UpdateHandlerStats(handlerID, true, "")
}

// prepareEventData prepares event data for handler execution
func (ae *ActionExecutor) prepareEventData(event map[string]interface{}) map[string]interface{} {
  eventData := make(map[string]interface{})
  
  // Copy all event fields
  for key, value := range event {
    eventData[key] = value
  }

  // Download media if present
  if mediaType, ok := event["media_type"].(string); ok && mediaType != "" {
    mediaPath, err := ae.downloadMedia(event)
    if err == nil && mediaPath != "" {
      eventData["media_path"] = mediaPath
      eventData["has_media"] = true
    }
  }

  return eventData
}

// downloadMedia downloads media from an event
func (ae *ActionExecutor) downloadMedia(event map[string]interface{}) (string, error) {
  // Check if we have a message ID
  messageID, ok := event["message_id"].(string)
  if !ok || messageID == "" {
    return "", fmt.Errorf("no message ID")
  }

  mediaType, _ := event["media_type"].(string)
  if mediaType == "" {
    return "", fmt.Errorf("no media type")
  }

  // Create temp directory
  tempDir := filepath.Join(os.TempDir(), "whatsapp_media")
  os.MkdirAll(tempDir, 0755)

  // Generate filename
  ext := ""
  switch mediaType {
  case "image":
    ext = ".jpg"
  case "video":
    ext = ".mp4"
  case "audio":
    ext = ".ogg"
  case "document":
    ext = ".bin"
  }
  
  filename := fmt.Sprintf("%s_%s%s", messageID, mediaType, ext)
  filePath := filepath.Join(tempDir, filename)

  // Check if already downloaded
  if _, err := os.Stat(filePath); err == nil {
    return filePath, nil
  }

  // Use WhatsApp client to download
  if global_whatsapp_client == nil || global_whatsapp_client.client == nil {
    return "", fmt.Errorf("WhatsApp client not available")
  }

  // Call DownloadMediaWithPath via dispatcher
  params := map[string]interface{}{
    "message":     event["raw_message"], // Full message protobuf
    "path":        filePath,
  }

  result := CallWhatsmeowMethod("DownloadMediaWithPath", params)
  if result == nil || !result.Success {
    errMsg := "failed to download media"
    if result != nil && result.Error != "" {
      errMsg = result.Error
    }
    return "", fmt.Errorf(errMsg)
  }

  return filePath, nil
}

// executePythonAction executes a Python action
func (ae *ActionExecutor) executePythonAction(action map[string]interface{}, eventData map[string]interface{}, timeout int) (map[string]interface{}, error) {
  code, ok := action["code"].(string)
  if !ok || code == "" {
    return nil, fmt.Errorf("missing Python code")
  }

  // Prepare variables
  variables := map[string]interface{}{
    "event": eventData,
  }

  // Add any custom variables from action
  if actionVars, ok := action["variables"].(map[string]interface{}); ok {
    for key, value := range actionVars {
      variables[key] = value
    }
  }

  // Build Python code with event data
  pythonCode := fmt.Sprintf(`
import json
import sys

# Event data
event = %s

# User code
%s
`, toJSON(eventData), code)

  // Call Python MCP tool
  pythonInput := map[string]interface{}{
    "input": map[string]interface{}{
      "operation":         "execute",
      "code":              pythonCode,
      "tool_unlock_token": "d2e9e014",
    },
  }

  if global_sse_connection == nil {
    return nil, fmt.Errorf("MCP connection not available")
  }

  rawResult, err := callMCPTool(global_sse_connection, "python", pythonInput)
  if err != nil {
    return nil, fmt.Errorf("Python tool call failed: %w", err)
  }

  // Parse result from JSON
  var resultMap map[string]interface{}
  if err := json.Unmarshal(rawResult, &resultMap); err != nil {
    return nil, fmt.Errorf("failed to parse Python result: %w", err)
  }

  // Check if Python execution succeeded
  if success, ok := resultMap["success"].(bool); ok && !success {
    errorMsg, _ := resultMap["error"].(string)
    return nil, fmt.Errorf("Python execution failed: %s", errorMsg)
  }

  // Try to parse output as JSON (handler return value)
  if output, ok := resultMap["output"].(string); ok && output != "" {
    var handlerResult map[string]interface{}
    if err := json.Unmarshal([]byte(output), &handlerResult); err == nil {
      return handlerResult, nil
    }
    // If not JSON, treat as plain output
    return map[string]interface{}{
      "success": true,
      "output":  output,
    }, nil
  }

  return resultMap, nil
}

// executeDirectActions executes direct actions (no Python)
func (ae *ActionExecutor) executeDirectActions(action map[string]interface{}, eventData map[string]interface{}) (map[string]interface{}, error) {
  actions, ok := action["actions"].([]interface{})
  if !ok {
    return nil, fmt.Errorf("missing actions array")
  }

  executed := ae.executeReturnedActions(actions, eventData)

  return map[string]interface{}{
    "success":          true,
    "actions_executed": executed,
  }, nil
}

// executeReturnedActions executes the actions returned by a handler
func (ae *ActionExecutor) executeReturnedActions(actions []interface{}, eventData map[string]interface{}) int {
  executed := 0

  for _, action := range actions {
    actionMap, ok := action.(map[string]interface{})
    if !ok {
      continue
    }

    // Substitute variables in action
    actionMap = ae.substituteVariables(actionMap, eventData)

    actionType, _ := actionMap["type"].(string)
    
    switch actionType {
    case "send_message":
      if ae.executeSendMessage(actionMap) {
        executed++
      }
    case "send_reaction":
      if ae.executeSendReaction(actionMap) {
        executed++
      }
    case "mark_read":
      if ae.executeMarkRead(actionMap) {
        executed++
      }
    case "send_presence":
      if ae.executeSendPresence(actionMap) {
        executed++
      }
    case "send_chat_presence":
      if ae.executeSendChatPresence(actionMap) {
        executed++
      }
    case "delay":
      if ae.executeDelay(actionMap) {
        executed++
      }
    case "call_method":
      if ae.executeCallMethod(actionMap) {
        executed++
      }
    }
  }

  return executed
}

// substituteVariables replaces variables in action with event data
func (ae *ActionExecutor) substituteVariables(action map[string]interface{}, eventData map[string]interface{}) map[string]interface{} {
  result := make(map[string]interface{})

  for key, value := range action {
    result[key] = ae.substituteValue(value, eventData)
  }

  return result
}

// substituteValue recursively substitutes variables in a value
func (ae *ActionExecutor) substituteValue(value interface{}, eventData map[string]interface{}) interface{} {
  switch v := value.(type) {
  case string:
    // Replace {event.field} with actual values
    if len(v) > 7 && v[:7] == "{event." && v[len(v)-1:] == "}" {
      fieldName := v[7 : len(v)-1]
      if fieldValue, ok := eventData[fieldName]; ok {
        return fieldValue
      }
    }
    return v
  case map[string]interface{}:
    result := make(map[string]interface{})
    for k, val := range v {
      result[k] = ae.substituteValue(val, eventData)
    }
    return result
  case []interface{}:
    result := make([]interface{}, len(v))
    for i, val := range v {
      result[i] = ae.substituteValue(val, eventData)
    }
    return result
  default:
    return v
  }
}

// Action execution methods

func (ae *ActionExecutor) executeSendMessage(action map[string]interface{}) bool {
  to, ok := action["to"].(string)
  if !ok {
    return false
  }

  message, ok := action["message"].(map[string]interface{})
  if !ok {
    return false
  }

  params := map[string]interface{}{
    "to":      to,
    "message": message,
  }

  result := CallWhatsmeowMethod("SendMessage", params)
  return result != nil && result.Success
}

func (ae *ActionExecutor) executeSendReaction(action map[string]interface{}) bool {
  // Not implemented yet - would need BuildReaction + SendMessage
  return false
}

func (ae *ActionExecutor) executeMarkRead(action map[string]interface{}) bool {
  messageIDs, ok := action["message_ids"].([]interface{})
  if !ok {
    return false
  }

  chat, _ := action["chat"].(string)
  sender, _ := action["sender"].(string)

  params := map[string]interface{}{
    "message_ids": messageIDs,
    "chat":        chat,
    "sender":      sender,
  }

  result := CallWhatsmeowMethod("MarkRead", params)
  return result != nil && result.Success
}

func (ae *ActionExecutor) executeSendPresence(action map[string]interface{}) bool {
  state, ok := action["state"].(string)
  if !ok {
    return false
  }

  params := map[string]interface{}{
    "state": state,
  }

  result := CallWhatsmeowMethod("SendPresence", params)
  return result != nil && result.Success
}

func (ae *ActionExecutor) executeSendChatPresence(action map[string]interface{}) bool {
  jid, ok := action["jid"].(string)
  if !ok {
    return false
  }

  state, ok := action["state"].(string)
  if !ok {
    return false
  }

  params := map[string]interface{}{
    "jid":   jid,
    "state": state,
  }

  if media, ok := action["media"].(string); ok {
    params["media"] = media
  }

  result := CallWhatsmeowMethod("SendChatPresence", params)
  return result != nil && result.Success
}

func (ae *ActionExecutor) executeDelay(action map[string]interface{}) bool {
  seconds, ok := action["seconds"].(float64)
  if !ok {
    return false
  }

  time.Sleep(time.Duration(seconds * float64(time.Second)))
  return true
}

func (ae *ActionExecutor) executeCallMethod(action map[string]interface{}) bool {
  method, ok := action["method"].(string)
  if !ok {
    return false
  }

  params, ok := action["params"].(map[string]interface{})
  if !ok {
    params = make(map[string]interface{})
  }

  result := CallWhatsmeowMethod(method, params)
  return result != nil && result.Success
}

// Logging methods

func (ae *ActionExecutor) logExecutionSuccess(handlerID string, event map[string]interface{}, startTime time.Time, durationMs int64, actionsExecuted int) {
  eventID, _ := event["message_id"].(string)
  eventType, _ := event["event_type"].(string)
  fromJID, _ := event["from"].(string)

  execution := map[string]interface{}{
    "handler_id":       handlerID,
    "event_id":         eventID,
    "event_type":       eventType,
    "from_jid":         fromJID,
    "started_at":       startTime,
    "completed_at":     time.Now(),
    "duration_ms":      durationMs,
    "success":          true,
    "actions_executed": actionsExecuted,
  }

  ae.database.LogHandlerExecution(execution)
  ae.errorState.LogError(ErrorSeverityInfo, "handler_execution",
    fmt.Sprintf("Handler '%s' executed successfully (%dms, %d actions)", handlerID, durationMs, actionsExecuted), "")
}

func (ae *ActionExecutor) logExecutionError(handlerID string, event map[string]interface{}, startTime time.Time, errorMsg string) {
  eventID, _ := event["message_id"].(string)
  eventType, _ := event["event_type"].(string)
  fromJID, _ := event["from"].(string)

  duration := time.Since(startTime).Milliseconds()

  execution := map[string]interface{}{
    "handler_id":   handlerID,
    "event_id":     eventID,
    "event_type":   eventType,
    "from_jid":     fromJID,
    "started_at":   startTime,
    "completed_at": time.Now(),
    "duration_ms":  duration,
    "success":      false,
    "error":        errorMsg,
  }

  ae.database.LogHandlerExecution(execution)
  ae.errorState.LogError(ErrorSeverityWarning, "handler_execution",
    fmt.Sprintf("Handler '%s' failed: %s", handlerID, errorMsg), "")
}

// Helper functions

func toJSON(data interface{}) string {
  jsonBytes, err := json.Marshal(data)
  if err != nil {
    return "{}"
  }
  return string(jsonBytes)
}

