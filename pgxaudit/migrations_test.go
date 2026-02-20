package pgxaudit

import (
	"os"
	"path/filepath"
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
