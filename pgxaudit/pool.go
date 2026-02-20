// Package pgxaudit provides a PostgreSQL implementation of the audit
// repository and an AuditPool wrapper that sets session variables for
// DB-level audit triggers.
package pgxaudit

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	audit "github.com/kafeiih/go-audit"
)

// DB abstracts the pgxpool.Pool methods used by repositories.
// Both *pgxpool.Pool and *AuditPool satisfy this interface.
type DB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// AuditPool wraps a pgxpool.Pool and automatically sets audit session
// variables on write operations via SET LOCAL inside a transaction.
// Read operations pass through directly.
type AuditPool struct {
	pool *pgxpool.Pool
}

// NewAuditPool creates a new AuditPool wrapping the given pool.
func NewAuditPool(pool *pgxpool.Pool) *AuditPool {
	return &AuditPool{pool: pool}
}

// Query passes through to the underlying pool (reads don't need audit context).
func (p *AuditPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return p.pool.Query(ctx, sql, args...)
}

// QueryRow passes through to the underlying pool.
func (p *AuditPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return p.pool.QueryRow(ctx, sql, args...)
}

// Exec wraps write operations in a transaction with audit session variables.
// If no audit info is in context or skip_audit is set, passes through directly.
func (p *AuditPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	info := audit.InfoFrom(ctx)
	if info == nil || audit.ShouldSkip(ctx) {
		return p.pool.Exec(ctx, sql, args...)
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer tx.Rollback(ctx)

	configs := map[string]string{
		"app.user_id":        info.UserID,
		"app.username":       info.Username,
		"app.correlation_id": info.CorrelationID,
		"app.resource":       info.Resource,
		"app.resource_id":    info.ResourceID,
		"app.ip":             info.IP,
		"app.user_agent":     info.UserAgent,
	}
	for key, val := range configs {
		if _, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", key, val); err != nil {
			return pgconn.CommandTag{}, err
		}
	}

	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return tag, err
	}

	if err := tx.Commit(ctx); err != nil {
		return pgconn.CommandTag{}, err
	}

	return tag, nil
}
