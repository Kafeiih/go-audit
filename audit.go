// Package audit provides a reusable audit logging system with context
// propagation, a generic repository interface, and pluggable middleware.
package audit

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ---------- Context propagation ----------

type contextKey struct{ name string }

var (
	infoKey = contextKey{"audit-info"}
	skipKey = contextKey{"skip-audit"}
)

// Info holds all audit context for a request.
type Info struct {
	UserID        string
	Username      string
	CorrelationID string
	Resource      string
	ResourceID    string
	IP            string
	UserAgent     string
}

// WithInfo attaches audit info to the context.
func WithInfo(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, infoKey, info)
}

// InfoFrom extracts audit info from context. Returns nil if absent.
func InfoFrom(ctx context.Context) *Info {
	i, ok := ctx.Value(infoKey).(Info)
	if !ok {
		return nil
	}
	return &i
}

// WithSkipAudit marks the context to skip audit triggers (e.g. bulk import).
func WithSkipAudit(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipKey, true)
}

// ShouldSkip reports whether audit should be skipped for this context.
func ShouldSkip(ctx context.Context) bool {
	v, _ := ctx.Value(skipKey).(bool)
	return v
}

// ---------- Action ----------

// Action represents the type of action recorded in the audit log.
type Action string

const (
	ActionCreate Action = "CREATE"
	ActionUpdate Action = "UPDATE"
	ActionDelete Action = "DELETE"
	ActionRead   Action = "READ"
)

// IsValid reports whether a is a known audit action.
func (a Action) IsValid() bool {
	switch a {
	case ActionCreate, ActionUpdate, ActionDelete, ActionRead:
		return true
	}
	return false
}

// ---------- AuditLog entity ----------

// AuditLog represents an immutable audit log entry.
type AuditLog struct {
	ID            uuid.UUID
	UserID        string
	Username      string
	CorrelationID string
	Action        Action
	Resource      string
	ResourceID    string
	IP            string
	UserAgent     string
	Details       map[string]any

	// ChangedFields stores field-level deltas when available.
	ChangedFields map[string]any

	CreatedAt time.Time
}

// NewAuditLog creates a new audit log entry with basic validation.
// Accepts an optional nowFn to allow injecting a clock for testing.
func NewAuditLog(
	userID, username string,
	correlationID string,
	action Action,
	resource, resourceID string,
	ip, userAgent string,
	details map[string]any,
	nowFn ...func() time.Time,
) (*AuditLog, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}
	if !action.IsValid() {
		return nil, errors.New("invalid action")
	}
	if resource == "" {
		return nil, errors.New("resource is required")
	}

	now := time.Now
	if len(nowFn) > 0 && nowFn[0] != nil {
		now = nowFn[0]
	}

	if details == nil {
		details = map[string]any{}
	}

	changedFields := map[string]any{}
	if raw, ok := details["changed_fields"]; ok {
		if m, ok := raw.(map[string]any); ok {
			changedFields = m
		}
	}

	return &AuditLog{
		ID:            uuid.New(),
		UserID:        userID,
		Username:      username,
		CorrelationID: correlationID,
		Action:        action,
		Resource:      resource,
		ResourceID:    resourceID,
		IP:            ip,
		UserAgent:     userAgent,
		Details:       details,
		ChangedFields: changedFields,
		CreatedAt:     now(),
	}, nil
}
