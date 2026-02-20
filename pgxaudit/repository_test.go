package pgxaudit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	audit "github.com/kafeiih/go-audit"
)

// ---------- Mock DB ----------

type mockDB struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return nil, nil
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return nil
}

// ---------- Create ----------

func TestPostgresRepo_Create_Success(t *testing.T) {
	var capturedSQL string
	var capturedArgs []any

	db := &mockDB{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			capturedSQL = sql
			capturedArgs = args
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	repo := NewPostgresRepo(db)

	entry, err := audit.NewAuditLog(
		"user-1", "alice", "corr-123",
		audit.ActionCreate,
		"orders", "ord-1",
		"10.0.0.1", "TestAgent/1.0",
		map[string]any{"amount": 100.5},
	)
	if err != nil {
		t.Fatalf("unexpected error creating audit log: %v", err)
	}

	err = repo.Create(context.Background(), entry)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if capturedSQL == "" {
		t.Fatal("expected SQL to be captured")
	}

	// Verify all 12 args were passed.
	if len(capturedArgs) != 12 {
		t.Fatalf("expected 12 args, got %d", len(capturedArgs))
	}

	// Verify the ID is passed correctly.
	if capturedArgs[0] != entry.ID {
		t.Errorf("arg[0] (id) = %v, want %v", capturedArgs[0], entry.ID)
	}

	if capturedArgs[3] != "corr-123" {
		t.Errorf("arg[3] (correlation_id) = %v, want corr-123", capturedArgs[3])
	}
	// Verify details is serialized as JSON bytes.
	detailsBytes, ok := capturedArgs[9].([]byte)
	if !ok {
		t.Fatalf("arg[9] (details) expected []byte, got %T", capturedArgs[9])
	}
	var details map[string]any
	if err := json.Unmarshal(detailsBytes, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["amount"] != 100.5 {
		t.Errorf("details[amount] = %v, want 100.5", details["amount"])
	}
	changedFieldsBytes, ok := capturedArgs[10].([]byte)
	if !ok {
		t.Fatalf("arg[10] (changed_fields) expected []byte, got %T", capturedArgs[10])
	}
	var changed map[string]any
	if err := json.Unmarshal(changedFieldsBytes, &changed); err != nil {
		t.Fatalf("failed to unmarshal changed_fields: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("expected empty changed_fields by default, got %v", changed)
	}
}

func TestPostgresRepo_Create_ExecError(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("connection refused")
		},
	}

	repo := NewPostgresRepo(db)

	entry, _ := audit.NewAuditLog(
		"user-1", "alice", "", audit.ActionCreate, "orders", "ord-1", "", "", nil,
	)

	err := repo.Create(context.Background(), entry)
	if err == nil {
		t.Fatal("expected error from Create")
	}
	if !errors.Is(err, errors.Unwrap(err)) && err.Error() == "" {
		t.Fatal("expected wrapped error")
	}
}

func TestPostgresRepo_Create_InvalidDetails(t *testing.T) {
	db := &mockDB{}
	repo := NewPostgresRepo(db)

	// Create entry with a channel in details — json.Marshal will fail.
	entry := &audit.AuditLog{
		ID:        uuid.New(),
		UserID:    "user-1",
		Username:  "alice",
		Action:    audit.ActionCreate,
		Resource:  "orders",
		Details:   map[string]any{"bad": make(chan int)},
		CreatedAt: time.Now(),
	}

	err := repo.Create(context.Background(), entry)
	if err == nil {
		t.Fatal("expected error for non-serializable details")
	}
}

// ---------- GetByID ----------

func TestPostgresRepo_GetByID_QueryRowError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &errorRow{err: errors.New("not found")}
		},
	}

	repo := NewPostgresRepo(db)
	_, err := repo.GetByID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from GetByID")
	}
}

// ---------- List ----------

func TestPostgresRepo_List_QueryError(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, errors.New("query failed")
		},
	}

	repo := NewPostgresRepo(db)
	_, _, err := repo.List(context.Background(), audit.AuditFilters{Limit: 10})
	if err == nil {
		t.Fatal("expected error from List")
	}
}

func TestPostgresRepo_List_FiltersPassedCorrectly(t *testing.T) {
	var capturedArgs []any

	db := &mockDB{
		queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
			capturedArgs = args
			return nil, errors.New("stop") // return error to short-circuit
		},
	}

	repo := NewPostgresRepo(db)
	now := time.Now()
	from := now.Add(-24 * time.Hour)

	repo.List(context.Background(), audit.AuditFilters{
		UserID:        "user-1",
		CorrelationID: "corr-123",
		Resource:      "orders",
		Action:        audit.ActionCreate,
		From:          &from,
		To:            &now,
		Limit:         20,
		Offset:        5,
	})

	if len(capturedArgs) != 8 {
		t.Fatalf("expected 8 args, got %d", len(capturedArgs))
	}

	// $1 = UserID (as *string)
	if s := capturedArgs[0].(*string); s == nil || *s != "user-1" {
		t.Errorf("arg[0] (UserID) = %v, want 'user-1'", capturedArgs[0])
	}
	// $2 = CorrelationID
	if s := capturedArgs[1].(*string); s == nil || *s != "corr-123" {
		t.Errorf("arg[1] (CorrelationID) = %v, want corr-123", capturedArgs[1])
	}
	// $3 = Resource
	if s := capturedArgs[2].(*string); s == nil || *s != "orders" {
		t.Errorf("arg[2] (Resource) = %v, want 'orders'", capturedArgs[2])
	}
	// $4 = Action
	if s := capturedArgs[3].(*string); s == nil || *s != "CREATE" {
		t.Errorf("arg[3] (Action) = %v, want 'CREATE'", capturedArgs[3])
	}
	// $7 = Limit
	if capturedArgs[6] != 20 {
		t.Errorf("arg[6] (Limit) = %v, want 20", capturedArgs[6])
	}
	// $8 = Offset
	if capturedArgs[7] != 5 {
		t.Errorf("arg[7] (Offset) = %v, want 5", capturedArgs[7])
	}
}

func TestPostgresRepo_List_EmptyFilters(t *testing.T) {
	var capturedArgs []any

	db := &mockDB{
		queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
			capturedArgs = args
			return nil, errors.New("stop")
		},
	}

	repo := NewPostgresRepo(db)
	repo.List(context.Background(), audit.AuditFilters{})

	// Empty strings should be converted to nil pointers.
	if capturedArgs[0] != (*string)(nil) {
		t.Errorf("arg[0] (UserID) should be nil for empty filter, got %v", capturedArgs[0])
	}
	if capturedArgs[1] != (*string)(nil) {
		t.Errorf("arg[1] (CorrelationID) should be nil for empty filter, got %v", capturedArgs[1])
	}
	if capturedArgs[2] != (*string)(nil) {
		t.Errorf("arg[2] (Resource) should be nil for empty filter, got %v", capturedArgs[2])
	}
	if capturedArgs[3] != (*string)(nil) {
		t.Errorf("arg[3] (Action) should be nil for empty filter, got %v", capturedArgs[3])
	}
}

// ---------- Helpers ----------

// errorRow implements pgx.Row returning a fixed error.
type errorRow struct {
	err error
}

func (r *errorRow) Scan(_ ...any) error {
	return r.err
}
