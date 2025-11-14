package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/encoding/protojson"
)

//go:embed method_registry.json
var methodRegistryJSON []byte

// MethodRegistry holds all available whatsmeow methods
type MethodRegistry struct {
	Methods          map[string]MethodSpec `json:"methods"`
	MessageTemplates map[string]interface{} `json:"message_templates"`
	TypeNotes        map[string]string      `json:"type_notes"`
}

// MethodSpec defines a callable method
type MethodSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Params      []ParamSpec            `json:"params"`
	Returns     map[string]string      `json:"returns"`
	Example     map[string]interface{} `json:"example"`
	Notes       string                 `json:"notes"`
}

// ParamSpec defines a parameter
type ParamSpec struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Description string      `json:"description"`
	Example     interface{} `json:"example"`
	Enum        []string    `json:"enum,omitempty"`
	Notes       string      `json:"notes,omitempty"`
}

var globalMethodRegistry *MethodRegistry

// LoadMethodRegistry loads the method registry from embedded JSON
func LoadMethodRegistry() error {
	globalMethodRegistry = &MethodRegistry{}
	if err := json.Unmarshal(methodRegistryJSON, globalMethodRegistry); err != nil {
		return fmt.Errorf("failed to load method registry: %w", err)
	}
	return nil
}

// Type converters

func convertToContext(v interface{}) (reflect.Value, error) {
	// Always return background context
	// TODO: Support timeout contexts via {"timeout": "30s"}
	return reflect.ValueOf(context.Background()), nil
}

func convertToJID(v interface{}) (reflect.Value, error) {
	str, ok := v.(string)
	if !ok {
		return reflect.Value{}, fmt.Errorf("JID must be string, got %T", v)
	}

	// If already contains @, parse as-is
	if strings.Contains(str, "@") {
		jid, err := types.ParseJID(str)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("invalid JID: %w", err)
		}
		return reflect.ValueOf(jid), nil
	}

	// Otherwise, assume phone number and add @s.whatsapp.net
	// Remove any non-digit characters
	phone := regexp.MustCompile(`[^\d+]`).ReplaceAllString(str, "")
	
	if len(phone) < 7 {
		return reflect.Value{}, fmt.Errorf("invalid phone number: too short (%s)", phone)
	}

	// Remove leading + if present
	phone = strings.TrimPrefix(phone, "+")

	jid := types.NewJID(phone, types.DefaultUserServer)
	return reflect.ValueOf(jid), nil
}

func convertToJIDSlice(v interface{}) (reflect.Value, error) {
	arr, ok := v.([]interface{})
	if !ok {
		return reflect.Value{}, fmt.Errorf("JID array must be array, got %T", v)
	}

	jids := make([]types.JID, len(arr))
	for i, item := range arr {
		jidVal, err := convertToJID(item)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("JID[%d]: %w", i, err)
		}
		jids[i] = jidVal.Interface().(types.JID)
	}

	return reflect.ValueOf(jids), nil
}

func convertToProtoMessage(v interface{}, protoType string) (reflect.Value, error) {
	switch protoType {
	case "proto:waE2E.Message":
		msg := &waE2E.Message{}
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("failed to marshal message: %w", err)
		}
		
		// Use protojson for proper protobuf unmarshaling
		if err := protojson.Unmarshal(jsonBytes, msg); err != nil {
			return reflect.Value{}, fmt.Errorf("failed to unmarshal proto message: %w", err)
		}
		
		return reflect.ValueOf(msg), nil
		
	default:
		return reflect.Value{}, fmt.Errorf("unknown proto type: %s", protoType)
	}
}

func convertToTime(v interface{}) (reflect.Value, error) {
	str, ok := v.(string)
	if !ok {
		return reflect.Value{}, fmt.Errorf("time must be ISO8601 string, got %T", v)
	}

	t, err := time.Parse(time.RFC3339, str)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("invalid time format (use ISO8601): %w", err)
	}

	return reflect.ValueOf(t), nil
}

func convertToDuration(v interface{}) (reflect.Value, error) {
	str, ok := v.(string)
	if !ok {
		return reflect.Value{}, fmt.Errorf("duration must be string like '30s', got %T", v)
	}

	d, err := time.ParseDuration(str)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("invalid duration: %w", err)
	}

	return reflect.ValueOf(d), nil
}

func convertToString(v interface{}) (reflect.Value, error) {
	str, ok := v.(string)
	if !ok {
		return reflect.Value{}, fmt.Errorf("expected string, got %T", v)
	}
	return reflect.ValueOf(str), nil
}

func convertToChatPresence(v interface{}) (reflect.Value, error) {
	str, ok := v.(string)
	if !ok {
		return reflect.Value{}, fmt.Errorf("chat presence must be string, got %T", v)
	}
	
	var presence types.ChatPresence
	switch str {
	case "composing":
		presence = types.ChatPresenceComposing
	case "paused":
		presence = types.ChatPresencePaused
	default:
		return reflect.Value{}, fmt.Errorf("invalid chat presence: %s (must be 'composing' or 'paused')", str)
	}
	
	return reflect.ValueOf(presence), nil
}

func convertToChatPresenceMedia(v interface{}) (reflect.Value, error) {
	str, ok := v.(string)
	if !ok {
		return reflect.Value{}, fmt.Errorf("chat presence media must be string, got %T", v)
	}
	
	var media types.ChatPresenceMedia
	switch str {
	case "text", "":
		media = types.ChatPresenceMediaText
	case "audio":
		media = types.ChatPresenceMediaAudio
	default:
		return reflect.Value{}, fmt.Errorf("invalid chat presence media: %s (must be 'text' or 'audio')", str)
	}
	
	return reflect.ValueOf(media), nil
}

func convertToPresence(v interface{}) (reflect.Value, error) {
	str, ok := v.(string)
	if !ok {
		return reflect.Value{}, fmt.Errorf("presence must be string, got %T", v)
	}
	
	var presence types.Presence
	switch str {
	case "available":
		presence = types.PresenceAvailable
	case "unavailable":
		presence = types.PresenceUnavailable
	default:
		return reflect.Value{}, fmt.Errorf("invalid presence: %s (must be 'available' or 'unavailable')", str)
	}
	
	return reflect.ValueOf(presence), nil
}

func convertToInt(v interface{}) (reflect.Value, error) {
	switch val := v.(type) {
	case float64:
		return reflect.ValueOf(int(val)), nil
	case int:
		return reflect.ValueOf(val), nil
	default:
		return reflect.Value{}, fmt.Errorf("expected number, got %T", v)
	}
}

func convertToBool(v interface{}) (reflect.Value, error) {
	b, ok := v.(bool)
	if !ok {
		return reflect.Value{}, fmt.Errorf("expected bool, got %T", v)
	}
	return reflect.ValueOf(b), nil
}

func convertToStringSlice(v interface{}) (reflect.Value, error) {
	arr, ok := v.([]interface{})
	if !ok {
		return reflect.Value{}, fmt.Errorf("expected array, got %T", v)
	}

	strs := make([]string, len(arr))
	for i, item := range arr {
		str, ok := item.(string)
		if !ok {
			return reflect.Value{}, fmt.Errorf("array[%d]: expected string, got %T", i, item)
		}
		strs[i] = str
	}

	return reflect.ValueOf(strs), nil
}

// convertParam converts a JSON value to the appropriate Go type
func convertParam(paramSpec ParamSpec, value interface{}) (reflect.Value, error) {
	paramType := paramSpec.Type

	// Handle proto types
	if strings.HasPrefix(paramType, "proto:") {
		return convertToProtoMessage(value, paramType)
	}

	// Handle slice types
	if strings.HasPrefix(paramType, "[]") {
		baseType := strings.TrimPrefix(paramType, "[]")
		switch baseType {
		case "jid":
			return convertToJIDSlice(value)
		case "string":
			return convertToStringSlice(value)
		default:
			return reflect.Value{}, fmt.Errorf("unsupported slice type: %s", paramType)
		}
	}

	// Handle base types
	switch paramType {
	case "context":
		return convertToContext(value)
	case "jid":
		return convertToJID(value)
	case "string":
		return convertToString(value)
	case "int":
		return convertToInt(value)
	case "bool":
		return convertToBool(value)
	case "time":
		return convertToTime(value)
	case "duration":
		return convertToDuration(value)
	case "chatpresence":
		return convertToChatPresence(value)
	case "chatpresencemedia":
		return convertToChatPresenceMedia(value)
	case "presence":
		return convertToPresence(value)
	case "interface", "object":
		// Pass through as-is (for complex types we don't yet support)
		return reflect.ValueOf(value), nil
	default:
		return reflect.Value{}, fmt.Errorf("unsupported type: %s", paramType)
	}
}

// CallWhatsmeowMethod calls a whatsmeow client method via reflection
func CallWhatsmeowMethod(methodName string, params map[string]interface{}) *OperationResult {
	// Panic recovery - catch any panics during reflection/execution
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Panic in CallWhatsmeowMethod: %v\n", r)
			// Note: We can't modify the return value here, but the panic won't crash the server
		}
	}()

	// Check if method exists in registry
	methodSpec, exists := globalMethodRegistry.Methods[methodName]
	if !exists {
		return &OperationResult{
			Success: false,
			Error:   fmt.Sprintf("unknown method: %s", methodName),
		}
	}

	// Check if client is available
	if global_whatsapp_client == nil || global_whatsapp_client.client == nil {
		return &OperationResult{
			Success: false,
			Error:   "WhatsApp client not initialized or not connected",
		}
	}

	// Get method via reflection
	client := global_whatsapp_client.client
	method := reflect.ValueOf(client).MethodByName(methodName)
	if !method.IsValid() {
		return &OperationResult{
			Success: false,
			Error:   fmt.Sprintf("method %s not found on client", methodName),
		}
	}

	// Convert parameters
	args := make([]reflect.Value, 0)
	
	// First parameter is always context for most methods
	// We'll add it automatically if needed
	methodType := method.Type()
	if methodType.NumIn() > 0 && methodType.In(0).String() == "context.Context" {
		args = append(args, reflect.ValueOf(context.Background()))
	}

	// Convert remaining parameters based on spec
	for _, paramSpec := range methodSpec.Params {
		if paramSpec.Name == "ctx" {
			continue // Already handled
		}

		paramValue, exists := params[paramSpec.Name]
		if !exists {
			if paramSpec.Required {
				return &OperationResult{
					Success: false,
					Error:   fmt.Sprintf("required parameter '%s' missing", paramSpec.Name),
				}
			}
			// Use zero value for optional params
			continue
		}

		arg, err := convertParam(paramSpec, paramValue)
		if err != nil {
			return &OperationResult{
				Success: false,
				Error:   fmt.Sprintf("parameter '%s': %v", paramSpec.Name, err),
			}
		}

		args = append(args, arg)
	}

	// Call the method with panic recovery
	var results []reflect.Value
	var callPanic interface{}
	
	func() {
		defer func() {
			if r := recover(); r != nil {
				callPanic = r
			}
		}()
		results = method.Call(args)
	}()
	
	// If panic occurred, return detailed error
	if callPanic != nil {
		return &OperationResult{
			Success: false,
			Error:   fmt.Sprintf("method call panicked: %v (this usually means type mismatch - check parameter types in registry)", callPanic),
			Data: map[string]interface{}{
				"panic_value": fmt.Sprintf("%v", callPanic),
				"method":      methodName,
				"params":      params,
				"hint":        "The panic suggests a type conversion error. Check that all parameters match the expected Go types.",
			},
		}
	}

	// Handle return values
	// Most methods return (result, error) or just error
	if len(results) == 0 {
		return &OperationResult{
			Success: true,
			Message: fmt.Sprintf("%s executed successfully", methodName),
		}
	}

	// Check last return value for error
	lastResult := results[len(results)-1]
	if lastResult.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		if !lastResult.IsNil() {
			err := lastResult.Interface().(error)
			return &OperationResult{
				Success: false,
				Error:   fmt.Sprintf("%s failed: %v", methodName, err),
			}
		}
	}

	// Convert first return value to map (if not error-only)
	if len(results) > 1 {
		firstResult := results[0].Interface()
		data := convertToMap(firstResult)
		
		return &OperationResult{
			Success: true,
			Message: fmt.Sprintf("%s executed successfully", methodName),
			Data:    data,
		}
	}

	return &OperationResult{
		Success: true,
		Message: fmt.Sprintf("%s executed successfully", methodName),
	}
}

// convertToMap converts a value to a map for JSON serialization
func convertToMap(v interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	
	// Handle nil
	if v == nil {
		return result
	}

	// Try to marshal/unmarshal via JSON (works for most types)
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		result["raw"] = fmt.Sprintf("%+v", v)
		return result
	}

	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		result["raw"] = string(jsonBytes)
	}

	return result
}

