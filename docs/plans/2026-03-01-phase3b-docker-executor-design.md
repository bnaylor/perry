# Phase 3b Design: Docker Executor (Container Runtime)

## Scope

Implement a `DockerExecutor` that satisfies the existing `executor.Executor` interface using the Docker Go SDK. Ephemeral, network-isolated, resource-limited containers run agent-generated code and return logs + output artifacts.

**In scope:** Python execution, fail-closed error handling, security hardening, unit + integration tests.
**Out of scope:** Notary integration, workspace/codebase mounts (Mirror Cage), debug-mode container retention, multi-language support beyond Python.

## File Layout

```
internal/executor/
├── executor.go                  # (existing) interface + types
├── mock.go                      # (existing) mock for testing
├── docker.go                    # NEW — DockerExecutor implementation
├── docker_client.go             # NEW — extracted Docker client interface
├── docker_test.go               # NEW — unit tests with mock client
└── docker_integration_test.go   # NEW — real Docker tests (//go:build integration)
```

## DockerExecutor Structure

`DockerExecutor` holds:
- A `DockerClient` interface (thin wrapper around the SDK's `client.Client`)
- A default base image (`python:3.12-slim`)
- Resource limits config via `SandboxLimits` struct with `DefaultSandboxLimits()`

`DockerClient` interface — extracted subset of the SDK:
- `ImagePull`
- `ContainerCreate`
- `ContainerStart`
- `ContainerWait`
- `ContainerLogs`
- `CopyToContainer`
- `CopyFromContainer`
- `ContainerRemove`

Real implementation wraps the SDK; tests use a mock. Same pattern as the rest of the project.

## Container Lifecycle

The `Run()` method:

```
1. Validate language is supported (fail-closed if not)
2. Build in-memory tar: /workspace/main.py + optional requirements.txt
3. Pull base image if not cached locally
4. Create container with security constraints (see below)
5. Copy tar into container at /workspace
6. Start container
7. Wait for exit (respects ctx deadline + RunRequest.TimeoutSec)
8. Capture logs (stdout + stderr)
9. Copy /out directory contents to host temp dir (os.MkdirTemp)
10. Remove container (always, via defer)
11. Return Result{Success, ExitCode, Logs, Output}
```

The caller (orchestrator) owns the temp dir lifecycle.

## Code Injection & Execution

Tar archive injected via `CopyToContainer`:
- `/workspace/main.py` — the generated code
- `/workspace/requirements.txt` — if `RunRequest.Dependencies` is non-empty

Entrypoint per language:

```go
var entrypoints = map[string][]string{
    "python": {"/bin/sh", "-c",
        "cd /workspace && if [ -f requirements.txt ]; then pip install -q --user -r requirements.txt; fi && python main.py"},
}
```

Unsupported language returns an error immediately.

Output convention: agent code writes to `/out/`. That tmpfs mount is copied back after execution.

## Security Constraints

| Constraint | Value | Rationale |
|---|---|---|
| Network | `none` | Coder output must not phone home |
| Rootfs | Read-only | Prevent persistent tampering |
| Tmpfs mounts | `/tmp` (64MB), `/out` (64MB), `/home/sandbox` (32MB) | Bounded writable scratch; home needed for `pip --user` |
| Memory | 256MB | Prevent OOM-killing the host |
| PID limit | 256 | Prevent fork bombs |
| CPU | 1 core | Fair scheduling |
| Capabilities | All dropped | No `CAP_NET_RAW`, `CAP_SYS_ADMIN`, etc. |
| Privileged | false | No escalation |
| User | UID 1000 | Non-root least privilege |
| Timeout | `RunRequest.TimeoutSec` (default 60s) | Enforced via context deadline |

Defaults live in `DefaultSandboxLimits()` so future phases can adjust without touching executor logic.

## Fail-Closed Semantics

Matching the audit gate pattern — every error path returns an error, never a silent success:

- Docker daemon unreachable: error
- Image pull fails: error
- Container creation fails: error
- Timeout: kill container, ExitCode -1, error
- Context cancelled: kill container, error
- Copy from /out fails: return logs + exit code, but Output is empty (not an error — code may not produce output files)

## Testing

### Unit tests (`docker_test.go`)
- Mock `DockerClient` interface with canned responses
- Full `Run()` lifecycle happy path
- Fail-closed: mock errors at each SDK call, verify errors returned
- Timeout: mock blocking `ContainerWait`, verify context cancellation
- Unsupported language: verify error
- Empty /out: verify Result has Logs but empty Output

### Integration tests (`docker_integration_test.go`, `//go:build integration`)
- Require Docker daemon running
- Trivial script: `print("hello")` — verify logs, exit code 0
- Output script: write to `/out/result.txt` — verify file copied back
- Timeout: `time.sleep(999)` — verify timeout kills container
- Non-zero exit: `sys.exit(1)` — verify ExitCode, Success=false
- Network isolation: attempt HTTP request — verify failure

### Existing tests unchanged
Orchestrator continues using `MockExecutor`. `DockerExecutor` wired at CLI level.

## Implementation Deviations

The following changes were discovered during integration testing and represent improvements over the original design:

### ReadonlyRootfs removed
`CopyToContainer` writes to the container's rootfs layer *before* the container starts (before tmpfs mounts are applied). With `ReadonlyRootfs: true`, this fails. Security is maintained via: no network, all capabilities dropped, non-root user, resource limits, and tmpfs mounts for writable directories.

### /out uses bind mount, not tmpfs
Tmpfs data is lost when the container process exits (mount namespace torn down). `/out` is now a host bind mount (`os.MkdirTemp` → bind into container) so output files persist and can be read directly. The `checkOutput` helper returns the path if files exist, or cleans up and returns empty string.

### /workspace is not a tmpfs
Same reason as `/out` — `CopyToContainer` injects code into `/workspace` before the container starts. A tmpfs mount applied at start would hide the injected files.

### Docker log demuxing
Docker's `ContainerLogs` API returns a multiplexed stream with 8-byte frame headers (stream type + payload length). Raw `io.ReadAll` returns garbled output. Added `stdcopy.StdCopy` from `github.com/docker/docker/pkg/stdcopy` to properly demux stdout/stderr. The mock client was updated to produce framed output matching real Docker behavior.

### Default timeout
Changed from 60s (design) to 30s (implementation) as a more conservative default for sandbox execution. Overridable via `RunRequest.TimeoutSec`.

## Dependencies

- `github.com/docker/docker` v28.5.2 (Go SDK)

## Wiring

`DockerExecutor` is constructed in `cmd/perry/main.go` and passed to the orchestrator, replacing `MockExecutor`. Falls back to `MockExecutor` with a warning if Docker client creation fails. The orchestrator doesn't know or care which implementation it gets — same interface.

The orchestrator's `StateExecuting` handler was also updated to extract `code`, `language`, and `dependencies` from the coder's stored output (previously passed `"placeholder"`).
