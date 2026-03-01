//go:build !integration

package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultSandboxLimits(t *testing.T) {
	limits := DefaultSandboxLimits()
	assert.Equal(t, int64(256*1024*1024), limits.MemoryBytes)
	assert.Equal(t, int64(256), limits.PidsLimit)
	assert.Equal(t, 1.0, limits.CPUs)
	assert.Equal(t, int64(64*1024*1024), limits.TmpfsSizeBytes)
	assert.Equal(t, int64(64*1024*1024), limits.OutSizeBytes)
	assert.Equal(t, int64(32*1024*1024), limits.HomeSizeBytes)
}
