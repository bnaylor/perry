package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0o755)
	require.NoError(t, err)
	return path
}

func TestSubprocessGate_PassingScript(t *testing.T) {
	dir := t.TempDir()
	writeTempScript(t, dir, "pass.py", `
import json, sys
data = json.loads(sys.stdin.read())
print(json.dumps({"pass": True, "gate": "test", "findings": []}))
`)

	gate := NewSubprocessGate("test-pass", filepath.Join(dir, "pass.py"),
		func(input AuditInput) (map[string]any, error) {
			return map[string]any{"code": input.Code}, nil
		},
	)

	result := gate.Run(context.Background(), AuditInput{Code: "print('hello')"})
	assert.True(t, result.Pass)
	assert.Equal(t, "test-pass", result.Gate)
	assert.Empty(t, result.Findings)
}

func TestSubprocessGate_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	writeTempScript(t, dir, "fail.py", `
import sys
sys.exit(1)
`)

	gate := NewSubprocessGate("test-exit", filepath.Join(dir, "fail.py"),
		func(input AuditInput) (map[string]any, error) {
			return map[string]any{"code": input.Code}, nil
		},
	)

	result := gate.Run(context.Background(), AuditInput{Code: "x"})
	assert.False(t, result.Pass, "non-zero exit should fail closed")
	assert.Equal(t, "test-exit", result.Gate)
	assert.NotEmpty(t, result.Findings)
}

func TestSubprocessGate_BadJSON(t *testing.T) {
	dir := t.TempDir()
	writeTempScript(t, dir, "badjson.py", `
print("this is not json")
`)

	gate := NewSubprocessGate("test-badjson", filepath.Join(dir, "badjson.py"),
		func(input AuditInput) (map[string]any, error) {
			return map[string]any{"code": input.Code}, nil
		},
	)

	result := gate.Run(context.Background(), AuditInput{Code: "x"})
	assert.False(t, result.Pass, "bad JSON should fail closed")
	assert.Equal(t, "test-badjson", result.Gate)
	assert.True(t, len(result.Findings) > 0)
}

func TestSubprocessGate_Timeout(t *testing.T) {
	dir := t.TempDir()
	writeTempScript(t, dir, "slow.py", `
import time, json, sys
data = json.loads(sys.stdin.read())
time.sleep(10)
print(json.dumps({"pass": True, "gate": "test", "findings": []}))
`)

	gate := NewSubprocessGate("test-timeout", filepath.Join(dir, "slow.py"),
		func(input AuditInput) (map[string]any, error) {
			return map[string]any{"code": input.Code}, nil
		},
		WithTimeout(100*time.Millisecond),
	)

	result := gate.Run(context.Background(), AuditInput{Code: "x"})
	assert.False(t, result.Pass, "timeout should fail closed")
	assert.Equal(t, "test-timeout", result.Gate)
}

func TestSubprocessGate_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	writeTempScript(t, dir, "slow.py", `
import time, json, sys
data = json.loads(sys.stdin.read())
time.sleep(10)
print(json.dumps({"pass": True, "gate": "test", "findings": []}))
`)

	gate := NewSubprocessGate("test-cancel", filepath.Join(dir, "slow.py"),
		func(input AuditInput) (map[string]any, error) {
			return map[string]any{"code": input.Code}, nil
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result := gate.Run(ctx, AuditInput{Code: "x"})
	assert.False(t, result.Pass, "cancelled context should fail closed")
	assert.Equal(t, "test-cancel", result.Gate)
}
