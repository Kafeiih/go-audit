package pgxaudit

import (
	"context"
	"testing"

	audit "github.com/kafeiih/go-audit"
)

// Note: AuditPool wraps *pgxpool.Pool directly and cannot be fully unit-tested
// without a real PostgreSQL connection. These tests document the expected
// behavior and cover the context-based branching logic.
//
// For full integration tests, use a test database with:
//   docker run --rm -p 5432:5432 -e POSTGRES_PASSWORD=test postgres:16

func TestAuditPool_ExecBranching_NoInfo(t *testing.T) {
	// When no audit.Info is in context, Exec should pass through directly.
	// We can't test this without a real pool, but we verify the logic path
	// by checking that InfoFrom returns nil for a bare context.
	ctx := context.Background()
	info := audit.InfoFrom(ctx)
	if info != nil {
		t.Fatal("expected nil info from bare context")
	}
}

func TestAuditPool_ExecBranching_WithSkip(t *testing.T) {
	// When skip is set, Exec should pass through even with info present.
	ctx := context.Background()
	ctx = audit.WithInfo(ctx, audit.Info{UserID: "u1", Resource: "test"})
	ctx = audit.WithSkipAudit(ctx)

	if !audit.ShouldSkip(ctx) {
		t.Fatal("expected ShouldSkip=true")
	}
	// With both info present and skip=true, AuditPool.Exec takes the
	// pass-through path (no transaction wrapping).
}

func TestAuditPool_ExecBranching_WithInfo(t *testing.T) {
	// When info is present and skip is not set, Exec should wrap in a
	// transaction with SET LOCAL calls.
	ctx := context.Background()
	ctx = audit.WithInfo(ctx, audit.Info{
		UserID:     "u1",
		Username:   "alice",
		Resource:   "orders",
		ResourceID: "ord-1",
		IP:         "10.0.0.1",
		UserAgent:  "TestAgent/1.0",
	})

	info := audit.InfoFrom(ctx)
	if info == nil {
		t.Fatal("expected info from context")
	}
	if audit.ShouldSkip(ctx) {
		t.Fatal("expected ShouldSkip=false")
	}

	// Verify all fields are propagated.
	if info.UserID != "u1" {
		t.Errorf("UserID = %q, want %q", info.UserID, "u1")
	}
	if info.Username != "alice" {
		t.Errorf("Username = %q, want %q", info.Username, "alice")
	}
	if info.Resource != "orders" {
		t.Errorf("Resource = %q, want %q", info.Resource, "orders")
	}
	if info.ResourceID != "ord-1" {
		t.Errorf("ResourceID = %q, want %q", info.ResourceID, "ord-1")
	}
	if info.IP != "10.0.0.1" {
		t.Errorf("IP = %q, want %q", info.IP, "10.0.0.1")
	}
	if info.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %q, want %q", info.UserAgent, "TestAgent/1.0")
	}
}
