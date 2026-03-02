package audit

import (
	"errors"
	"path/filepath"
)

// ErrSkipGate is returned by an InputMapper to indicate that the gate
// should be skipped (auto-pass) for this input — e.g., a Python-only
// gate receiving Go code.
var ErrSkipGate = errors.New("skip gate")

// NewASTGate creates a gate that runs the Python AST analyzer.
// allowedImports is the list of permitted top-level imports (from policy config).
func NewASTGate(scriptDir string, allowedImports []string) *SubprocessGate {
	return NewSubprocessGate("ast", filepath.Join(scriptDir, "audit", "ast_analyzer.py"),
		func(input AuditInput) (map[string]any, error) {
			if input.Language != "" && input.Language != "python" {
				return nil, ErrSkipGate
			}
			return map[string]any{
				"code":            input.Code,
				"allowed_imports": allowedImports,
			}, nil
		},
	)
}

// NewSecretsGate creates a gate that scans code for hardcoded secrets.
func NewSecretsGate(scriptDir string) *SubprocessGate {
	return NewSubprocessGate("secrets", filepath.Join(scriptDir, "audit", "secrets_scanner.py"),
		func(input AuditInput) (map[string]any, error) {
			return map[string]any{"code": input.Code}, nil
		},
	)
}

// NewSchemaValidationGate creates a gate that validates a context packet
// against the JSON schema.
func NewSchemaValidationGate(scriptDir string) *SubprocessGate {
	return NewSubprocessGate("schema_validation", filepath.Join(scriptDir, "context_packet", "validate.py"),
		func(input AuditInput) (map[string]any, error) {
			return map[string]any{"packet": input.ContextPacket}, nil
		},
	)
}

// NewContentScanGate creates a gate that scans a context packet for secrets
// and injection patterns.
func NewContentScanGate(scriptDir string) *SubprocessGate {
	return NewSubprocessGate("content_scan", filepath.Join(scriptDir, "context_packet", "scan.py"),
		func(input AuditInput) (map[string]any, error) {
			return map[string]any{"packet": input.ContextPacket}, nil
		},
	)
}
