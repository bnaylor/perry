package storage

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store provides durable persistence for tasks, audit records, and agent calls.
type Store struct {
	db          *sql.DB
	artifactDir string
}

// NewStore opens (or creates) the SQLite database and runs migrations.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Enable WAL mode and foreign keys
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	artifactDir := ""
	if dbPath != ":memory:" {
		artifactDir = filepath.Join(filepath.Dir(dbPath), "artifacts")
	}

	s := &Store{db: db, artifactDir: artifactDir}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	// Get current version — table may not exist yet
	var version int
	row := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version")
	if err := row.Scan(&version); err != nil {
		// Table doesn't exist yet — that's fine, version stays 0
		version = 0
	}

	for v := version + 1; v <= currentVersion; v++ {
		migration, ok := migrations[v]
		if !ok {
			return fmt.Errorf("missing migration for version %d", v)
		}
		if _, err := s.db.Exec(migration); err != nil {
			return fmt.Errorf("migration v%d: %w", v, err)
		}
		if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", v); err != nil {
			return fmt.Errorf("record migration v%d: %w", v, err)
		}
	}
	return nil
}
