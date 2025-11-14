# WhatsApp MCP Tool

**AI-Powered WhatsApp Automation with Event-Driven Smart Handlers**

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://go.dev/)
[![WhatsApp](https://img.shields.io/badge/WhatsApp-Multi--Device-25D366?logo=whatsapp)](https://github.com/tulir/whatsmeow)
[![MCP](https://img.shields.io/badge/MCP-Protocol-purple)](https://modelcontextprotocol.io)
[![License](https://img.shields.io/badge/License-Proprietary-red)](LICENSE)

---

## 🚀 What This Is

A **production-ready** Go application that transforms WhatsApp into a full-featured AI automation platform. Built on the [whatsmeow](https://github.com/tulir/whatsmeow) library and the [Model Context Protocol (MCP)](https://modelcontextprotocol.io), this tool enables AI agents to send/receive messages, manage groups, handle media, and create sophisticated event-driven automation workflows.

**Key Innovation:** Event handlers can trigger Python scripts, call LLMs, query databases, control hardware, browse the web, and orchestrate unlimited automation using the entire MCP tool ecosystem—all while maintaining safety through rate limiting, circuit breakers, and concurrent execution.

---

## ✨ Features

### 🎯 Core Capabilities

- ✅ **Send/Receive Messages** - Text, images, videos, audio, documents, stickers
- ✅ **Generic Dispatcher** - Call ANY whatsmeow method without custom code
- ✅ **Query Message History** - Filter by sender, chat, time, type
- ✅ **Event-Driven Automation** - Python handlers triggered by WhatsApp events
- ✅ **Multi-Modal QR Code** - ASCII art, Base64 image, beautiful HTML popup
- ✅ **Auto-Login** - Session persistence with automatic reconnection
- ✅ **Panic Recovery** - 2-layer crash protection, server stays running
- ✅ **Comprehensive Error Handling** - Detailed logs, health monitoring, AI inspection

### 🤖 Event-Driven Automation

**Program WhatsApp to respond to events using Python, LLMs, databases, and more:**

```python
# Handler receives WhatsApp message
# Queries database for context
# Calls LLM for smart response
# Returns actions to execute

return {
    'actions': [
        {'type': 'send_chat_presence', 'jid': '...', 'state': 'composing'},
        {'type': 'delay', 'seconds': 1.5},
        {'type': 'send_message', 'to': '...', 'message': {'conversation': 'AI reply'}},
        {'type': 'mark_read', 'chat': '...', 'message_ids': ['...']}
    ]
}
```

**Handlers can:**
- 🐍 Execute Python code with full MCP tool access
- 🧠 Call 500+ AI models via OpenRouter
- 🗄️ Query/update SQLite databases
- 🌐 Automate Chrome browser
- 🖥️ Control desktop applications
- 📱 Trigger hardware via serial/SSH
- 💬 Show HTML popups to users
- 🔗 Chain multiple actions with variable substitution

### 🛡️ Safety Features

- **Rate Limiting** - Per-minute, per-hour, per-sender limits
- **Circuit Breakers** - Auto-disable failing handlers
- **Loop Prevention** - Filter own messages, cooldowns, execution tracking
- **Concurrent Execution** - Non-blocking goroutines, main loop stays responsive
- **Timeout Protection** - Configurable handler timeouts (default 30s)
- **Execution Logging** - Complete audit trail in SQLite

---

## 📦 Installation

### Prerequisites

1. **MCP-Link Server**  
   Download from: [https://github.com/AuraFriday/mcp-link-server/releases/latest](https://github.com/AuraFriday/mcp-link-server/releases/latest)

2. **Go 1.24+** (for building from source)  
   Install from: [https://go.dev/dl/](https://go.dev/dl/)

### Build from Source

```bash
# Clone this repository
git clone https://github.com/yourusername/whatsmeow.git
cd whatsmeow

# Build the tool
make

# Run the tool (connects to MCP server automatically)
./whatsapp_mcp.exe
```

The tool auto-discovers the MCP server via native messaging manifest and registers itself as the `whatsapp` tool.

---

## 🎬 Quick Start

### 1. Authentication

**Get QR code for pairing:**
```json
{
  "operation": "get_qr_code"
}
```

Displays QR code in:
- ✅ Terminal (ASCII art)
- ✅ IDE (Base64 image)
- ✅ Desktop popup (HTML window)

Scan with WhatsApp mobile app → **Instant connection!**

### 2. Send Your First Message

```json
{
  "operation": "call_whatsmeow",
  "data": {
    "method": "SendMessage",
    "params": {
      "to": "61487543210",
      "message": {"conversation": "Hello from AI! 🤖"}
    }
  }
}
```

**Phone numbers auto-format:** `"61487543210"` → `"61487543210@s.whatsapp.net"`

### 3. Query Message History

```json
{
  "operation": "get_messages",
  "data": {
    "limit": 50,
    "from": "61487543210@s.whatsapp.net",
    "since": "2025-11-10T00:00:00Z"
  }
}
```

### 4. Register Event Handler

```json
{
  "operation": "register_handler",
  "data": {
    "handler_id": "auto_reply",
    "description": "Responds to hello messages",
    "event_filter": {
      "event_types": ["message"],
      "is_from_me": false,
      "text_contains": ["hello", "hi"]
    },
    "action": {
      "type": "python",
      "code": "return {'actions': [{'type': 'send_message', 'to': event['from'], 'message': {'conversation': 'Hello! How can I help?'}}]}"
    },
    "enabled": true,
    "max_executions_per_hour": 50,
    "timeout_seconds": 30
  }
}
```

**Now every "hello" message triggers your handler automatically!**

---

## 🏗️ Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────┐
│                   AI Agent (Claude, GPT, etc.)              │
│  Programs handlers via natural language → JSON              │
└────────┬────────────────────────────────────────┬───────────┘
         │                                        │
         │ Register Handlers                      │ Query/Control
         ▼                                        ▼
┌─────────────────────────────────────────────────────────────┐
│              WhatsApp MCP Tool (Go)                         │
│                                                             │
│  ┌──────────────┐  ┌───────────────┐  ┌─────────────────┐ │
│  │   Handler    │  │     Event     │  │     Action      │ │
│  │   Registry   │◀─│   Processor   │─▶│    Executor     │ │
│  │  (SQLite)    │  │  (Goroutines) │  │   (Python)      │ │
│  └──────────────┘  └───────────────┘  └─────────────────┘ │
│                           │                     │           │
│                           ▼                     ▼           │
│                    ┌─────────────┐      ┌─────────────┐    │
│                    │  whatsmeow  │      │  MCP Tools  │    │
│                    │   Client    │      │  Ecosystem  │    │
│                    └─────────────┘      └─────────────┘    │
└─────────────────────────────────────────────────────────────┘
                           │                     │
                           ▼                     ▼
                    WhatsApp Servers      Python, SQLite,
                                         OpenRouter, Browser,
                                         User, System, etc.
```

### Event Flow

```
1. WhatsApp message arrives
   ↓
2. Saved to database
   ↓
3. Event matcher finds handlers (filters, rate limits, circuit breakers)
   ↓
4. Spawn handler goroutines (NON-BLOCKING!)
   ↓
5. Download media if present
   ↓
6. Execute Python handler
   ├─→ Handler can call WhatsApp for research (GetUserInfo, etc.)
   │   └─→ Main loop responds (still running!)
   └─→ Handler returns actions
   ↓
7. Execute actions (send_message, mark_read, delay, etc.)
   ↓
8. Log execution, update stats
   ↓
9. Clean up temp files
```

**Main event loop NEVER blocks** - handlers run concurrently in goroutines!

---

## 🎯 Real-World Use Cases

### 1. Smart Customer Service Bot

```python
# Handler code
# Get conversation history from database
history = mcp.call('sqlite', {
    'input': {
        'sql': 'SELECT * FROM messages WHERE from_jid = :jid LIMIT 10',
        'bindings': {'jid': event['from']},
        'database': '@user_data/conversations.db'
    }
})

# Analyze with LLM
response = mcp.call('openrouter', {
    'input': {
        'operation': 'chat_completion',
        'model': 'anthropic/claude-3.5-sonnet',
        'messages': [
            {'role': 'system', 'content': 'You are a helpful customer service agent.'},
            {'role': 'user', 'content': f'History: {history}\nNew: {event["text_content"]}'}
        ]
    }
})

# Return actions
return {
    'actions': [
        {'type': 'send_chat_presence', 'jid': event['from'], 'state': 'composing'},
        {'type': 'delay', 'seconds': 2.0},
        {'type': 'send_message', 'to': event['from'], 
         'message': {'conversation': response['content'][0]['text']}},
        {'type': 'mark_read', 'chat': event['chat'], 'message_ids': [event['message_id']]}
    ]
}
```

### 2. Media Archive Bot

```python
# Handler automatically receives downloaded media
if event.get('has_media'):
    media_path = event['media_path']  # /tmp/whatsapp_media/3EB0ABC123_image.jpg
    
    # Process media
    from PIL import Image
    img = Image.open(media_path)
    
    # Archive it
    import shutil
    archive_path = f"/archive/{event['sender_name']}_{event['message_id']}.jpg"
    shutil.copy(media_path, archive_path)
    
    # Log to database
    mcp.call('sqlite', {
        'input': {
            'sql': 'INSERT INTO media_archive (path, from_jid, size) VALUES (?, ?, ?)',
            'params': [archive_path, event['from'], event['media_size']],
            'database': '@user_data/media.db'
        }
    })
    
    return {
        'actions': [
            {'type': 'send_message', 'to': event['from'], 
             'message': {'conversation': f'Media archived: {archive_path}'}}
        ]
    }
```

### 3. Smart Home Control

```python
# Handler for owner only
if event['from'] == '61414505452@s.whatsapp.net':
    text = event['text_content'].lower()
    
    if 'turn on' in text or 'turn off' in text:
        # Send command to hardware via serial
        mcp.call('mcu_serial', {
            'input': {
                'operation': 'send',
                'port': 'COM5',
                'data': f'{text.upper()}\n'
            }
        })
        
        return {
            'actions': [
                {'type': 'send_message', 'to': event['from'], 
                 'message': {'conversation': f'Command sent: {text}'}}
            ]
        }
```

### 4. AI Group Moderator

```python
# Analyze message with LLM for toxicity
analysis = mcp.call('openrouter', {
    'input': {
        'operation': 'chat_completion',
        'model': 'anthropic/claude-3.5-sonnet',
        'messages': [
            {'role': 'system', 'content': 'Analyze if appropriate. Return JSON: {"appropriate": true/false, "reason": "..."}'},
            {'role': 'user', 'content': event['text_content']}
        ]
    }
})

import json
result = json.loads(analysis['content'][0]['text'])

if not result['appropriate']:
    return {
        'actions': [
            # Delete message
            {'type': 'call_method', 'method': 'BuildRevoke', 
             'params': {'chat': event['chat'], 'message_id': event['message_id']}},
            # Warn user privately
            {'type': 'send_message', 'to': event['from'], 
             'message': {'conversation': f'Message removed: {result["reason"]}'}}
        ]
    }
```

---

## 🔧 Available Operations

### Authentication
- `check_login_status` - Check connection status
- `get_qr_code` - Get QR code for pairing (multi-modal)
- `logout` - Disconnect and clear session
- `get_connection_info` - Detailed connection info

### Messaging
- `call_whatsmeow` - Generic dispatcher (call ANY whatsmeow method)
- `get_messages` - Query message history with filters
- `get_method_registry` - Get full method list with examples

### Event Handlers
- `register_handler` - Create event handler
- `list_handlers` - List all handlers
- `get_handler` - Get specific handler details
- `update_handler` - Update handler configuration
- `delete_handler` - Remove handler
- `enable_handler` / `disable_handler` - Toggle handler
- `get_handler_executions` - Query execution logs
- `reload_handlers` - Reload from database

### System
- `get_version` - Tool version and PID
- `get_health_status` - System health check
- `get_error_log` - Recent errors
- `clear_error_state` - Clear non-critical errors
- `get_config` / `set_config` - Configuration management
- `shutdown` - Graceful shutdown

---

## 📋 Available Methods via Generic Dispatcher

### Currently Implemented (9 methods)

1. **SendMessage** - Send text/media messages
2. **SendPresence** - Set online/offline status
3. **SendChatPresence** - Typing/recording indicators
4. **GetUserInfo** - Get user profile information
5. **GetProfilePictureInfo** - Get profile pictures
6. **MarkRead** - Mark messages as read
7. **BuildEdit** - Edit sent messages
8. **BuildRevoke** - Delete/revoke messages
9. **DownloadMediaWithPath** - Download media files

**More methods coming soon:** Groups, contacts, reactions, polls, locations, and more!

---

## 🐍 Handler Best Practices

### ✅ Return Actions, Don't Execute Directly

**CORRECT:**
```python
# Handler returns what to do
return {
    'actions': [
        {'type': 'send_message', 'to': event['from'], 'message': {...}}
    ]
}
```

**WRONG:**
```python
# Don't call WhatsApp directly from handler
mcp.call('whatsapp', {'operation': 'call_whatsmeow', ...})
```

**Exception:** Research queries (READ operations) are OK:
```python
# This is fine - querying WhatsApp state
user_info = mcp.call('whatsapp', {
    'input': {'operation': 'call_whatsmeow', 'method': 'GetUserInfo', ...}
})
```

### 📁 File Management

**Use Python's `tempfile` module:**
```python
import tempfile
temp_dir = tempfile.mkdtemp(prefix='whatsapp_')
# OS cleans up on reboot
```

**Media is pre-downloaded:**
```python
if event.get('has_media'):
    media_path = event['media_path']
    # Process it - Go cleans up after handler completes
```

**Save complex logic to files:**
```python
# Use Python tool's save_script feature
mcp.call('python', {
    'input': {
        'operation': 'save_script',
        'filename': 'my_handler.py',
        'code': '...'
    }
})
# Stored in: @user_data/python_scripts/
```

### 🚨 Safety Configuration

**Always configure limits:**
```json
{
  "limits": {
    "max_executions_per_minute": 10,
    "max_executions_per_hour": 100,
    "max_executions_per_sender_per_hour": 5,
    "cooldown_seconds": 60,
    "timeout_seconds": 30
  },
  "circuit_breaker": {
    "enabled": true,
    "failure_threshold": 5,
    "reset_timeout_seconds": 300
  }
}
```

**Critical filters:**
- Always use `"is_from_me": false` to prevent responding to own messages
- Set reasonable rate limits
- Configure cooldowns between executions

---

## 🛠️ Built-in MCP Tools

**These tools ship with MCP-Link and work immediately:**

| Tool | Description |
|------|-------------|
| 🐍 **Python** | Execute code locally with full MCP tool access |
| 🧠 **SQLite** | Database with semantic search and embeddings |
| 🌐 **Browser** | Automate Chrome: read, click, type, navigate |
| 🤖 **OpenRouter** | Access 500+ AI models (free and paid) |
| 🤗 **HuggingFace** | Run AI models offline (no internet needed) |
| 📚 **Context7** | Pull live documentation for any library |
| 🖥️ **Desktop** | Control Windows apps (click, type, read) |
| 💬 **User** | Show HTML popups for forms, confirmations |
| 🔗 **Remote** | Let external systems offer tools (like this one!) |

**Want more?** Add any third-party MCP tools or build your own!

---

## 📊 Technical Details

### Built With
- **Go 1.24+** - Fast, concurrent, reliable
- **whatsmeow** - WhatsApp Web multidevice API
- **SQLite** - Session storage, handler registry, message history
- **MCP Protocol** - Tool registration and communication
- **Server-Sent Events (SSE)** - Real-time event streaming
- **JSON-RPC** - Command execution protocol

### Performance
- **Zero overhead** on WhatsApp connection
- **Non-blocking architecture** - goroutines for concurrency
- **Panic recovery** - 2-layer crash protection
- **Auto-reconnect** - Exponential backoff for transient errors
- **Efficient media handling** - Temp files, automatic cleanup

### Security
- **Session encryption** - SQLite database with secure storage
- **Rate limiting** - Multiple layers of protection
- **Loop prevention** - Filter own messages, cooldowns, execution tracking
- **Circuit breakers** - Auto-disable failing handlers
- **Execution logging** - Complete audit trail

---

## 📚 Documentation

| File | Purpose |
|------|---------|
| `README.md` | This file - overview and quick start |
| `reverse_mcp_dev_plans.md` | Complete development plan and roadmap |
| `EVENT_DRIVEN_ARCHITECTURE.md` | Event system architecture and examples |
| `HANDLER_BEST_PRACTICES.md` | Best practices for programming handlers |
| `TOOL_DOCUMENTATION_FOR_LLMS.md` | Complete API reference for AI agents |
| `ARCHITECTURE_SUMMARY.md` | Quick reference guide |

---

## 🚀 Roadmap

### ✅ Completed (v2.0.0)
- [x] Authentication (QR code, auto-login, session persistence)
- [x] Send/receive messages (text, media, documents)
- [x] Generic dispatcher (call ANY whatsmeow method)
- [x] Query message history
- [x] Event handler storage (SQLite)
- [x] Event matching engine (11 filter types)
- [x] Action executor (Python + 7 action types)
- [x] Concurrent execution model
- [x] Media handling (download, process, cleanup)
- [x] Safety features (rate limits, circuit breakers, loop prevention)
- [x] Comprehensive error handling
- [x] Panic recovery
- [x] Execution logging

### 🔜 Coming Soon
- [ ] More whatsmeow methods (groups, contacts, reactions)
- [ ] More action types (reactions, edits, forwards)
- [ ] More event types (presence, receipts, group changes)
- [ ] Handler templates (pre-built common handlers)
- [ ] Testing framework (automated handler testing)
- [ ] Performance metrics (execution time analytics)

---

## 🤝 Contributing

**Want to contribute?** PRs welcome! This is the future of AI-powered messaging automation.

### Ideas for Contributors
- Add more whatsmeow methods to `method_registry.json`
- Create example handlers for common use cases
- Improve error messages and logging
- Write integration tests
- Create tutorial videos
- Build handler templates

---

## 📄 License

Proprietary - See [LICENSE](LICENSE) file for details

---

## 👤 Author

Created by [AuraFriday](https://github.com/AuraFriday)  
Part of the MCP-Link ecosystem

---

## 🔗 Links

- **MCP-Link Server**: https://github.com/AuraFriday/mcp-link-server
- **Model Context Protocol**: https://modelcontextprotocol.io
- **whatsmeow Library**: https://github.com/tulir/whatsmeow
- **Fusion 360 MCP Tool**: https://github.com/AuraFriday/Fusion-360-MCP-Server (similar project)

---

## ❓ FAQ

**Q: Does this work with my existing WhatsApp account?**  
A: Yes! Scan the QR code with your WhatsApp mobile app to link.

**Q: Can I run multiple handlers simultaneously?**  
A: Yes! Handlers run concurrently in goroutines. Main loop stays responsive.

**Q: How do I prevent infinite loops?**  
A: Use `"is_from_me": false` filter, configure rate limits, and set cooldowns.

**Q: Can handlers call other MCP tools?**  
A: Yes! Python handlers have full access to all MCP tools (SQLite, OpenRouter, Browser, etc.)

**Q: Does this work offline?**  
A: WhatsApp requires internet, but you can use local AI models (HuggingFace tool) for processing.

**Q: Is this production-ready?**  
A: Yes! Includes panic recovery, error handling, rate limiting, circuit breakers, and execution logging.

**Q: Can I use this with Claude/ChatGPT/etc?**  
A: Yes! Works with any AI that supports MCP protocol.

---

## 🌟 Star This Project!

If you find this useful, please star the repository and share with the WhatsApp automation community!

**Questions?** Open an issue or check the [documentation](reverse_mcp_dev_plans.md)

---

**This is the future of AI-powered WhatsApp automation.** 🚀

