package main

import (
  "regexp"
  "strings"
  "sync"
  "time"
)

// EventMatcher handles matching events against handler filters
type EventMatcher struct {
  database      *Database
  handlers      []map[string]interface{}
  handlersMutex sync.RWMutex
  rateLimits    map[string]*RateLimiter
  limitsMutex   sync.RWMutex
}

// RateLimiter tracks execution counts for rate limiting
type RateLimiter struct {
  perMinuteCounts map[int64]int
  perHourCounts   map[int64]int
  perSenderCounts map[string]map[int64]int
  lastExecution   time.Time
  mutex           sync.Mutex
}

// NewEventMatcher creates a new event matcher
func NewEventMatcher(database *Database) *EventMatcher {
  return &EventMatcher{
    database:   database,
    handlers:   []map[string]interface{}{},
    rateLimits: make(map[string]*RateLimiter),
  }
}

// LoadHandlers loads all enabled handlers from database
func (em *EventMatcher) LoadHandlers() error {
  em.handlersMutex.Lock()
  defer em.handlersMutex.Unlock()

  handlers, err := em.database.ListHandlers(true) // enabled only
  if err != nil {
    return err
  }

  // Load full handler details
  fullHandlers := make([]map[string]interface{}, 0)
  for _, h := range handlers {
    handlerID := h["handler_id"].(string)
    fullHandler, err := em.database.GetHandler(handlerID)
    if err != nil {
      continue // skip invalid handlers
    }
    fullHandlers = append(fullHandlers, fullHandler)
  }

  em.handlers = fullHandlers
  return nil
}

// MatchEvent finds all handlers that match the given event
func (em *EventMatcher) MatchEvent(event map[string]interface{}) []map[string]interface{} {
  em.handlersMutex.RLock()
  defer em.handlersMutex.RUnlock()

  var matches []map[string]interface{}

  for _, handler := range em.handlers {
    // Check if handler is enabled
    if enabled, ok := handler["enabled"].(bool); !ok || !enabled {
      continue
    }

    // Check circuit breaker
    if em.isCircuitBreakerOpen(handler) {
      continue
    }

    // Check rate limits
    if !em.checkRateLimits(handler, event) {
      continue
    }

    // Check cooldown
    if !em.checkCooldown(handler) {
      continue
    }

    // Check event filter
    if em.matchesFilter(handler, event) {
      matches = append(matches, handler)
    }
  }

  // Sort by priority (highest first)
  sortHandlersByPriority(matches)

  return matches
}

// matchesFilter checks if event matches handler's filter
func (em *EventMatcher) matchesFilter(handler map[string]interface{}, event map[string]interface{}) bool {
  filter, ok := handler["event_filter"].(map[string]interface{})
  if !ok {
    return false
  }

  // Check event_types
  if eventTypes, ok := filter["event_types"].([]interface{}); ok && len(eventTypes) > 0 {
    eventType, _ := event["event_type"].(string)
    if !containsString(eventTypes, eventType) {
      return false
    }
  }

  // Check is_from_me
  if isFromMe, ok := filter["is_from_me"].(bool); ok {
    eventIsFromMe, _ := event["is_from_me"].(bool)
    if isFromMe != eventIsFromMe {
      return false
    }
  }

  // Check message_types (for message events)
  if messageTypes, ok := filter["message_types"].([]interface{}); ok && len(messageTypes) > 0 {
    msgType, _ := event["message_type"].(string)
    if !containsString(messageTypes, msgType) {
      return false
    }
  }

  // Check from_jids
  if fromJIDs, ok := filter["from_jids"].([]interface{}); ok && len(fromJIDs) > 0 {
    fromJID, _ := event["from"].(string)
    if !containsString(fromJIDs, fromJID) {
      return false
    }
  }

  // Check chat_jids
  if chatJIDs, ok := filter["chat_jids"].([]interface{}); ok && len(chatJIDs) > 0 {
    chatJID, _ := event["chat"].(string)
    if !containsString(chatJIDs, chatJID) {
      return false
    }
  }

  // Check is_group
  if isGroup, ok := filter["is_group"].(bool); ok {
    eventIsGroup, _ := event["is_group"].(bool)
    if isGroup != eventIsGroup {
      return false
    }
  }

  // Check group_jids
  if groupJIDs, ok := filter["group_jids"].([]interface{}); ok && len(groupJIDs) > 0 {
    chatJID, _ := event["chat"].(string)
    isGroup, _ := event["is_group"].(bool)
    if !isGroup || !containsString(groupJIDs, chatJID) {
      return false
    }
  }

  // Check has_media
  if hasMedia, ok := filter["has_media"].(bool); ok {
    eventHasMedia := false
    if mediaType, ok := event["media_type"].(string); ok && mediaType != "" {
      eventHasMedia = true
    }
    if hasMedia != eventHasMedia {
      return false
    }
  }

  // Check has_quoted_message
  if hasQuoted, ok := filter["has_quoted_message"].(bool); ok {
    eventHasQuoted := false
    if quotedID, ok := event["quoted_message_id"].(string); ok && quotedID != "" {
      eventHasQuoted = true
    }
    if hasQuoted != eventHasQuoted {
      return false
    }
  }

  // Check text_contains
  if textContains, ok := filter["text_contains"].([]interface{}); ok && len(textContains) > 0 {
    textContent, _ := event["text_content"].(string)
    textContent = strings.ToLower(textContent)
    matched := false
    for _, keyword := range textContains {
      if keywordStr, ok := keyword.(string); ok {
        if strings.Contains(textContent, strings.ToLower(keywordStr)) {
          matched = true
          break
        }
      }
    }
    if !matched {
      return false
    }
  }

  // Check text_regex
  if textRegex, ok := filter["text_regex"].(string); ok && textRegex != "" {
    textContent, _ := event["text_content"].(string)
    matched, err := regexp.MatchString(textRegex, textContent)
    if err != nil || !matched {
      return false
    }
  }

  return true
}

// checkRateLimits checks if handler's rate limits allow execution
func (em *EventMatcher) checkRateLimits(handler map[string]interface{}, event map[string]interface{}) bool {
  handlerID := handler["handler_id"].(string)

  em.limitsMutex.Lock()
  limiter, exists := em.rateLimits[handlerID]
  if !exists {
    limiter = &RateLimiter{
      perMinuteCounts: make(map[int64]int),
      perHourCounts:   make(map[int64]int),
      perSenderCounts: make(map[string]map[int64]int),
    }
    em.rateLimits[handlerID] = limiter
  }
  em.limitsMutex.Unlock()

  limiter.mutex.Lock()
  defer limiter.mutex.Unlock()

  now := time.Now()
  currentMinute := now.Unix() / 60
  currentHour := now.Unix() / 3600

  // Check per-minute limit
  if maxPerMin, ok := handler["max_executions_per_minute"].(int64); ok && maxPerMin > 0 {
    if limiter.perMinuteCounts[currentMinute] >= int(maxPerMin) {
      return false
    }
  }

  // Check per-hour limit
  if maxPerHour, ok := handler["max_executions_per_hour"].(int64); ok && maxPerHour > 0 {
    if limiter.perHourCounts[currentHour] >= int(maxPerHour) {
      return false
    }
  }

  // Check per-sender-per-hour limit
  if maxPerSenderHour, ok := handler["max_executions_per_sender_per_hour"].(int64); ok && maxPerSenderHour > 0 {
    fromJID, _ := event["from"].(string)
    if fromJID != "" {
      if limiter.perSenderCounts[fromJID] == nil {
        limiter.perSenderCounts[fromJID] = make(map[int64]int)
      }
      if limiter.perSenderCounts[fromJID][currentHour] >= int(maxPerSenderHour) {
        return false
      }
    }
  }

  return true
}

// RecordExecution records an execution for rate limiting
func (em *EventMatcher) RecordExecution(handlerID string, event map[string]interface{}) {
  em.limitsMutex.Lock()
  limiter, exists := em.rateLimits[handlerID]
  if !exists {
    limiter = &RateLimiter{
      perMinuteCounts: make(map[int64]int),
      perHourCounts:   make(map[int64]int),
      perSenderCounts: make(map[string]map[int64]int),
    }
    em.rateLimits[handlerID] = limiter
  }
  em.limitsMutex.Unlock()

  limiter.mutex.Lock()
  defer limiter.mutex.Unlock()

  now := time.Now()
  currentMinute := now.Unix() / 60
  currentHour := now.Unix() / 3600

  // Increment counters
  limiter.perMinuteCounts[currentMinute]++
  limiter.perHourCounts[currentHour]++

  fromJID, _ := event["from"].(string)
  if fromJID != "" {
    if limiter.perSenderCounts[fromJID] == nil {
      limiter.perSenderCounts[fromJID] = make(map[int64]int)
    }
    limiter.perSenderCounts[fromJID][currentHour]++
  }

  limiter.lastExecution = now

  // Cleanup old entries (older than 2 hours)
  oldestMinute := (now.Unix() - 7200) / 60
  oldestHour := (now.Unix() - 7200) / 3600

  for minute := range limiter.perMinuteCounts {
    if minute < oldestMinute {
      delete(limiter.perMinuteCounts, minute)
    }
  }

  for hour := range limiter.perHourCounts {
    if hour < oldestHour {
      delete(limiter.perHourCounts, hour)
    }
  }

  for sender, counts := range limiter.perSenderCounts {
    for hour := range counts {
      if hour < oldestHour {
        delete(counts, hour)
      }
    }
    if len(counts) == 0 {
      delete(limiter.perSenderCounts, sender)
    }
  }
}

// checkCooldown checks if enough time has passed since last execution
func (em *EventMatcher) checkCooldown(handler map[string]interface{}) bool {
  cooldownSeconds, ok := handler["cooldown_seconds"].(int64)
  if !ok || cooldownSeconds <= 0 {
    return true
  }

  handlerID := handler["handler_id"].(string)

  em.limitsMutex.RLock()
  limiter, exists := em.rateLimits[handlerID]
  em.limitsMutex.RUnlock()

  if !exists {
    return true
  }

  limiter.mutex.Lock()
  lastExec := limiter.lastExecution
  limiter.mutex.Unlock()

  if lastExec.IsZero() {
    return true
  }

  elapsed := time.Since(lastExec)
  return elapsed.Seconds() >= float64(cooldownSeconds)
}

// isCircuitBreakerOpen checks if handler's circuit breaker is open
func (em *EventMatcher) isCircuitBreakerOpen(handler map[string]interface{}) bool {
  // Circuit breaker enabled?
  cbEnabled, ok := handler["circuit_breaker_enabled"].(bool)
  if !ok || !cbEnabled {
    return false
  }

  // Get circuit breaker state
  cbState, _ := handler["circuit_breaker_state"].(string)
  if cbState != "open" {
    return false
  }

  // Check if reset timeout has passed
  lastErrorTime, ok := handler["last_error_time"].(string)
  if !ok {
    return false
  }

  lastError, err := time.Parse(time.RFC3339, lastErrorTime)
  if err != nil {
    return false
  }

  resetSeconds, ok := handler["circuit_breaker_reset_seconds"].(int64)
  if !ok {
    resetSeconds = 300 // default 5 minutes
  }

  elapsed := time.Since(lastError)
  if elapsed.Seconds() >= float64(resetSeconds) {
    // Time to try again - circuit breaker should reset
    // (This will be handled by the executor when it succeeds)
    return false
  }

  return true
}

// UpdateCircuitBreaker updates the circuit breaker state based on execution result
func (em *EventMatcher) UpdateCircuitBreaker(handlerID string, success bool) error {
  handler, err := em.database.GetHandler(handlerID)
  if err != nil {
    return err
  }

  cbEnabled, ok := handler["circuit_breaker_enabled"].(bool)
  if !ok || !cbEnabled {
    return nil
  }

  if success {
    // Reset circuit breaker on success
    handler["circuit_breaker_state"] = "closed"
    return em.database.SaveHandler(handler)
  }

  // On failure, check if we need to open the circuit breaker
  totalErrors, _ := handler["total_errors"].(int)
  threshold, ok := handler["circuit_breaker_threshold"].(int64)
  if !ok {
    threshold = 5
  }

  if totalErrors >= int(threshold) {
    handler["circuit_breaker_state"] = "open"
    return em.database.SaveHandler(handler)
  }

  return nil
}

// Helper functions

func containsString(slice []interface{}, str string) bool {
  for _, item := range slice {
    if itemStr, ok := item.(string); ok && itemStr == str {
      return true
    }
  }
  return false
}

func sortHandlersByPriority(handlers []map[string]interface{}) {
  // Simple bubble sort by priority (descending)
  n := len(handlers)
  for i := 0; i < n-1; i++ {
    for j := 0; j < n-i-1; j++ {
      priority1, _ := handlers[j]["priority"].(int)
      priority2, _ := handlers[j+1]["priority"].(int)
      if priority1 < priority2 {
        handlers[j], handlers[j+1] = handlers[j+1], handlers[j]
      }
    }
  }
}

