package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const schemaVersion = 1

// InitSchema creates the schema_migrations tracking table (if absent) and
// applies all pending migrations in order. Migrations are idempotent: already-
// applied versions are skipped, so this is safe to call on both new and
// existing databases.
func InitSchema(db *sql.DB) error {
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("bootstrap schema_migrations: %w", err)
	}

	for v := 1; v <= schemaVersion; v++ {
		var applied int
		err := db.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE version = ?`, v).Scan(&applied)
		if err == nil {
			continue // already applied
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		data, err := migrationsFS.ReadFile(fmt.Sprintf("migrations/%03d_schema.sql", v))
		if err != nil {
			return fmt.Errorf("read schema v%d: %w", v, err)
		}

		if err := execMigration(ctx, db, v, string(data)); err != nil {
			return err
		}
	}
	return nil
}

// execMigration applies a single migration inside a transaction.
// Statements are split on ";\n" to handle ALTER TABLE, CREATE TABLE, etc.
func execMigration(ctx context.Context, db *sql.DB, version int, sql string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Split the file into individual statements for SQLite compatibility.
	// Some drivers handle multi-statement exec; splitting is more robust.
	stmts := splitSQL(sql)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply schema v%d (%q): %w", version, truncate(stmt, 60), err)
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
		return fmt.Errorf("record migration v%d: %w", version, err)
	}
	return tx.Commit()
}

// splitSQL splits a SQL string on semicolons, stripping comment lines.
func splitSQL(input string) []string {
	var stmts []string
	var cur strings.Builder
	for _, line := range strings.Split(input, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
		if strings.HasSuffix(trimmed, ";") {
			stmt := strings.TrimSuffix(strings.TrimSpace(cur.String()), ";")
			stmts = append(stmts, stmt)
			cur.Reset()
		}
	}
	if rest := strings.TrimSpace(cur.String()); rest != "" {
		stmts = append(stmts, strings.TrimSuffix(rest, ";"))
	}
	return stmts
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
