package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/kafeiih/go-audit/pgxaudit"
)

func main() {
	outDir := flag.String("out", "./migrations", "destination directory for migration files")
	flag.Parse()

	if err := pgxaudit.CopyMigrations(*outDir); err != nil {
		log.Fatalf("copying migrations: %v", err)
	}

	files, err := pgxaudit.MigrationFiles()
	if err != nil {
		log.Fatalf("listing migrations: %v", err)
	}

	fmt.Printf("copied %d migration files to %s\n", len(files), *outDir)
}
