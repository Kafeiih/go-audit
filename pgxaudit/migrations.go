package pgxaudit

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
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

// CopyGooseMigrations writes embedded migrations using Goose SQL format
// (<version>_<name>.sql with -- +goose Up/Down sections).
func CopyGooseMigrations(dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	files, err := MigrationFiles()
	if err != nil {
		return err
	}

	type pair struct {
		upFile   string
		downFile string
	}
	pairs := map[string]*pair{}

	for _, name := range files {
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".up.sql"), ".down.sql")
		if base == name {
			continue
		}
		if _, ok := pairs[base]; !ok {
			pairs[base] = &pair{}
		}
		if strings.HasSuffix(name, ".up.sql") {
			pairs[base].upFile = name
		}
		if strings.HasSuffix(name, ".down.sql") {
			pairs[base].downFile = name
		}
	}

	if len(pairs) == 0 {
		return fmt.Errorf("no migration up/down pairs found")
	}

	bases := make([]string, 0, len(pairs))
	for base := range pairs {
		bases = append(bases, base)
	}
	sort.Strings(bases)

	for _, base := range bases {
		p := pairs[base]
		if p.upFile == "" || p.downFile == "" {
			return fmt.Errorf("incomplete migration pair for %s", base)
		}

		target := filepath.Join(dstDir, base+".sql")
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("migration already exists: %s", target)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking existing migration %s: %w", target, err)
		}

		upContent, err := fs.ReadFile(embeddedMigrations, path.Join("migrations", p.upFile))
		if err != nil {
			return fmt.Errorf("reading embedded migration %s: %w", p.upFile, err)
		}

		downContent, err := fs.ReadFile(embeddedMigrations, path.Join("migrations", p.downFile))
		if err != nil {
			return fmt.Errorf("reading embedded migration %s: %w", p.downFile, err)
		}

		content := fmt.Sprintf("-- +goose Up\n%s\n\n-- +goose Down\n%s\n", strings.TrimSpace(string(upContent)), strings.TrimSpace(string(downContent)))

		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing migration %s: %w", target, err)
		}
	}

	return nil
}
