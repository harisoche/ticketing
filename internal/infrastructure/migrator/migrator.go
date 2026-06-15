// Package migrator applies SQL migration files in lexical order and tracks
// applied versions in a `schema_migrations` table. It is intentionally tiny
// so students can read the source end-to-end.
package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SchemaMigrationsDDL is created idempotently on first run.
const SchemaMigrationsDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    VARCHAR(64) PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

// File represents one migration on disk.
type File struct {
	Version  string // e.g. "000001"
	Name     string // e.g. "000001_create_auth_tables"
	UpPath   string
	DownPath string
}

var fileNamePattern = regexp.MustCompile(`^(\d+)_([A-Za-z0-9_]+)\.(up|down)\.sql$`)

// Load scans dir for `<version>_<name>.<up|down>.sql` files and returns
// every migration pair, ordered by version.
func Load(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	byName := map[string]*File{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := fileNamePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		version, name, kind := m[1], m[2], m[3]
		key := version + "_" + name
		f, ok := byName[key]
		if !ok {
			f = &File{Version: version, Name: key}
			byName[key] = f
		}
		full := filepath.Join(dir, e.Name())
		switch kind {
		case "up":
			f.UpPath = full
		case "down":
			f.DownPath = full
		}
	}
	out := make([]File, 0, len(byName))
	for _, f := range byName {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// Status returns the set of versions already applied.
func Status(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	if _, err := db.ExecContext(ctx, SchemaMigrationsDDL); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// Up applies every pending up migration. Returns the list of versions just
// applied.
func Up(ctx context.Context, db *sql.DB, files []File) ([]string, error) {
	applied, err := Status(ctx, db)
	if err != nil {
		return nil, err
	}
	var done []string
	for _, f := range files {
		if applied[f.Version] {
			continue
		}
		if f.UpPath == "" {
			return done, fmt.Errorf("migration %s has no up file", f.Version)
		}
		if err := applyFile(ctx, db, f, f.UpPath, "up"); err != nil {
			return done, err
		}
		done = append(done, f.Version)
	}
	return done, nil
}

// Down rolls back the most recent N applied migrations.
func Down(ctx context.Context, db *sql.DB, files []File, steps int) ([]string, error) {
	if steps < 1 {
		steps = 1
	}
	applied, err := Status(ctx, db)
	if err != nil {
		return nil, err
	}
	// Reverse order, take only applied versions, then trim to steps.
	var pending []File
	for i := len(files) - 1; i >= 0; i-- {
		if applied[files[i].Version] {
			pending = append(pending, files[i])
		}
		if len(pending) == steps {
			break
		}
	}
	var rolled []string
	for _, f := range pending {
		if f.DownPath == "" {
			return rolled, fmt.Errorf("migration %s has no down file", f.Version)
		}
		if err := applyFile(ctx, db, f, f.DownPath, "down"); err != nil {
			return rolled, err
		}
		rolled = append(rolled, f.Version)
	}
	return rolled, nil
}

func applyFile(ctx context.Context, db *sql.DB, f File, path, kind string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// The SQL files may contain their own BEGIN/COMMIT (Phase 1 does).
	// Strip those so we don't nest transactions.
	sqlText := stripOuterTx(string(body))
	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("apply %s: %w", filepath.Base(path), err)
	}
	if kind == "up" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, f.Version); err != nil {
			return fmt.Errorf("record %s: %w", f.Version, err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, f.Version); err != nil {
			return fmt.Errorf("unrecord %s: %w", f.Version, err)
		}
	}
	return tx.Commit()
}

// stripOuterTx removes a leading BEGIN; / COMMIT; pair if present. Some of
// the project's migration files (Phase 1) include their own transaction
// boundaries; we wrap each migration ourselves so they must come off.
func stripOuterTx(sqlText string) string {
	lines := strings.Split(sqlText, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(strings.ToUpper(line))
		// Strip standalone BEGIN; and COMMIT;
		if trim == "BEGIN;" || trim == "COMMIT;" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
