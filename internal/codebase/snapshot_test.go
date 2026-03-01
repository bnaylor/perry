package codebase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTakeSnapshot(t *testing.T) {
	// Parse internal/task as a real-world example in our project
	snapshot, err := TakeSnapshot([]string{"../task"})
	require.NoError(t, err)

	pkg, ok := snapshot["../task"]
	require.True(t, ok, "package ../task should be in snapshot")

	// Verify Task struct
	taskType, ok := pkg.Types["Task"]
	require.True(t, ok, "Task struct should be found")
	assert.Equal(t, "Task", taskType.Name)
	assert.Contains(t, taskType.Fields, "ID")
	assert.Contains(t, taskType.Fields, "State")

	// Verify NewTask function (it's called New in our codebase)
	foundNew := false
	for _, f := range pkg.Functions {
		if f.Name == "New" {
			foundNew = true
			assert.NotEmpty(t, f.Params)
			assert.Equal(t, "*Task", f.Returns[0])
			break
		}
	}
	assert.True(t, foundNew, "New function should be found")

	// Verify imports
	assert.Contains(t, pkg.Imports, "crypto/rand")
}
