package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_MoveArtifacts(t *testing.T) {
	s := newTestStore(t)
	baseDir := t.TempDir()
	s.artifactDir = baseDir

	// Create a fake executor output dir with a file
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "output.txt"), []byte("hello"), 0644))

	managedPath, err := s.MoveArtifacts("task-abc123", srcDir)
	require.NoError(t, err)

	// Verify file was moved
	assert.DirExists(t, managedPath)
	content, err := os.ReadFile(filepath.Join(managedPath, "output.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))

	// Verify source dir no longer exists
	_, err = os.Stat(srcDir)
	assert.True(t, os.IsNotExist(err))
}

func TestStore_MoveArtifacts_EmptyDir(t *testing.T) {
	s := newTestStore(t)
	baseDir := t.TempDir()
	s.artifactDir = baseDir

	srcDir := t.TempDir()
	managedPath, err := s.MoveArtifacts("task-abc123", srcDir)
	require.NoError(t, err)
	assert.Empty(t, managedPath)
}
