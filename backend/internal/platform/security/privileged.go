package security

import (
	"context"
	"encoding/json"
	"time"
)

type PrivilegedAccessEvent struct {
	ID        string
	ActorID   string
	TenantID  string
	Action    Action
	Resource  Resource
	MFAUsed   bool
	Success   bool
	Details   json.RawMessage
	Timestamp time.Time
}

type PrivilegedAccessLogger interface {
	Log(ctx context.Context, event PrivilegedAccessEvent) error
}

type NoOpPrivilegedAccessLogger struct {
	events []PrivilegedAccessEvent
}

func NewNoOpPrivilegedAccessLogger() *NoOpPrivilegedAccessLogger {
	return &NoOpPrivilegedAccessLogger{}
}

func (l *NoOpPrivilegedAccessLogger) Log(ctx context.Context, event PrivilegedAccessEvent) error {
	if !isPrivilegedAction(string(event.Action)) {
		return nil
	}
	l.events = append(l.events, event)
	return nil
}

func (l *NoOpPrivilegedAccessLogger) Events() []PrivilegedAccessEvent {
	return l.events
}
