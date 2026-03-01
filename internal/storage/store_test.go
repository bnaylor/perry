package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore_CreatesTablesInMemory(t *testing.T) {
	s, err := NewStore(":memory:")
	require.NoError(t, err)
	defer s.Close()

	// Verify all tables exist by querying sqlite_master
	rows, err := s.db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}

	assert.Contains(t, tables, "tasks")
	assert.Contains(t, tables, "transitions")
	assert.Contains(t, tables, "audit_records")
	assert.Contains(t, tables, "agent_calls")
	assert.Contains(t, tables, "schema_version")
}

func TestNewStore_MigrationsAreIdempotent(t *testing.T) {
	s, err := NewStore(":memory:")
	require.NoError(t, err)

	// Run migrations again — should not error
	err = s.migrate()
	assert.NoError(t, err)
	s.Close()
}
