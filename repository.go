package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuditFilters defines the search criteria for listing audit log entries.
type AuditFilters struct {
	UserID        string
	CorrelationID string
	Resource      string
	Action        Action
	From          *time.Time
	To            *time.Time
	Limit         int
	Offset        int
}

// AuditRepository defines the contract for audit log persistence.
type AuditRepository interface {
	Create(ctx context.Context, entry *AuditLog) error
	GetByID(ctx context.Context, id uuid.UUID) (*AuditLog, error)
	List(ctx context.Context, filters AuditFilters) ([]AuditLog, int, error)
}
