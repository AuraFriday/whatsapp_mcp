package main

import "os"

const (
	ToolVersion     = "2.0.0"
	ToolName        = "WhatsApp MCP Tool"
	ToolDescription = "AI-powered WhatsApp client with generic dispatcher"
)

// GetVersionInfo returns version information
func GetVersionInfo() map[string]interface{} {
	return map[string]interface{}{
		"version":     ToolVersion,
		"name":        ToolName,
		"description": ToolDescription,
		"pid":         os.Getpid(),
		"features": []string{
			"Generic method dispatcher (call ANY whatsmeow method)",
			"9+ pre-configured operations",
			"Auto-login with session persistence",
			"QR code authentication with popup",
			"Comprehensive error handling with panic recovery",
			"Type-safe parameter conversion",
			"Message templates and examples",
		},
	}
}

