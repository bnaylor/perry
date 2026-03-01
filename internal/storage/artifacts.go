package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// MoveArtifacts relocates executor output from srcDir into the store's managed
// artifact directory, keyed by taskID. Returns the destination path, or an
// empty string if srcDir contained no files (the empty dir is cleaned up).
func (s *Store) MoveArtifacts(taskID, srcDir string) (string, error) {
	// Check if source dir has any files
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return "", fmt.Errorf("read source dir: %w", err)
	}
	if len(entries) == 0 {
		os.RemoveAll(srcDir)
		return "", nil
	}

	destDir := filepath.Join(s.artifactDir, taskID)
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		return "", fmt.Errorf("create artifact parent dir: %w", err)
	}

	if err := os.Rename(srcDir, destDir); err != nil {
		return "", fmt.Errorf("move artifacts: %w", err)
	}
	return destDir, nil
}
