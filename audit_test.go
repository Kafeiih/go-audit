package audit_test

import (
	"context"
	"testing"
	"time"

	audit "github.com/kafeiih/go-audit"
)

func TestNewAuditLog_Valid(t *testing.T) {
	entry, err := audit.NewAuditLog(
		"user-1", "john",
		audit.ActionCreate,
		"payments", "pay-123",
		"127.0.0.1", "TestAgent/1.0",
		map[string]any{"status_code": 200},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if entry.UserID != "user-1" {
		t.Errorf("expected user_id=user-1, got %s", entry.UserID)
	}
	if entry.Action != audit.ActionCreate {
		t.Errorf("expected action=CREATE, got %s", entry.Action)
	}
	if entry.ID.String() == "" {
		t.Error("expected UUID to be generated")
	}
}

func TestNewAuditLog_RequiresUserID(t *testing.T) {
	_, err := audit.NewAuditLog("", "john", audit.ActionRead, "payments", "", "", "", nil)
	if err == nil {
		t.Fatal("expected error for empty user_id")
	}
}

func TestNewAuditLog_RequiresValidAction(t *testing.T) {
	_, err := audit.NewAuditLog("user-1", "john", audit.Action("INVALID"), "payments", "", "", "", nil)
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestNewAuditLog_RequiresResource(t *testing.T) {
	_, err := audit.NewAuditLog("user-1", "john", audit.ActionRead, "", "", "", "", nil)
	if err == nil {
		t.Fatal("expected error for empty resource")
	}
}

func TestNewAuditLog_NilDetailsBecomesEmptyMap(t *testing.T) {
	entry, err := audit.NewAuditLog("user-1", "john", audit.ActionRead, "payments", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Details == nil {
		t.Error("expected details to be initialized, got nil")
	}
}

func TestNewAuditLog_CustomClock(t *testing.T) {
	fixed := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entry, err := audit.NewAuditLog(
		"user-1", "john", audit.ActionRead, "payments", "", "", "", nil,
		func() time.Time { return fixed },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !entry.CreatedAt.Equal(fixed) {
		t.Errorf("expected created_at=%v, got %v", fixed, entry.CreatedAt)
	}
}

func TestAction_IsValid(t *testing.T) {
	valid := []audit.Action{audit.ActionCreate, audit.ActionRead, audit.ActionUpdate, audit.ActionDelete}
	for _, a := range valid {
		if !a.IsValid() {
			t.Errorf("expected %s to be valid", a)
		}
	}
	if audit.Action("NOPE").IsValid() {
		t.Error("expected NOPE to be invalid")
	}
}

func TestContextPropagation(t *testing.T) {
	ctx := context.Background()

	// No info initially
	if audit.InfoFrom(ctx) != nil {
		t.Error("expected nil info from empty context")
	}

	// Attach info
	info := audit.Info{UserID: "u1", Username: "alice", Resource: "test"}
	ctx = audit.WithInfo(ctx, info)
	got := audit.InfoFrom(ctx)
	if got == nil {
		t.Fatal("expected info from context")
	}
	if got.UserID != "u1" {
		t.Errorf("expected UserID=u1, got %s", got.UserID)
	}

	// Skip audit
	if audit.ShouldSkip(ctx) {
		t.Error("expected ShouldSkip=false before WithSkipAudit")
	}
	ctx = audit.WithSkipAudit(ctx)
	if !audit.ShouldSkip(ctx) {
		t.Error("expected ShouldSkip=true after WithSkipAudit")
	}
}
