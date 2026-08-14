package security

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

type AlertLevel string

const (
	AlertInfo     AlertLevel = "info"
	AlertWarning  AlertLevel = "warning"
	AlertCritical AlertLevel = "critical"
)

type Alert struct {
	ID           string          `json:"id"`
	Level        AlertLevel      `json:"level"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	ActorID      string          `json:"actor_id,omitempty"`
	TenantID     string          `json:"tenant_id,omitempty"`
	EventType    string          `json:"event_type"`
	Details      json.RawMessage `json:"details,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	Acknowledged bool            `json:"acknowledged"`
}

type AlertHandler func(ctx context.Context, alert Alert)

type AlertEngine struct {
	mu       sync.RWMutex
	alerts   []Alert
	handlers []AlertHandler
}

func NewAlertEngine() *AlertEngine {
	return &AlertEngine{}
}

func (e *AlertEngine) RegisterHandler(h AlertHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers = append(e.handlers, h)
}

func (e *AlertEngine) Emit(ctx context.Context, alert Alert) {
	alert.CreatedAt = time.Now()
	if alert.ID == "" {
		alert.ID = newAlertID()
	}

	e.mu.Lock()
	e.alerts = append(e.alerts, alert)
	handlers := make([]AlertHandler, len(e.handlers))
	copy(handlers, e.handlers)
	e.mu.Unlock()

	for _, h := range handlers {
		h(ctx, alert)
	}
}

func (e *AlertEngine) EmitPrivilegedAction(ctx context.Context, actorID, tenantID, action, resourceType, resourceID string, success bool) {
	level := AlertInfo
	if !success {
		level = AlertCritical
	}
	e.Emit(ctx, Alert{
		Level:       level,
		Title:       "Privileged action detected",
		Description: action + " on " + resourceType + "/" + resourceID,
		ActorID:     actorID,
		TenantID:    tenantID,
		EventType:   "privileged_access",
	})
}

func (e *AlertEngine) EmitRateLimitExceeded(ctx context.Context, clientKey, route string, count int) {
	e.Emit(ctx, Alert{
		Level:       AlertWarning,
		Title:       "Rate limit exceeded",
		Description: "Client " + clientKey + " exceeded rate limit on " + route + " (" + itoaSimple(count) + " requests)",
		EventType:   "rate_limit",
	})
}

func (e *AlertEngine) EmitSecretExposure(ctx context.Context, filePath string, pattern string) {
	e.Emit(ctx, Alert{
		Level:       AlertCritical,
		Title:       "Potential secret exposure detected",
		Description: "Secret pattern '" + pattern + "' found in " + filePath,
		EventType:   "secret_exposure",
	})
}

func (e *AlertEngine) EmitPromptInjection(ctx context.Context, input string, patterns []string) {
	e.Emit(ctx, Alert{
		Level:       AlertCritical,
		Title:       "Prompt injection attempt detected",
		Description: "Blocked prompt injection matching: " + joinPatterns(patterns),
		EventType:   "prompt_injection",
	})
}

func (e *AlertEngine) Alerts() []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Alert, len(e.alerts))
	copy(result, e.alerts)
	return result
}

func (e *AlertEngine) Acknowledge(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.alerts {
		if e.alerts[i].ID == id {
			e.alerts[i].Acknowledged = true
			return
		}
	}
}

func newAlertID() string {
	return "alert-" + time.Now().Format("20060102150405") + "-" + randomHex(8)
}

func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func joinPatterns(patterns []string) string {
	if len(patterns) == 0 {
		return ""
	}
	result := patterns[0]
	for i := 1; i < len(patterns); i++ {
		result += ", " + patterns[i]
	}
	return result
}

func randomHex(n int) string {
	const hexChars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hexChars[time.Now().UnixNano()%16]
	}
	return string(b)
}
