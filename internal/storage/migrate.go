package storage

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

// RunMigrations applies all embedded migrations in order to the database.
// It uses a simple version table (schema_migrations) to track applied migrations.
func RunMigrations(db *sql.DB) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}
	applied, err := appliedMigrations(db)
	if err != nil {
		return err
	}
	for _, name := range migrationNames() {
		if applied[name] {
			continue
		}
		up, ok := migrationsUp[name]
		if !ok {
			return fmt.Errorf("migration %q not found", name)
		}
		if _, err := db.Exec(up); err != nil {
			return fmt.Errorf("migration %q: %w", name, err)
		}
		if err := recordMigration(db, name); err != nil {
			return err
		}
	}
	return nil
}

func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY
		)
	`)
	return err
}

func appliedMigrations(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT name FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

func recordMigration(db *sql.DB, name string) error {
	_, err := db.Exec("INSERT INTO schema_migrations (name) VALUES (?)", name)
	return err
}

func migrationNames() []string {
	names := make([]string, 0, len(migrationsUp))
	for n := range migrationsUp {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// migrationsUp is populated by init() from embedded SQL.
var migrationsUp = map[string]string{
	"001_schema": strings.TrimSpace(migration001Up),
}
