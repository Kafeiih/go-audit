package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/kafeiih/go-audit/pgxaudit"
)

func main() {
	outDir := flag.String("out", "./migrations", "destination directory for migration files")
	format := flag.String("format", "split", "migration output format: split|goose")
	goose := flag.Bool("goose", false, "write Goose-compatible migration files (equivalent to -format goose)")
	flag.Parse()

	selectedFormat := *format
	if *goose {
		if *format != "split" && *format != "goose" {
			log.Fatalf("invalid format %q, expected split or goose", *format)
		}
		selectedFormat = "goose"
	}

	var err error
	switch selectedFormat {
	case "split":
		err = pgxaudit.CopyMigrations(*outDir)
	case "goose":
		err = pgxaudit.CopyGooseMigrations(*outDir)
	default:
		log.Fatalf("invalid format %q, expected split or goose", selectedFormat)
	}

	if err != nil {
		log.Fatalf("copying migrations: %v", err)
	}

	files, err := pgxaudit.MigrationFiles()
	if err != nil {
		log.Fatalf("listing migrations: %v", err)
	}

	fmt.Printf("copied %d embedded migration files to %s using %s format\n", len(files), *outDir, selectedFormat)
}
