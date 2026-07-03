# Testing Strategy

Tiered testing using Go's standard testing framework with build tags. Prioritise real behaviour over mocking. Use `scripts/invoke-tests` as the everyday entry point.

## Test Types

Unit tests:

- Location: `*_test.go` alongside source files
- Purpose: Pure functions, logic, isolated components
- Build tag: None (default)
- Run: `go test ./...`

Integration tests:

- Location: `test/integration/*_test.go`
- Purpose: Component interactions (CLI + CUE loader + validator)
- Build tag: `//go:build integration`
- Run: `go test -tags=integration ./...`

E2E tests:

- Location: `test/e2e/*_test.go`
- Purpose: Compiled binary end-to-end (currently the auto-setup flows, `TestE2E_AutoSetup_*`)
- Build tag: `//go:build e2e`
- Prerequisite: Binary must be built first (the tests locate it at `bin/start`)
- Run: `go test -tags=e2e ./...`

Registry integration tests:

- Location: `internal/cli/doctor_validate_integration_test.go`
- Purpose: Live-registry checks (`doctor validate --json` shape)
- Build tag: `//go:build registry`
- Run: `go test -tags=registry ./internal/cli/...`
- Not run by `scripts/invoke-tests` — the script never adds this tag, so these tests only run when invoked explicitly

Naming caveat: `internal/cli/config_integration_test.go` carries no build tag despite its name — it runs as a default unit test. The tag, not the filename, decides the tier.

## Directory Structure

```
start/
├── internal/
│   ├── cli/
│   │   ├── root.go
│   │   ├── root_test.go
│   │   └── ...
│   ├── cue/
│   │   ├── loader.go
│   │   ├── loader_test.go
│   │   └── ...
│   └── config/
├── test/
│   ├── integration/
│   ├── e2e/
│   └── testdata/
│       ├── valid/
│       └── invalid/
└── scripts/
    └── invoke-tests
```

## Test Script

`scripts/invoke-tests` is the everyday entry point. Before running any tests it runs a mandatory lint gate — `scripts/invoke-linter` (golangci-lint; supports `-- --fix` for auto-fixes) — and aborts if lint fails. With `-e`/`-a` it builds `bin/start` for the e2e tests first.

```bash
./scripts/invoke-tests                 # Lint gate + unit tests
./scripts/invoke-tests -i              # + integration
./scripts/invoke-tests -e              # + integration + e2e (builds bin/start)
./scripts/invoke-tests -a              # Same as -e (does NOT include registry-tagged tests)
./scripts/invoke-tests -r              # Race detector
./scripts/invoke-tests -c              # Coverage report (coverage.html, no threshold enforced)
./scripts/invoke-tests -v              # Verbose output
./scripts/invoke-tests -s              # Short mode (skip long tests)
./scripts/invoke-tests -- -run TestFoo # Forward args after -- to go test verbatim
```

The passthrough cannot narrow the package set (`./...` always wins); drop to raw `go test ./subpkg/...` for that.

## Patterns

Cobra command testing — build the command via `NewRootCmd()`, set args and capture streams:

```go
cmd := NewRootCmd()
var out, errBuf bytes.Buffer
cmd.SetOut(&out)
cmd.SetErr(&errBuf)
cmd.SetArgs([]string{"list", "--json"})
err := cmd.Execute()
```

Registry-backed commands go through the shared capture helpers instead, which inject a stub client on the command context: `captureStreams(t, stub, args...)` returns separate stdout/stderr (`internal/cli/seam_coverage_test.go`), and `captureJSON(t, stub, args...)` decodes the stdout JSON (`internal/cli/json_capture_test.go`). Threading the stub as a parameter makes a network bypass a compile error.

Table-driven tests:

```go
tests := []struct {
    name    string
    input   string
    wantErr bool
}{
    {"valid input", "good", false},
    {"invalid input", "bad", true},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // test logic
    })
}
```

Real file system — use `t.TempDir()` and write real CUE files, then exercise the real loader (see `internal/cue/loader_test.go` for the canonical examples).

Interactive input — cobra's `SetIn`/`SetOut`/`SetErr` inject the streams; no custom command struct exists.

E2E binary testing — the e2e tests (`test/e2e/autosetup_test.go`) locate the pre-built binary via a `binaryPath(t)` helper that checks `../../bin/start` then `./bin/start`, and run it with `exec.Command` in a `t.TempDir()` working directory with an isolated environment passed via `cmd.Env`.

## Isolation and Seams

The cli test binary establishes two suite-wide guarantees in `TestMain` (`internal/cli/main_test.go`):

- An execution guard: a process-replacer seam that panics if any test reaches the real `syscall.Exec` launch path, so a test can never replace the test process and mask failures.
- Offline by default: `offlineRegistryForTests = true` wires the resolver's `indexSource` seam to an offline source (nil index), keeping resolver-backed surfaces (`get`, `describe`, `start`, `task`) off the network.

Registry substitution happens at two seams, never by mocking HTTP:

- The resolver's `indexSource`: `newTestResolver` (offline), `newResolverWithIndex` (pre-loaded index), `newRecordingResolver` (records the live-vs-cache-gated decision).
- The `registry.Client` interface via the per-command provider: `setupStartTestConfigWithRegistry(t, idx)` installs a `registryStub`, consumed through `captureStreams`/`captureJSON`.

Environment isolation is `isolateConfigEnv` (`internal/cli/start_test.go`): `t.TempDir()` plus `t.Setenv` of `HOME`, `XDG_CONFIG_HOME`, and `XDG_CACHE_HOME`, with a cleanup that chmods the read-only CUE module cache so the temp dir can be removed. `setupStartTestConfig(t)` layers a seeded `.start/` config on top. In-process tests override the environment with `t.Setenv`; `cmd.Env` is only for the e2e subprocess.

`--json` output shapes are drift-guarded: structural assertions live in `internal/cli/schemas_drift_test.go` for local commands and `internal/cli/json_capture_test.go` for registry-backed ones; `doctor validate --json` is covered by the `registry`-tagged integration test.

Parallelism: tests that call `os.Chdir` or mutate `color.NoColor` touch process-global state and must not use `t.Parallel()`.

## Mocking Guidelines

Do not mock:

- CUE library — use real CUE validation (fast and deterministic)
- File system — use `t.TempDir()` for real files
- Time — unless testing time-based logic specifically

Acceptable to substitute (through the seams above, not ad-hoc mocks):

- Registry access (`indexSource`, `registry.Client` stub via the provider)
- System command detection (`exec.LookPath` via interface)
- Environment variables (`t.Setenv` in-process; `cmd.Env` for the e2e subprocess)

## Coverage Goals

Aspirational, not enforced — `invoke-tests -c` generates an HTML report but no threshold is checked:

- `internal/cue/`, `internal/config/`: 80%+ coverage
- `internal/cli/`: 70%+ coverage
- Overall: focus on meaningful coverage, not percentage targets
