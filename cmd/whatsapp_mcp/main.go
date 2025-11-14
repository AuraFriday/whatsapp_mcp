package main

/*
WhatsApp MCP Tool - Main Entry Point
Based on reverse_mcp.go template

This is the minimal MVP to get started quickly.
Phase 1: Basic authentication and message sending/receiving.
*/

import (
  "bufio"
  "bytes"
  "crypto/tls"
  "encoding/binary"
  "encoding/json"
  "flag"
  "fmt"
  "io"
  "io/ioutil"
  "net/http"
  "net/url"
  "os"
  "os/exec"
  "os/signal"
  "path/filepath"
  "runtime"
  "strings"
  "syscall"
  "time"

  "github.com/rs/zerolog"
  "github.com/rs/zerolog/log"
)

// Global system components
var (
  global_error_state       *ErrorState
  global_config            *Config
  global_whatsapp_state    *WhatsAppState
  global_database          *Database
  global_operation_handler *OperationHandler
  global_whatsapp_client   *WhatsAppClient
  global_sse_connection    *SSEConnection
  global_event_matcher     *EventMatcher
  global_action_executor   *ActionExecutor
)

// MCP Server configuration
type MCPServer struct {
  URL     string            `json:"url"`
  Note    string            `json:"note"`
  Headers map[string]string `json:"headers"`
}

type MCPConfig struct {
  MCPServers map[string]MCPServer `json:"mcpServers"`
}

type Manifest struct {
  Name string `json:"name"`
  Type string `json:"type"`
  Path string `json:"path"`
}

// JSON-RPC structures
type JSONRPCRequest struct {
  JSONRPC string      `json:"jsonrpc"`
  ID      string      `json:"id"`
  Method  string      `json:"method"`
  Params  interface{} `json:"params"`
}

type JSONRPCResponse struct {
  JSONRPC string          `json:"jsonrpc"`
  ID      string          `json:"id"`
  Result  json.RawMessage `json:"result,omitempty"`
  Error   interface{}     `json:"error,omitempty"`
}

type ReverseCall struct {
  Tool    string          `json:"tool"`
  CallID  string          `json:"call_id"`
  Input   json.RawMessage `json:"input"`
  IsError bool            `json:"isError"`
}

type ReverseMessage struct {
  JSONRPC string      `json:"jsonrpc"`
  ID      string      `json:"id"`
  Reverse ReverseCall `json:"reverse"`
}

// SSE Connection
type SSEConnection struct {
  ServerURL       string
  AuthHeader      string
  SessionID       string
  MessageEndpoint string
  Client          *http.Client
  ReverseChannel  chan ReverseMessage
  ResponseChannel map[string]chan JSONRPCResponse
  StopChannel     chan bool
  IsAlive         *bool
}

// Find native messaging manifest (same as reverse_mcp.go)
func findNativeMessagingManifest() (string, error) {
  var possiblePaths []string
  homeDir, _ := os.UserHomeDir()

  switch runtime.GOOS {
  case "windows":
    localAppData := os.Getenv("LOCALAPPDATA")
    if localAppData == "" {
      localAppData = filepath.Join(homeDir, "AppData", "Local")
    }
    possiblePaths = append(possiblePaths, filepath.Join(localAppData, "AuraFriday", "com.aurafriday.shim.json"))

  case "darwin":
    possiblePaths = append(possiblePaths,
      filepath.Join(homeDir, "Library/Application Support/Google/Chrome/NativeMessagingHosts/com.aurafriday.shim.json"),
      filepath.Join(homeDir, "Library/Application Support/Chromium/NativeMessagingHosts/com.aurafriday.shim.json"),
      filepath.Join(homeDir, "Library/Application Support/Microsoft Edge/NativeMessagingHosts/com.aurafriday.shim.json"),
    )

  default: // linux
    possiblePaths = append(possiblePaths,
      filepath.Join(homeDir, ".config/google-chrome/NativeMessagingHosts/com.aurafriday.shim.json"),
      filepath.Join(homeDir, ".config/chromium/NativeMessagingHosts/com.aurafriday.shim.json"),
      filepath.Join(homeDir, ".config/microsoft-edge/NativeMessagingHosts/com.aurafriday.shim.json"),
    )
  }

  for _, path := range possiblePaths {
    if _, err := os.Stat(path); err == nil {
      return path, nil
    }
  }

  return "", fmt.Errorf("manifest not found")
}

// Read manifest
func readManifest(path string) (*Manifest, error) {
  data, err := ioutil.ReadFile(path)
  if err != nil {
    return nil, err
  }

  var manifest Manifest
  if err := json.Unmarshal(data, &manifest); err != nil {
    return nil, err
  }

  return &manifest, nil
}

// Discover MCP server endpoint (same as reverse_mcp.go)
func discoverMCPServerEndpoint(manifest *Manifest) (*MCPConfig, error) {
  binaryPath := manifest.Path
  if _, err := os.Stat(binaryPath); err != nil {
    return nil, fmt.Errorf("binary not found: %s", binaryPath)
  }

  fmt.Fprintf(os.Stderr, "Running native binary: %s\n", binaryPath)

  cmd := exec.Command(binaryPath)
  stdout, err := cmd.StdoutPipe()
  if err != nil {
    return nil, err
  }

  if err := cmd.Start(); err != nil {
    return nil, err
  }

  done := make(chan *MCPConfig, 1)
  go func() {
    lengthBytes := make([]byte, 4)
    n, err := io.ReadFull(stdout, lengthBytes)
    if err != nil || n != 4 {
      fmt.Fprintf(os.Stderr, "ERROR: Failed to read length prefix: %v\n", err)
      done <- nil
      return
    }

    messageLength := binary.LittleEndian.Uint32(lengthBytes)
    if messageLength <= 0 || messageLength > 10000000 {
      fmt.Fprintf(os.Stderr, "ERROR: Invalid message length: %d\n", messageLength)
      done <- nil
      return
    }

    jsonBytes := make([]byte, messageLength)
    n, err = io.ReadFull(stdout, jsonBytes)
    if err != nil || n != int(messageLength) {
      fmt.Fprintf(os.Stderr, "ERROR: Failed to read JSON: %v\n", err)
      done <- nil
      return
    }

    var config MCPConfig
    if err := json.Unmarshal(jsonBytes, &config); err != nil {
      fmt.Fprintf(os.Stderr, "ERROR: Failed to parse JSON: %v\n", err)
      done <- nil
      return
    }

    done <- &config
  }()

  select {
  case config := <-done:
    cmd.Process.Kill()
    if config != nil {
      return config, nil
    }
    return nil, fmt.Errorf("no valid JSON received")
  case <-time.After(5 * time.Second):
    cmd.Process.Kill()
    return nil, fmt.Errorf("timeout")
  }
}

// Connect to SSE endpoint (same as reverse_mcp.go)
func connectSSE(serverURL, authHeader string) (*SSEConnection, error) {
  isAlive := true
  conn := &SSEConnection{
    ServerURL:       serverURL,
    AuthHeader:      authHeader,
    ReverseChannel:  make(chan ReverseMessage, 100),
    ResponseChannel: make(map[string]chan JSONRPCResponse),
    StopChannel:     make(chan bool, 1),
    IsAlive:         &isAlive,
    Client: &http.Client{
      Transport: &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
      },
    },
  }

  req, err := http.NewRequest("GET", serverURL, nil)
  if err != nil {
    return nil, err
  }

  req.Header.Set("Accept", "text/event-stream")
  req.Header.Set("Cache-Control", "no-cache")
  req.Header.Set("Authorization", authHeader)

  resp, err := conn.Client.Do(req)
  if err != nil {
    return nil, err
  }

  if resp.StatusCode != 200 {
    return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
  }

  // Read SSE stream
  go func() {
    defer func() {
      *conn.IsAlive = false
    }()
    
    scanner := bufio.NewScanner(resp.Body)
    var eventType string

    for scanner.Scan() {
      select {
      case <-conn.StopChannel:
        return
      default:
      }
      
      line := scanner.Text()
      line = strings.TrimRight(line, "\r")

      if line == "" {
        eventType = ""
        continue
      }

      if strings.HasPrefix(line, ":") {
        continue
      }

      colonIdx := strings.Index(line, ":")
      if colonIdx == -1 {
        continue
      }

      field := line[:colonIdx]
      value := line[colonIdx+1:]
      if strings.HasPrefix(value, " ") {
        value = value[1:]
      }

      switch field {
      case "event":
        eventType = value
      case "data":
        if eventType == "endpoint" {
          conn.MessageEndpoint = value
          if strings.Contains(value, "session_id=") {
            parts := strings.Split(value, "session_id=")
            if len(parts) > 1 {
              conn.SessionID = strings.Split(parts[1], "&")[0]
            }
          }
        } else {
          var msg map[string]interface{}
          if err := json.Unmarshal([]byte(value), &msg); err == nil {
            if _, ok := msg["reverse"]; ok {
              var revMsg ReverseMessage
              json.Unmarshal([]byte(value), &revMsg)
              conn.ReverseChannel <- revMsg
            } else if id, ok := msg["id"].(string); ok {
              if ch, exists := conn.ResponseChannel[id]; exists {
                var response JSONRPCResponse
                json.Unmarshal([]byte(value), &response)
                ch <- response
                delete(conn.ResponseChannel, id)
              }
            }
          }
        }
      }
    }
  }()

  // Wait for session ID
  for i := 0; i < 50; i++ {
    if conn.SessionID != "" {
      break
    }
    time.Sleep(100 * time.Millisecond)
  }

  if conn.SessionID == "" {
    return nil, fmt.Errorf("no session ID received")
  }

  return conn, nil
}

// Send JSON-RPC request
func (conn *SSEConnection) sendRequest(method string, params interface{}) (json.RawMessage, error) {
  requestID := fmt.Sprintf("%d", time.Now().UnixNano())

  request := JSONRPCRequest{
    JSONRPC: "2.0",
    ID:      requestID,
    Method:  method,
    Params:  params,
  }

  body, err := json.Marshal(request)
  if err != nil {
    return nil, err
  }

  respChan := make(chan JSONRPCResponse, 1)
  conn.ResponseChannel[requestID] = respChan

  u, _ := url.Parse(conn.ServerURL)
  fullURL := fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, conn.MessageEndpoint)

  req, err := http.NewRequest("POST", fullURL, bytes.NewReader(body))
  if err != nil {
    delete(conn.ResponseChannel, requestID)
    return nil, err
  }

  req.Header.Set("Content-Type", "application/json")
  req.Header.Set("Authorization", conn.AuthHeader)

  resp, err := conn.Client.Do(req)
  if err != nil {
    delete(conn.ResponseChannel, requestID)
    return nil, err
  }
  defer resp.Body.Close()

  if resp.StatusCode != 202 {
    delete(conn.ResponseChannel, requestID)
    return nil, fmt.Errorf("POST failed: %d", resp.StatusCode)
  }

  select {
  case response := <-respChan:
    return response.Result, nil
  case <-time.After(10 * time.Second):
    delete(conn.ResponseChannel, requestID)
    return nil, fmt.Errorf("timeout")
  }
}

// Send tool reply
func (conn *SSEConnection) sendToolReply(callID string, result interface{}) error {
  params := map[string]interface{}{
    "result": result,
  }

  request := JSONRPCRequest{
    JSONRPC: "2.0",
    ID:      callID,
    Method:  "tools/reply",
    Params:  params,
  }

  body, err := json.Marshal(request)
  if err != nil {
    return err
  }

  u, _ := url.Parse(conn.ServerURL)
  fullURL := fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, conn.MessageEndpoint)

  req, err := http.NewRequest("POST", fullURL, bytes.NewReader(body))
  if err != nil {
    return err
  }

  req.Header.Set("Content-Type", "application/json")
  req.Header.Set("Authorization", conn.AuthHeader)

  resp, err := conn.Client.Do(req)
  if err != nil {
    return err
  }
  defer resp.Body.Close()

  if resp.StatusCode == 202 {
    fmt.Fprintf(os.Stderr, "[OK] Sent tools/reply for call_id %s\n", callID)
    return nil
  }

  return fmt.Errorf("POST failed: %d", resp.StatusCode)
}

// Initialize system components
func initializeSystem() error {
  // Initialize logging
  zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
  log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

  // Load method registry
  fmt.Fprintln(os.Stderr, "[INFO] Loading method registry...")
  if err := LoadMethodRegistry(); err != nil {
    return fmt.Errorf("failed to load method registry: %w", err)
  }
  fmt.Fprintf(os.Stderr, "[OK] Loaded %d methods from registry\n", len(globalMethodRegistry.Methods))

  // Initialize configuration
  global_config = NewConfig()

  // Initialize error state
  global_error_state = NewErrorState(100) // Keep last 100 errors in memory

  // Initialize WhatsApp state
  global_whatsapp_state = &WhatsAppState{
    connection_state: StateDisconnected,
  }

  // Initialize database
  dbPath := global_config.GetHandlersDatabasePath()
  db, err := NewDatabase(dbPath)
  if err != nil {
    return fmt.Errorf("failed to initialize database: %w", err)
  }
  global_database = db

  // Load saved config from database
  var savedConfig map[string]interface{}
  if err := global_database.LoadConfig("app_config", &savedConfig); err == nil && savedConfig != nil {
    global_config.UpdateFromMap(savedConfig)
  }

  // Initialize operation handler
  global_operation_handler = NewOperationHandler(
    global_error_state,
    global_config,
    global_whatsapp_state,
    global_database,
  )

  // Initialize event matcher
  global_event_matcher = NewEventMatcher(global_database)
  fmt.Fprintln(os.Stderr, "[INFO] Loading event handlers...")
  if err := global_event_matcher.LoadHandlers(); err != nil {
    fmt.Fprintf(os.Stderr, "[WARN] Failed to load handlers: %v\n", err)
  } else {
    fmt.Fprintf(os.Stderr, "[OK] Loaded %d event handlers\n", len(global_event_matcher.handlers))
  }

  // Initialize action executor
  global_action_executor = NewActionExecutor(global_database, global_error_state, global_event_matcher)
  fmt.Fprintln(os.Stderr, "[OK] Action executor initialized\n")

  // Initialize WhatsApp client
  whatsappClient, err := NewWhatsAppClient(global_config.GetDatabasePath())
  if err != nil {
    return fmt.Errorf("failed to initialize WhatsApp client: %w", err)
  }
  global_whatsapp_client = whatsappClient

  // Setup event handlers
  global_whatsapp_client.SetupEventHandlers()

  // Try to auto-connect if session exists
  if global_whatsapp_client.IsLoggedIn() {
    log.Info().Msg("Existing session found, attempting to connect...")
    go func() {
      if err := global_whatsapp_client.Connect(); err != nil {
        log.Error().Err(err).Msg("Failed to auto-connect")
        global_error_state.LogError(ErrorSeverityWarning, "auto_connect", "Failed to auto-connect", err.Error())
      } else {
        log.Info().Msg("Auto-connected successfully")
      }
    }()
  } else {
    log.Info().Msg("No existing session, call get_qr_code to pair")
  }

  log.Info().Msg("System initialized successfully")
  log.Info().Str("database_path", global_config.GetDatabasePath()).Msg("Configuration loaded")

  // Log startup event
  global_error_state.LogError(ErrorSeverityInfo, "startup", "WhatsApp MCP Tool started", fmt.Sprintf("PID: %d", os.Getpid()))
  global_database.LogConnectionEvent("startup", fmt.Sprintf("PID: %d", os.Getpid()))

  return nil
}

// Shutdown system components
func shutdownSystem() {
  log.Info().Msg("Shutting down system...")

  if global_whatsapp_client != nil {
    log.Info().Msg("Disconnecting WhatsApp client...")
    if err := global_whatsapp_client.Close(); err != nil {
      log.Error().Err(err).Msg("Error closing WhatsApp client")
    }
  }

  if global_database != nil {
    if err := global_database.Close(); err != nil {
      log.Error().Err(err).Msg("Error closing database")
    }
  }

  log.Info().Msg("Shutdown complete")
}

// callMCPTool calls another MCP tool (e.g., user, sqlite, etc.)
func callMCPTool(conn *SSEConnection, toolName string, arguments interface{}) (json.RawMessage, error) {
  toolCallParams := map[string]interface{}{
    "name":      toolName,
    "arguments": arguments,
  }

  // Use longer timeout for tool calls (30 seconds)
  requestID := fmt.Sprintf("%d", time.Now().UnixNano())

  request := JSONRPCRequest{
    JSONRPC: "2.0",
    ID:      requestID,
    Method:  "tools/call",
    Params:  toolCallParams,
  }

  body, err := json.Marshal(request)
  if err != nil {
    return nil, err
  }

  // Create response channel
  respChan := make(chan JSONRPCResponse, 1)
  conn.ResponseChannel[requestID] = respChan

  // Parse URL
  u, _ := url.Parse(conn.ServerURL)
  fullURL := fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, conn.MessageEndpoint)

  req, err := http.NewRequest("POST", fullURL, bytes.NewReader(body))
  if err != nil {
    delete(conn.ResponseChannel, requestID)
    return nil, err
  }

  req.Header.Set("Content-Type", "application/json")
  req.Header.Set("Authorization", conn.AuthHeader)

  resp, err := conn.Client.Do(req)
  if err != nil {
    delete(conn.ResponseChannel, requestID)
    return nil, err
  }
  defer resp.Body.Close()

  if resp.StatusCode != 202 {
    delete(conn.ResponseChannel, requestID)
    return nil, fmt.Errorf("POST failed: %d", resp.StatusCode)
  }

  // Wait for response with 30 second timeout
  select {
  case response := <-respChan:
    if response.Error != nil {
      return nil, fmt.Errorf("tool call error: %v", response.Error)
    }
    return response.Result, nil
  case <-time.After(30 * time.Second):
    delete(conn.ResponseChannel, requestID)
    return nil, fmt.Errorf("timeout waiting for tool response")
  }
}

// Register WhatsApp tool
func registerWhatsAppTool(conn *SSEConnection) error {
  fmt.Fprintln(os.Stderr, "Registering whatsapp tool with MCP server...")

  params := map[string]interface{}{
    "name": "remote",
    "arguments": map[string]interface{}{
      "input": map[string]interface{}{
        "operation": "register",
        "tool_name": "whatsapp",
        "readme": fmt.Sprintf(`%s v%s

## Operations
- check_login_status, get_qr_code, logout - Authentication
- call_whatsmeow - Generic dispatcher (call ANY whatsmeow method)
- get_messages - Query message history (limit, from, chat, since)
- get_method_registry - Get full method list with examples
- get_version, get_health_status, get_error_log - System ops
- shutdown - Graceful exit

## Send Message
{
  "operation": "call_whatsmeow",
  "data": {
    "method": "SendMessage",
    "params": {
      "to": "61487543210",
      "message": {"conversation": "Hello!"}
    }
  }
}

## Get Messages
{
  "operation": "get_messages",
  "data": {"limit": 50, "from": "61487543210@s.whatsapp.net"}
}

Phone numbers auto-format: "61487543210" → "61487543210@s.whatsapp.net"

Available methods: SendMessage, SendPresence, SendChatPresence, GetUserInfo, GetProfilePictureInfo, MarkRead, BuildEdit, BuildRevoke, DownloadMediaWithPath

Use get_method_registry for full documentation with parameters, types, and examples.

## Event Handlers (Phase 2.2+)
Handlers return actions, don't execute directly:
✅ return {'actions': [{'type': 'send_message', 'to': '...', 'message': {...}}]}
❌ Don't call mcp.call('whatsapp', ...) for writes
✅ Research queries (GetUserInfo, etc.) are OK

See TOOL_DOCUMENTATION_FOR_LLMS.md for complete guide.`, ToolName, ToolVersion),
        "description": fmt.Sprintf("%s v%s - Send/receive WhatsApp messages, query history, call ANY whatsmeow method via generic dispatcher. Auto-login, panic recovery, message templates.", ToolName, ToolVersion),
        "parameters": map[string]interface{}{
          "type": "object",
          "properties": map[string]interface{}{
            "operation": map[string]interface{}{
              "type": "string",
              "enum": []string{
                "get_version",
                "get_health_status",
                "get_error_log",
                "clear_error_state",
                "get_config",
                "set_config",
                "get_connection_info",
                "get_qr_code",
                "check_login_status",
                "logout",
                "shutdown",
                "call_whatsmeow",
                "get_method_registry",
                "get_messages",
                "register_handler",
                "list_handlers",
                "get_handler",
                "update_handler",
                "delete_handler",
                "enable_handler",
                "disable_handler",
                "get_handler_executions",
                "reload_handlers",
              },
              "description": "Operation to perform",
            },
            "data": map[string]interface{}{
              "type": "object",
              "description": "Operation-specific data",
            },
          },
          "required": []string{"operation"},
        },
        "callback_endpoint": "whatsapp://tool",
        "TOOL_API_KEY":      "whatsapp_mcp_auth_key_12345",
      },
    },
  }

  result, err := conn.sendRequest("tools/call", params)
  if err != nil {
    return err
  }

  var response map[string]interface{}
  if err := json.Unmarshal(result, &response); err == nil {
    if content, ok := response["content"].([]interface{}); ok && len(content) > 0 {
      if item, ok := content[0].(map[string]interface{}); ok {
        if text, ok := item["text"].(string); ok && strings.Contains(text, "Successfully registered tool") {
          fmt.Fprintf(os.Stderr, "[OK] %s\n", text)
          return nil
        }
      }
    }
  }

  return fmt.Errorf("unexpected registration response")
}

// Handle WhatsApp operations
func handleWhatsAppOperation(inputData json.RawMessage, conn *SSEConnection) map[string]interface{} {
  var callData map[string]interface{}
  if err := json.Unmarshal(inputData, &callData); err != nil {
    log.Error().Err(err).Msg("Failed to unmarshal call data")
    return map[string]interface{}{
      "content": []map[string]interface{}{
        {
          "type": "text",
          "text": fmt.Sprintf("Error: Failed to parse input: %v", err),
        },
      },
      "isError": true,
    }
  }

  params, _ := callData["params"].(map[string]interface{})
  arguments, _ := params["arguments"].(map[string]interface{})
  operation, _ := arguments["operation"].(string)
  data, _ := arguments["data"].(map[string]interface{})

  log.Info().Str("operation", operation).Msg("Handling WhatsApp operation")

  // Create operation input
  input := &OperationInput{
    Operation: operation,
    Data:      data,
  }

  // Handle operation
  result := global_operation_handler.HandleOperation(input)

  // Log to database if error
  if !result.Success {
    global_error_state.LogError(ErrorSeverityError, operation, result.Error, "")
    global_database.LogError(global_error_state.GetRecentErrors(nil, 1)[0])
  }

  // Special handling for get_qr_code - return image
  if operation == "get_qr_code" && result.Success {
    if qrBase64, ok := result.Data["qr_code_base64"].(string); ok && qrBase64 != "" {
      // Return as image using proper MCP image content type
      return map[string]interface{}{
        "content": []map[string]interface{}{
          {
            "type": "image",
            "mimeType": "image/png",
            "data": qrBase64,
          },
          {
            "type": "text",
            "text": fmt.Sprintf("QR Code generated successfully!\n\nInstructions: %s\n\nQR Code Text: %s\n\nTimeout: %d seconds",
              result.Data["instructions"],
              result.Data["qr_code_text"],
              result.Data["timeout"]),
          },
        },
        "isError": false,
      }
    }
  }

  // Format result as MCP response (standard text response)
  resultJSON, err := json.Marshal(result)
  if err != nil {
    log.Error().Err(err).Msg("Failed to marshal result")
    return map[string]interface{}{
      "content": []map[string]interface{}{
        {
          "type": "text",
          "text": fmt.Sprintf("Error: Failed to format result: %v", err),
        },
      },
      "isError": true,
    }
  }

  return map[string]interface{}{
    "content": []map[string]interface{}{
      {
        "type": "text",
        "text": string(resultJSON),
      },
    },
    "isError": !result.Success,
  }
}

// Main worker
func mainWorker() int {
	fmt.Fprintf(os.Stderr, "=== %s v%s ===\n", ToolName, ToolVersion)
	fmt.Fprintf(os.Stderr, "PID: %d\n", os.Getpid())
	fmt.Fprintln(os.Stderr, "Initializing system...\n")

  // Initialize system components
  if err := initializeSystem(); err != nil {
    fmt.Fprintf(os.Stderr, "ERROR: Failed to initialize system: %v\n", err)
    return 1
  }
  defer shutdownSystem()

  fmt.Fprintln(os.Stderr, "Connecting to MCP server...\n")

  // Setup signal handling
  sigChan := make(chan os.Signal, 1)
  signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

  // Step 1: Find manifest
  fmt.Fprintln(os.Stderr, "Step 1: Finding native messaging manifest...")
  manifestPath, err := findNativeMessagingManifest()
  if err != nil {
    fmt.Fprintln(os.Stderr, "ERROR: Could not find native messaging manifest")
    return 1
  }
  fmt.Fprintf(os.Stderr, "[OK] Found manifest: %s\n\n", manifestPath)

  // Step 2: Read manifest
  fmt.Fprintln(os.Stderr, "Step 2: Reading manifest...")
  manifest, err := readManifest(manifestPath)
  if err != nil {
    fmt.Fprintln(os.Stderr, "ERROR: Could not read manifest")
    return 1
  }
  fmt.Fprintln(os.Stderr, "[OK] Manifest loaded\n")

  // Step 3: Discover endpoint
  fmt.Fprintln(os.Stderr, "Step 3: Discovering MCP server endpoint...")
  config, err := discoverMCPServerEndpoint(manifest)
  if err != nil {
    fmt.Fprintln(os.Stderr, "ERROR: Could not get configuration")
    return 1
  }

  var serverURL, authHeader string
  for _, server := range config.MCPServers {
    serverURL = server.URL
    authHeader = server.Headers["Authorization"]
    break
  }

  if serverURL == "" {
    fmt.Fprintln(os.Stderr, "ERROR: Could not extract server URL")
    return 1
  }
  fmt.Fprintf(os.Stderr, "[OK] Found server at: %s\n\n", serverURL)

  // Step 4: Connect to SSE
  fmt.Fprintln(os.Stderr, "Step 4: Connecting to SSE endpoint...")
  conn, err := connectSSE(serverURL, authHeader)
  if err != nil {
    fmt.Fprintf(os.Stderr, "ERROR: Could not connect: %v\n", err)
    return 1
  }
  fmt.Fprintf(os.Stderr, "[OK] Connected! Session ID: %s\n\n", conn.SessionID)
  
  // Store connection globally for tool calls
  global_sse_connection = conn

  // Step 5: Register WhatsApp tool
  fmt.Fprintln(os.Stderr, "Step 5: Registering whatsapp tool...")
  if err := registerWhatsAppTool(conn); err != nil {
    fmt.Fprintf(os.Stderr, "ERROR: Failed to register: %v\n", err)
    return 1
  }

  fmt.Fprintln(os.Stderr, "\n"+strings.Repeat("=", 60))
  fmt.Fprintln(os.Stderr, "[OK] WhatsApp tool registered successfully!")
  fmt.Fprintln(os.Stderr, "Listening for tool calls... (Press Ctrl+C to stop)")
  fmt.Fprintln(os.Stderr, strings.Repeat("=", 60)+"\n")

  // Step 6: Listen for reverse calls
  for {
    select {
    case msg := <-conn.ReverseChannel:
      fmt.Fprintln(os.Stderr, "\n[CALL] Reverse call received:")
      fmt.Fprintf(os.Stderr, "       Tool: %s\n", msg.Reverse.Tool)
      fmt.Fprintf(os.Stderr, "       Call ID: %s\n", msg.Reverse.CallID)

      if msg.Reverse.Tool == "whatsapp" {
        result := handleWhatsAppOperation(msg.Reverse.Input, conn)
        conn.sendToolReply(msg.Reverse.CallID, result)
      } else {
        fmt.Fprintf(os.Stderr, "[WARN] Unknown tool: %s\n", msg.Reverse.Tool)
      }

    case <-sigChan:
      fmt.Fprintln(os.Stderr, "\n\n"+strings.Repeat("=", 60))
      fmt.Fprintln(os.Stderr, "Shutting down...")
      fmt.Fprintln(os.Stderr, strings.Repeat("=", 60))
      conn.StopChannel <- true
      return 0
    }
  }
}

func main() {
  background := flag.Bool("background", false, "Run in background mode")
  help := flag.Bool("help", false, "Show help")
  flag.Parse()

  if *help {
    fmt.Println("Usage: whatsapp_mcp [--background]")
    fmt.Println("\nWhatsApp MCP Tool - Registers whatsapp tool with MCP server")
    return
  }

  if *background {
    fmt.Fprintf(os.Stderr, "Starting in background mode (PID: %d)...\n", os.Getpid())
  }

  os.Exit(mainWorker())
}


