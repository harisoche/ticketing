// cmd/migrate is the project's tiny SQL migration runner.
//
// Usage:
//
//	go run ./cmd/migrate up                  # apply every pending migration
//	go run ./cmd/migrate down                # roll back the latest one
//	go run ./cmd/migrate down 3              # roll back the 3 most-recent
//	go run ./cmd/migrate status              # list applied / pending
//
// Reads DATABASE_URL from environment (loaded via internal/config). Reads
// migration files from `db/migrations`.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"ticketing-api/internal/config"
	"ticketing-api/internal/infrastructure/migrator"
)

const defaultDir = "db/migrations"

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: migrate (up|down [steps]|status) [-dir db/migrations]")
	}
	dir := flag.String("dir", defaultDir, "directory containing <version>_<name>.{up,down}.sql files")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	cmd := args[0]

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	db, err := openDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	files, err := migrator.Load(*dir)
	if err != nil {
		log.Fatalf("%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	switch cmd {
	case "up":
		applied, err := migrator.Up(ctx, db, files)
		if err != nil {
			log.Fatalf("up: %v", err)
		}
		if len(applied) == 0 {
			fmt.Println("nothing to apply")
			return
		}
		fmt.Printf("applied %d migration(s):\n", len(applied))
		for _, v := range applied {
			fmt.Printf("  + %s\n", v)
		}

	case "down":
		steps := 1
		if len(args) >= 2 {
			v, err := strconv.Atoi(args[1])
			if err != nil || v < 1 {
				log.Fatalf("invalid steps %q", args[1])
			}
			steps = v
		}
		rolled, err := migrator.Down(ctx, db, files, steps)
		if err != nil {
			log.Fatalf("down: %v", err)
		}
		if len(rolled) == 0 {
			fmt.Println("nothing to roll back")
			return
		}
		fmt.Printf("rolled back %d migration(s):\n", len(rolled))
		for _, v := range rolled {
			fmt.Printf("  - %s\n", v)
		}

	case "status":
		applied, err := migrator.Status(ctx, db)
		if err != nil {
			log.Fatalf("status: %v", err)
		}
		for _, f := range files {
			marker := "[ ]"
			if applied[f.Version] {
				marker = "[x]"
			}
			fmt.Printf("  %s %s\n", marker, f.Name)
		}

	default:
		flag.Usage()
		os.Exit(2)
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
