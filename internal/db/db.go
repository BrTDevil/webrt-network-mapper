// Package db gestionează conexiunea SQLite și migrațiile.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB învelește conexiunea SQLite.
type DB struct {
	*sql.DB
}

// Open deschide (și creează la nevoie) baza de date și aplică migrațiile.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creare director date: %w", err)
	}
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite suportă un singur scriitor; o singură conexiune evită SQLITE_BUSY.
	sqlDB.SetMaxOpenConns(1)
	d := &DB{sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate() error {
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	// Unele migrații refac tabele referite prin chei străine; procedura SQLite
	// cere foreign_keys dezactivat în afara tranzacției (o singură conexiune în pool).
	if _, err := d.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer d.Exec(`PRAGMA foreign_keys = ON`)
	for _, name := range names {
		var n int
		if err := d.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrația %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(name, applied_at) VALUES (?, ?)`, name, Now()); err != nil {
			tx.Rollback()
			return err
		}
		if err := foreignKeyCheck(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrația %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// foreignKeyCheck verifică integritatea referințelor după o migrație.
func foreignKeyCheck(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("referințe invalide după migrație")
	}
	return rows.Err()
}

// Now întoarce momentul curent în format RFC3339 (UTC).
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
