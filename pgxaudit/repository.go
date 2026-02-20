package pgxaudit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	audit "github.com/kafeiih/go-audit"
)

// PostgresRepo implements audit.AuditRepository using any DB-compatible pool.
type PostgresRepo struct {
	pool DB
}

// NewPostgresRepo creates a new PostgresRepo.
// It accepts any DB implementation (*pgxpool.Pool, *AuditPool, or a test mock).
func NewPostgresRepo(pool DB) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

func (r *PostgresRepo) Create(ctx context.Context, b *audit.AuditLog) error {
	detailsJSON, err := json.Marshal(b.Details)
	if err != nil {
		return fmt.Errorf("serializing details: %w", err)
	}

	changedFields := b.ChangedFields
	if changedFields == nil {
		changedFields = map[string]any{}
	}
	changedFieldsJSON, err := json.Marshal(changedFields)
	if err != nil {
		return fmt.Errorf("serializing changed_fields: %w", err)
	}

	_, err = r.pool.Exec(ctx,
		`INSERT INTO audit.audit_logentry (id, user_id, username, correlation_id, action, resource, resource_id, ip, user_agent, details, changed_fields, created_at)
		 	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		b.ID, b.UserID, b.Username, b.CorrelationID, string(b.Action), b.Resource, b.ResourceID,
		b.IP, b.UserAgent, detailsJSON, changedFieldsJSON, b.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting audit log entry: %w", err)
	}

	return nil
}

func (r *PostgresRepo) GetByID(ctx context.Context, id uuid.UUID) (*audit.AuditLog, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, username, correlation_id, action, resource, resource_id, ip, user_agent, details, changed_fields, created_at
		 	FROM audit.audit_logentry WHERE id = $1`, id,
	)

	b, err := scanAuditLog(row)
	if err != nil {
		return nil, fmt.Errorf("fetching audit log by ID: %w", err)
	}

	return b, nil
}

func (r *PostgresRepo) List(ctx context.Context, f audit.AuditFilters) ([]audit.AuditLog, int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, username, correlation_id, action, resource, resource_id, ip, user_agent, details, changed_fields, created_at,
				count(*) OVER()::INT AS total
			FROM audit.audit_logentry
			WHERE ($1::TEXT IS NULL OR user_id  = $1)
				AND ($2::TEXT IS NULL OR correlation_id = $2)
				AND ($3::TEXT IS NULL OR resource = $3)
				AND ($4::TEXT IS NULL OR action   = $4)
				AND ($5::TIMESTAMPTZ IS NULL OR created_at >= $5)
				AND ($6::TIMESTAMPTZ IS NULL OR created_at <= $6)
			ORDER BY created_at DESC
			LIMIT $7 OFFSET $8`,
		nullString(f.UserID), nullString(f.CorrelationID), nullString(f.Resource), nullString(string(f.Action)),
		f.From, f.To,
		f.Limit, f.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing audit log entries: %w", err)
	}
	defer rows.Close()

	var items []audit.AuditLog
	var total int
	for rows.Next() {
		b, err := scanAuditLogWithTotal(rows, &total)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning audit log entry: %w", err)
		}
		items = append(items, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating rows: %w", err)
	}

	return items, total, nil
}

// scanner abstracts pgx.Row and pgx.Rows for shared scan logic.
type scanner interface {
	Scan(dest ...any) error
}

func scanAuditLogWithTotal(s scanner, total *int) (*audit.AuditLog, error) {
	var b audit.AuditLog
	var action string
	var detailsJSON []byte
	var changedFieldsJSON []byte

	err := s.Scan(
		&b.ID, &b.UserID, &b.Username, &b.CorrelationID, &action,
		&b.Resource, &b.ResourceID, &b.IP, &b.UserAgent,
		&detailsJSON, &changedFieldsJSON, &b.CreatedAt, total,
	)
	if err != nil {
		return nil, err
	}

	b.Action = audit.Action(action)
	if err := json.Unmarshal(detailsJSON, &b.Details); err != nil {
		return nil, fmt.Errorf("deserializing details: %w", err)
	}
	if err := json.Unmarshal(changedFieldsJSON, &b.ChangedFields); err != nil {
		return nil, fmt.Errorf("deserializing changed_fields: %w", err)
	}

	return &b, nil
}

func scanAuditLog(s scanner) (*audit.AuditLog, error) {
	var b audit.AuditLog
	var action string
	var detailsJSON []byte
	var changedFieldsJSON []byte

	err := s.Scan(
		&b.ID, &b.UserID, &b.Username, &b.CorrelationID, &action,
		&b.Resource, &b.ResourceID, &b.IP, &b.UserAgent,
		&detailsJSON, &changedFieldsJSON, &b.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	b.Action = audit.Action(action)
	if err := json.Unmarshal(detailsJSON, &b.Details); err != nil {
		return nil, fmt.Errorf("deserializing details: %w", err)
	}
	if err := json.Unmarshal(changedFieldsJSON, &b.ChangedFields); err != nil {
		return nil, fmt.Errorf("deserializing changed_fields: %w", err)
	}

	return &b, nil
}

// nullString returns nil for empty strings, used for optional SQL filters.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
