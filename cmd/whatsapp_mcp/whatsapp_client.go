package main

import (
  "bytes"
  "context"
  "encoding/base64"
  "encoding/json"
  "fmt"
  "image/png"
  "os"
  "path/filepath"
  "time"

  "go.mau.fi/whatsmeow"
  "go.mau.fi/whatsmeow/store/sqlstore"
  "go.mau.fi/whatsmeow/types"
  "go.mau.fi/whatsmeow/types/events"
  waLog "go.mau.fi/whatsmeow/util/log"

  _ "github.com/mattn/go-sqlite3"
)

// WhatsAppClient wraps the whatsmeow client with our error handling
type WhatsAppClient struct {
  client        *whatsmeow.Client
  container     *sqlstore.Container
  event_handler_id uint32
  qr_channel    chan string
  connected_channel chan bool
}

// NewWhatsAppClient creates a new WhatsApp client
func NewWhatsAppClient(dbPath string) (*WhatsAppClient, error) {
  // Ensure directory exists
  dir := filepath.Dir(dbPath)
  if err := os.MkdirAll(dir, 0755); err != nil {
    return nil, fmt.Errorf("failed to create database directory: %w", err)
  }

  // Create database container
  container, err := sqlstore.New(context.Background(), "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), waLog.Noop)
  if err != nil {
    return nil, fmt.Errorf("failed to create database container: %w", err)
  }

  // Get first device (or create new one)
  deviceStore, err := container.GetFirstDevice(context.Background())
  if err != nil {
    return nil, fmt.Errorf("failed to get device: %w", err)
  }

  // Create client
  client := whatsmeow.NewClient(deviceStore, waLog.Noop)

  wac := &WhatsAppClient{
    client:        client,
    container:     container,
    qr_channel:    make(chan string, 1),
    connected_channel: make(chan bool, 1),
  }

  return wac, nil
}

// SetupEventHandlers sets up the event handlers for the client
func (wac *WhatsAppClient) SetupEventHandlers() {
  handler := func(evt interface{}) {
    switch v := evt.(type) {
    case *events.QR:
      // QR code received
      global_error_state.LogError(ErrorSeverityInfo, "whatsapp_event", "QR code received", "")
      select {
      case wac.qr_channel <- v.Codes[len(v.Codes)-1]:
      default:
      }

    case *events.PairSuccess:
      // Successfully paired
      global_error_state.LogError(ErrorSeverityInfo, "whatsapp_event", "Paired successfully", fmt.Sprintf("ID: %s", v.ID))
      global_whatsapp_state.mu.Lock()
      global_whatsapp_state.phone_number = v.ID.User
      global_whatsapp_state.device_id = fmt.Sprintf("%d", v.ID.Device)
      global_whatsapp_state.mu.Unlock()

    case *events.Connected:
      // Connected to WhatsApp
      global_error_state.LogError(ErrorSeverityInfo, "whatsapp_event", "Connected to WhatsApp", "")
      global_whatsapp_state.mu.Lock()
      global_whatsapp_state.connection_state = StateConnected
      global_whatsapp_state.last_connected = time.Now()
      global_whatsapp_state.reconnect_attempts = 0
      global_whatsapp_state.mu.Unlock()
      
      global_database.LogConnectionEvent("connected", "Successfully connected to WhatsApp")
      
      select {
      case wac.connected_channel <- true:
      default:
      }

    case *events.Disconnected:
      // Disconnected from WhatsApp
      global_error_state.LogError(ErrorSeverityWarning, "whatsapp_event", "Disconnected from WhatsApp", "")
      global_whatsapp_state.mu.Lock()
      global_whatsapp_state.connection_state = StateDisconnected
      global_whatsapp_state.last_disconnected = time.Now()
      global_whatsapp_state.mu.Unlock()
      
      global_database.LogConnectionEvent("disconnected", "Disconnected from WhatsApp")

    case *events.LoggedOut:
      // Logged out
      global_error_state.LogError(ErrorSeverityWarning, "whatsapp_event", "Logged out from WhatsApp", fmt.Sprintf("Reason: %v", v.Reason))
      global_whatsapp_state.mu.Lock()
      global_whatsapp_state.connection_state = StateDisconnected
      global_whatsapp_state.phone_number = ""
      global_whatsapp_state.device_id = ""
      global_whatsapp_state.mu.Unlock()
      
      global_database.LogConnectionEvent("logged_out", fmt.Sprintf("Reason: %v", v.Reason))

    case *events.Message:
      // Message received - store in database
      msg := map[string]interface{}{
        "message_id":  v.Info.ID,
        "timestamp":   v.Info.Timestamp,
        "from":        v.Info.Sender.String(),
        "chat":        v.Info.Chat.String(),
        "sender_name": v.Info.PushName,
        "is_group":    v.Info.IsGroup,
        "is_from_me":  v.Info.IsFromMe,
        "message_type": "text", // Default, will be updated based on message content
      }

      // Extract text content
      if v.Message.Conversation != nil && *v.Message.Conversation != "" {
        msg["text_content"] = *v.Message.Conversation
        msg["message_type"] = "conversation"
      } else if v.Message.ExtendedTextMessage != nil && v.Message.ExtendedTextMessage.Text != nil {
        msg["text_content"] = *v.Message.ExtendedTextMessage.Text
        msg["message_type"] = "extended_text"
        if v.Message.ExtendedTextMessage.ContextInfo != nil && v.Message.ExtendedTextMessage.ContextInfo.StanzaID != nil {
          msg["quoted_message_id"] = *v.Message.ExtendedTextMessage.ContextInfo.StanzaID
        }
      }

      // Check for media
      if v.Message.ImageMessage != nil {
        msg["message_type"] = "image"
        msg["media_type"] = "image"
        if v.Message.ImageMessage.Mimetype != nil {
          msg["media_mime_type"] = *v.Message.ImageMessage.Mimetype
        }
        if v.Message.ImageMessage.FileLength != nil {
          msg["media_size"] = *v.Message.ImageMessage.FileLength
        }
        if v.Message.ImageMessage.Caption != nil {
          msg["text_content"] = *v.Message.ImageMessage.Caption
        }
      } else if v.Message.VideoMessage != nil {
        msg["message_type"] = "video"
        msg["media_type"] = "video"
        if v.Message.VideoMessage.Mimetype != nil {
          msg["media_mime_type"] = *v.Message.VideoMessage.Mimetype
        }
        if v.Message.VideoMessage.FileLength != nil {
          msg["media_size"] = *v.Message.VideoMessage.FileLength
        }
        if v.Message.VideoMessage.Caption != nil {
          msg["text_content"] = *v.Message.VideoMessage.Caption
        }
      } else if v.Message.DocumentMessage != nil {
        msg["message_type"] = "document"
        msg["media_type"] = "document"
        if v.Message.DocumentMessage.Mimetype != nil {
          msg["media_mime_type"] = *v.Message.DocumentMessage.Mimetype
        }
        if v.Message.DocumentMessage.FileLength != nil {
          msg["media_size"] = *v.Message.DocumentMessage.FileLength
        }
      } else if v.Message.AudioMessage != nil {
        msg["message_type"] = "audio"
        msg["media_type"] = "audio"
        if v.Message.AudioMessage.Mimetype != nil {
          msg["media_mime_type"] = *v.Message.AudioMessage.Mimetype
        }
        if v.Message.AudioMessage.FileLength != nil {
          msg["media_size"] = *v.Message.AudioMessage.FileLength
        }
      }

      // Store raw message for media downloads
      msgBytes, _ := json.Marshal(v.Message)
      msg["raw_message"] = string(msgBytes)

      // Save to database
      if err := global_database.SaveMessage(msg); err != nil {
        global_error_state.LogError(ErrorSeverityWarning, "whatsapp_event", "Failed to save message", err.Error())
      } else {
        global_error_state.LogError(ErrorSeverityInfo, "whatsapp_event", "Message received and stored", fmt.Sprintf("From: %s, Type: %s", v.Info.Sender, msg["message_type"]))
      }

      // Execute handlers for this event (in background)
      if global_action_executor != nil {
        eventData := map[string]interface{}{
          "event_type":   "message",
          "message_id":   msg["message_id"],
          "timestamp":    msg["timestamp"],
          "from":         msg["from"],
          "chat":         msg["chat"],
          "sender_name":  msg["sender_name"],
          "is_group":     msg["is_group"],
          "is_from_me":   msg["is_from_me"],
          "message_type": msg["message_type"],
        }

        // Copy optional fields
        if textContent, ok := msg["text_content"]; ok {
          eventData["text_content"] = textContent
        }
        if mediaType, ok := msg["media_type"]; ok {
          eventData["media_type"] = mediaType
        }
        if mediaMimeType, ok := msg["media_mime_type"]; ok {
          eventData["media_mime_type"] = mediaMimeType
        }
        if mediaSize, ok := msg["media_size"]; ok {
          eventData["media_size"] = mediaSize
        }
        if quotedID, ok := msg["quoted_message_id"]; ok {
          eventData["quoted_message_id"] = quotedID
        }
        if rawMsg, ok := msg["raw_message"]; ok {
          eventData["raw_message"] = rawMsg
        }

        // Execute handlers in background (non-blocking)
        go global_action_executor.ExecuteHandlersForEvent(eventData)
      }
    }
  }

  wac.event_handler_id = wac.client.AddEventHandler(handler)
}

// Connect connects to WhatsApp (auto-login if session exists)
func (wac *WhatsAppClient) Connect() error {
  if wac.client.Store.ID == nil {
    // No session, need to pair
    global_error_state.LogError(ErrorSeverityInfo, "whatsapp_connect", "No session found, pairing required", "")
    return fmt.Errorf("no session found, call get_qr_code to pair")
  }

  // Session exists, try to connect
  global_error_state.LogError(ErrorSeverityInfo, "whatsapp_connect", "Connecting with existing session", "")
  global_whatsapp_state.mu.Lock()
  global_whatsapp_state.connection_state = StateConnecting
  global_whatsapp_state.mu.Unlock()

  err := wac.client.Connect()
  if err != nil {
    global_error_state.LogError(ErrorSeverityError, "whatsapp_connect", "Failed to connect", err.Error())
    global_whatsapp_state.mu.Lock()
    global_whatsapp_state.connection_state = StateError
    global_whatsapp_state.mu.Unlock()
    return err
  }

  return nil
}

// GetQRCode initiates pairing and returns QR code as base64 PNG
func (wac *WhatsAppClient) GetQRCode(timeout int) (string, string, error) {
  if wac.client.Store.ID != nil {
    return "", "", fmt.Errorf("already logged in")
  }

  // Start connection to get QR code
  global_whatsapp_state.mu.Lock()
  global_whatsapp_state.connection_state = StateConnecting
  global_whatsapp_state.mu.Unlock()

  qrChan, err := wac.client.GetQRChannel(context.Background())
  if err != nil {
    global_error_state.LogError(ErrorSeverityCritical, "get_qr_code", "Failed to get QR channel", err.Error())
    return "", "", err
  }

  err = wac.client.Connect()
  if err != nil {
    global_error_state.LogError(ErrorSeverityCritical, "get_qr_code", "Failed to connect for QR", err.Error())
    return "", "", err
  }

  // Wait for QR code with timeout
  timeoutDuration := time.Duration(timeout) * time.Second
  select {
  case evt := <-qrChan:
    if evt.Event == "code" {
      // Generate QR code image
      qrCode := evt.Code
      
      // Generate PNG image from QR code
      img, err := generateQRCodeImage(qrCode)
      if err != nil {
        global_error_state.LogError(ErrorSeverityError, "get_qr_code", "Failed to generate QR image", err.Error())
        return qrCode, "", err
      }

      // Convert to base64
      var buf bytes.Buffer
      if err := png.Encode(&buf, img); err != nil {
        global_error_state.LogError(ErrorSeverityError, "get_qr_code", "Failed to encode QR image", err.Error())
        return qrCode, "", err
      }

      base64Image := base64.StdEncoding.EncodeToString(buf.Bytes())
      
      global_error_state.LogError(ErrorSeverityInfo, "get_qr_code", "QR code generated successfully", "")
      
      return qrCode, base64Image, nil
    }
  case <-time.After(timeoutDuration):
    global_error_state.LogError(ErrorSeverityError, "get_qr_code", "Timeout waiting for QR code", "")
    return "", "", fmt.Errorf("timeout waiting for QR code")
  }

  return "", "", fmt.Errorf("failed to get QR code")
}

// WaitForConnection waits for successful connection after QR scan
func (wac *WhatsAppClient) WaitForConnection(timeout int) error {
  timeoutDuration := time.Duration(timeout) * time.Second
  
  select {
  case <-wac.connected_channel:
    global_error_state.LogError(ErrorSeverityInfo, "wait_for_connection", "Successfully connected", "")
    return nil
  case <-time.After(timeoutDuration):
    global_error_state.LogError(ErrorSeverityError, "wait_for_connection", "Timeout waiting for connection", "")
    return fmt.Errorf("timeout waiting for connection")
  }
}

// IsLoggedIn checks if the client is logged in
func (wac *WhatsAppClient) IsLoggedIn() bool {
  return wac.client.Store.ID != nil
}

// IsConnected checks if the client is connected
func (wac *WhatsAppClient) IsConnected() bool {
  return wac.client.IsConnected()
}

// GetJID returns the user's JID
func (wac *WhatsAppClient) GetJID() types.JID {
  if wac.client.Store.ID == nil {
    return types.EmptyJID
  }
  return *wac.client.Store.ID
}

// Disconnect disconnects from WhatsApp
func (wac *WhatsAppClient) Disconnect() {
  if wac.client != nil {
    wac.client.Disconnect()
  }
}

// Logout logs out and clears the session
func (wac *WhatsAppClient) Logout() error {
  if wac.client.Store.ID == nil {
    return fmt.Errorf("not logged in")
  }

  err := wac.client.Logout(context.Background())
  if err != nil {
    global_error_state.LogError(ErrorSeverityError, "logout", "Failed to logout", err.Error())
    return err
  }

  global_error_state.LogError(ErrorSeverityInfo, "logout", "Logged out successfully", "")
  global_whatsapp_state.mu.Lock()
  global_whatsapp_state.connection_state = StateDisconnected
  global_whatsapp_state.phone_number = ""
  global_whatsapp_state.device_id = ""
  global_whatsapp_state.mu.Unlock()

  return nil
}

// Close closes the client and container
func (wac *WhatsAppClient) Close() error {
  if wac.client != nil {
    wac.client.Disconnect()
  }
  if wac.container != nil {
    return wac.container.Close()
  }
  return nil
}

