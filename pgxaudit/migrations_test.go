package pgxaudit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationFiles(t *testing.T) {
	files, err := MigrationFiles()
	if err != nil {
		t.Fatalf("MigrationFiles returned error: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected embedded migration files")
	}
}

func TestCopyMigrations(t *testing.T) {
	dir := t.TempDir()

	if err := CopyMigrations(dir); err != nil {
		t.Fatalf("CopyMigrations returned error: %v", err)
	}

	files, err := MigrationFiles()
	if err != nil {
		t.Fatalf("MigrationFiles returned error: %v", err)
	}

	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("expected copied file %s: %v", f, err)
		}
	}

	if err := CopyMigrations(dir); err == nil {
		t.Fatal("expected error when copying on top of existing files")
	}
}

func TestCopyGooseMigrations(t *testing.T) {
	dir := t.TempDir()

	if err := CopyGooseMigrations(dir); err != nil {
		t.Fatalf("CopyGooseMigrations returned error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "000001_create_audit_schema.sql"))
	if err != nil {
		t.Fatalf("expected goose migration file: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "-- +goose Up") {
		t.Fatal("expected goose Up marker")
	}
	if !strings.Contains(text, "-- +goose Down") {
		t.Fatal("expected goose Down marker")
	}

	if err := CopyGooseMigrations(dir); err == nil {
		t.Fatal("expected error when copying on top of existing goose files")
	}
}
