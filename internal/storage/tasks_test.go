package storage

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_ImplementsTaskStore(t *testing.T) {
	s := newTestStore(t)
	// Compile-time check
	var _ task.Store = s
}

func TestStore_CreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk := task.New("test task", "tester")
	require.NoError(t, s.Create(ctx, tk))

	got, err := s.Get(ctx, tk.ID)
	require.NoError(t, err)
	assert.Equal(t, tk.ID, got.ID)
	assert.Equal(t, tk.Description, got.Description)
	assert.Equal(t, tk.CreatedBy, got.CreatedBy)
	assert.Equal(t, tk.State, got.State)
}

func TestStore_CreateDuplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk := task.New("test task", "tester")
	require.NoError(t, s.Create(ctx, tk))
	err := s.Create(ctx, tk)
	assert.Error(t, err)
}

func TestStore_GetNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Get(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestStore_Update(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk := task.New("test task", "tester")
	require.NoError(t, s.Create(ctx, tk))

	tk.State = task.StatePlanning
	require.NoError(t, s.Update(ctx, tk))

	got, err := s.Get(ctx, tk.ID)
	require.NoError(t, err)
	assert.Equal(t, task.StatePlanning, got.State)
}

func TestStore_UpdateNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk := task.New("test task", "tester")
	err := s.Update(ctx, tk)
	assert.Error(t, err)
}

func TestStore_List(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk1 := task.New("task one", "tester")
	tk2 := task.New("task two", "tester")
	require.NoError(t, s.Create(ctx, tk1))
	require.NoError(t, s.Create(ctx, tk2))

	tasks, err := s.List(ctx)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}
