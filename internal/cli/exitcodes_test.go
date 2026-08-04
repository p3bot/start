package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	internalcue "github.com/p3bot/start/internal/cue"
	"github.com/p3bot/start/internal/fault"
	"github.com/p3bot/start/internal/registry"
)

func TestExitCodeFromError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil is success", nil, ExitSuccess},
		{"plain error is general", errors.New("boom"), ExitFailure},
		{"ExitError pins its code", &ExitError{Code: ExitConflict, Err: errors.New("x")}, ExitConflict},
		{"fetch transient", &registry.FetchError{Kind: registry.FetchTransient, Err: errors.New("net")}, ExitTransient},
		{"fetch not found", &registry.FetchError{Kind: registry.FetchNotFound, Err: errors.New("gone")}, ExitNotFound},
		{"fetch usage", &registry.FetchError{Kind: registry.FetchUsage, Err: errors.New("bad path")}, ExitUsage},
		{"wrapped fetch transient", fmt.Errorf("fetching index: %w", &registry.FetchError{Kind: registry.FetchTransient}), ExitTransient},
		{"not-found sentinel", fault.NotFound(errors.New("role not found")), ExitNotFound},
		{"usage sentinel", fault.Usage(errors.New("bad flag")), ExitUsage},
		{"user-config sentinel", fault.UserConfig(errors.New("bad cue")), ExitConfig},
		{"validation error", &internalcue.ValidationError{Message: "bad"}, ExitConfig},
		{"permission", fmt.Errorf("writing: %w", fs.ErrPermission), ExitPermission},
		{"deadline exceeded", fmt.Errorf("fetch: %w", context.DeadlineExceeded), ExitTransient},
		// Ordering: a permission error the loader has also tagged as a user-config
		// fault must surface as 4 (fix perms), not 78.
		{"permission wins over user-config", fault.UserConfig(fmt.Errorf("reading config: %w", fs.ErrPermission)), ExitPermission},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ExitCodeFromError(tt.err); got != tt.want {
				t.Errorf("ExitCodeFromError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// transientFetchErr is a stand-in for the typed transient failure FetchIndex
// returns when the registry is unreachable.
func transientFetchErr() error {
	return &registry.FetchError{Kind: registry.FetchTransient, Op: "fetch", Path: "x", Err: errors.New("network down")}
}

func TestExitCodes_CommandPaths(t *testing.T) {
	t.Run("missing required arg is usage (2)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		_, err := captureText(t, stub, "list", "extra1", "extra2") // list takes at most one arg
		if got := ExitCodeFromError(err); got != ExitUsage {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitUsage)
		}
	})

	t.Run("unknown category is usage (2)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		_, err := captureText(t, stub, "list", "bogus-category")
		if got := ExitCodeFromError(err); got != ExitUsage {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitUsage)
		}
	})

	t.Run("invalid --color is usage (2)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		_, err := captureText(t, stub, "--color=technicolor", "list")
		if got := ExitCodeFromError(err); got != ExitUsage {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitUsage)
		}
	})

	t.Run("removed --no-color is rejected as usage (2)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		_, err := captureText(t, stub, "--no-color", "list")
		if got := ExitCodeFromError(err); got != ExitUsage {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitUsage)
		}
	})

	t.Run("removed config remove --yes is rejected as usage (2)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		_, err := captureText(t, stub, "config", "remove", "echo", "--yes")
		if got := ExitCodeFromError(err); got != ExitUsage {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitUsage)
		}
	})

	t.Run("removed config remove -y is rejected as usage (2)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		_, err := captureText(t, stub, "config", "remove", "echo", "-y")
		if got := ExitCodeFromError(err); got != ExitUsage {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitUsage)
		}
	})

	t.Run("missing installed module is not-found (3)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		_, err := captureText(t, stub, "config", "get", "zzz-no-such-item")
		if got := ExitCodeFromError(err); got != ExitNotFound {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitNotFound)
		}
	})

	t.Run("typo'd module name is not-found (3)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		_, err := captureText(t, stub, "install", "zzz-no-such-module", "--local")
		if got := ExitCodeFromError(err); got != ExitNotFound {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitNotFound)
		}
	})

	t.Run("registry-unreachable install is transient (75)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		stub.SetFetchIndexError(transientFetchErr())
		_, err := captureText(t, stub, "install", "claude", "--local")
		if got := ExitCodeFromError(err); got != ExitTransient {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitTransient)
		}
	})

	t.Run("registry-unreachable update is transient (75)", func(t *testing.T) {
		tmpDir, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		writeInstalledRegistryAgent(t, tmpDir)
		stub.SetFetchIndexError(transientFetchErr())
		_, err := captureText(t, stub, "update")
		if got := ExitCodeFromError(err); got != ExitTransient {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitTransient)
		}
	})

	t.Run("registry-unreachable search with no local match is transient (75)", func(t *testing.T) {
		_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		stub.SetFetchIndexError(transientFetchErr())
		_, err := captureText(t, stub, "search", "zzznomatchanywhere")
		if got := ExitCodeFromError(err); got != ExitTransient {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitTransient)
		}
	})

	t.Run("malformed user CUE is config (78)", func(t *testing.T) {
		tmpDir, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
		broken := filepath.Join(tmpDir, ".start", "agents.cue")
		if err := os.WriteFile(broken, []byte("agents: { this is not valid cue ::::"), 0o644); err != nil {
			t.Fatalf("writing broken cue: %v", err)
		}
		_, err := captureText(t, stub, "list")
		if got := ExitCodeFromError(err); got != ExitConfig {
			t.Fatalf("exit code = %d (err=%v), want %d", got, err, ExitConfig)
		}
	})
}
