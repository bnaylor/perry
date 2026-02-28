// internal/task/memstore_test.go
package task

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemStoreCreateAndGet(t *testing.T) {
	store := NewMemStore()
	ctx := context.Background()

	tk := New("test task", "user-1")
	err := store.Create(ctx, tk)
	require.NoError(t, err)

	got, err := store.Get(ctx, tk.ID)
	require.NoError(t, err)
	assert.Equal(t, tk.ID, got.ID)
	assert.Equal(t, tk.Description, got.Description)
}

func TestMemStoreGetNotFound(t *testing.T) {
	store := NewMemStore()
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestMemStoreUpdate(t *testing.T) {
	store := NewMemStore()
	ctx := context.Background()

	tk := New("test task", "user-1")
	require.NoError(t, store.Create(ctx, tk))

	tk.State = StatePlanning
	err := store.Update(ctx, tk)
	require.NoError(t, err)

	got, err := store.Get(ctx, tk.ID)
	require.NoError(t, err)
	assert.Equal(t, StatePlanning, got.State)
}

func TestMemStoreList(t *testing.T) {
	store := NewMemStore()
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, New("task 1", "user-1")))
	require.NoError(t, store.Create(ctx, New("task 2", "user-1")))

	tasks, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}
