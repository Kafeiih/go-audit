package pgxaudit

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// MigrationFiles returns migration file names embedded in the package.
func MigrationFiles() ([]string, error) {
	entries, err := fs.ReadDir(embeddedMigrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("reading embedded migrations: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	return files, nil
}

// CopyMigrations writes embedded migration files into dstDir.
// It fails if any target file already exists.
func CopyMigrations(dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	files, err := MigrationFiles()
	if err != nil {
		return err
	}

	for _, name := range files {
		target := filepath.Join(dstDir, name)
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("migration already exists: %s", target)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking existing migration %s: %w", target, err)
		}

		content, err := fs.ReadFile(embeddedMigrations, path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("reading embedded migration %s: %w", name, err)
		}

		if err := os.WriteFile(target, content, 0o644); err != nil {
			return fmt.Errorf("writing migration %s: %w", target, err)
		}
	}

	return nil
}
