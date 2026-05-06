package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScope_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		scope Scope
		want  string
	}{
		{"merged scope", ScopeMerged, "merged"},
		{"global scope", ScopeGlobal, "global"},
		{"local scope", ScopeLocal, "local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.scope.String()
			if got != tt.want {
				t.Errorf("Scope(%d).String() = %q, want %q", tt.scope, got, tt.want)
			}
		})
	}
}

func TestResolvePaths(t *testing.T) {
	// Create a temporary working directory
	workDir := t.TempDir()

	// Test with no config directories
	t.Run("no config dirs", func(t *testing.T) {
		p, err := ResolvePaths(workDir)
		if err != nil {
			t.Fatalf("ResolvePaths() error = %v", err)
		}

		if p.LocalExists {
			t.Error("LocalExists = true, want false")
		}
		if p.Local != filepath.Join(workDir, ".start") {
			t.Errorf("Local = %q, want %q", p.Local, filepath.Join(workDir, ".start"))
		}
	})

	// Test with local config directory
	t.Run("with local config", func(t *testing.T) {
		localDir := filepath.Join(workDir, ".start")
		if err := os.Mkdir(localDir, 0o755); err != nil {
			t.Fatalf("Failed to create local config dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(localDir) }()

		p, err := ResolvePaths(workDir)
		if err != nil {
			t.Fatalf("ResolvePaths() error = %v", err)
		}

		if !p.LocalExists {
			t.Error("LocalExists = false, want true")
		}
		if p.Local != localDir {
			t.Errorf("Local = %q, want %q", p.Local, localDir)
		}
	})
}

func TestResolvePaths_XDGConfigHome(t *testing.T) {
	// Create a temporary XDG config directory
	xdgDir := t.TempDir()
	startDir := filepath.Join(xdgDir, "start")
	if err := os.Mkdir(startDir, 0o755); err != nil {
		t.Fatalf("Failed to create start config dir: %v", err)
	}

	// Set XDG_CONFIG_HOME (t.Setenv auto-restores on cleanup)
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	workDir := t.TempDir()
	p, err := ResolvePaths(workDir)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}

	if p.Global != startDir {
		t.Errorf("Global = %q, want %q", p.Global, startDir)
	}
	if !p.GlobalExists {
		t.Error("GlobalExists = false, want true")
	}
}

func TestPaths_ForScope(t *testing.T) {
	globalDir := "/home/user/.config/start"
	localDir := "/project/.start"

	tests := []struct {
		name         string
		paths        Paths
		scope        Scope
		wantPaths    []string
		wantNilSlice bool
	}{
		{
			name: "merged both exist",
			paths: Paths{
				Global: globalDir, GlobalExists: true,
				Local: localDir, LocalExists: true,
			},
			scope:     ScopeMerged,
			wantPaths: []string{globalDir, localDir},
		},
		{
			name: "merged only global",
			paths: Paths{
				Global: globalDir, GlobalExists: true,
				Local: localDir, LocalExists: false,
			},
			scope:     ScopeMerged,
			wantPaths: []string{globalDir},
		},
		{
			name: "merged only local",
			paths: Paths{
				Global: globalDir, GlobalExists: false,
				Local: localDir, LocalExists: true,
			},
			scope:     ScopeMerged,
			wantPaths: []string{localDir},
		},
		{
			name: "merged none exist",
			paths: Paths{
				Global: globalDir, GlobalExists: false,
				Local: localDir, LocalExists: false,
			},
			scope:        ScopeMerged,
			wantNilSlice: true,
		},
		{
			name: "global scope exists",
			paths: Paths{
				Global: globalDir, GlobalExists: true,
				Local: localDir, LocalExists: true,
			},
			scope:     ScopeGlobal,
			wantPaths: []string{globalDir},
		},
		{
			name: "global scope not exists",
			paths: Paths{
				Global: globalDir, GlobalExists: false,
				Local: localDir, LocalExists: true,
			},
			scope:        ScopeGlobal,
			wantNilSlice: true,
		},
		{
			name: "local scope exists",
			paths: Paths{
				Global: globalDir, GlobalExists: true,
				Local: localDir, LocalExists: true,
			},
			scope:     ScopeLocal,
			wantPaths: []string{localDir},
		},
		{
			name: "local scope not exists",
			paths: Paths{
				Global: globalDir, GlobalExists: true,
				Local: localDir, LocalExists: false,
			},
			scope:        ScopeLocal,
			wantNilSlice: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.paths.ForScope(tt.scope)

			if tt.wantNilSlice {
				if got != nil {
					t.Errorf("ForScope(%v) = %v, want nil", tt.scope, got)
				}
				return
			}

			if len(got) != len(tt.wantPaths) {
				t.Errorf("ForScope(%v) len = %d, want %d", tt.scope, len(got), len(tt.wantPaths))
				return
			}
			for i, p := range got {
				if p != tt.wantPaths[i] {
					t.Errorf("ForScope(%v)[%d] = %q, want %q", tt.scope, i, p, tt.wantPaths[i])
				}
			}
		})
	}
}

func TestPaths_Dir(t *testing.T) {
	t.Parallel()
	p := Paths{
		Global: "/home/user/.config/start",
		Local:  "/project/.start",
	}

	tests := []struct {
		name  string
		local bool
		want  string
	}{
		{"global scope", false, "/home/user/.config/start"},
		{"local scope", true, "/project/.start"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Dir(tt.local)
			if got != tt.want {
				t.Errorf("Dir(%v) = %q, want %q", tt.local, got, tt.want)
			}
		})
	}
}

func TestPaths_AnyExists(t *testing.T) {
	tests := []struct {
		name   string
		paths  Paths
		expect bool
	}{
		{"both exist", Paths{GlobalExists: true, LocalExists: true}, true},
		{"only global", Paths{GlobalExists: true, LocalExists: false}, true},
		{"only local", Paths{GlobalExists: false, LocalExists: true}, true},
		{"none exist", Paths{GlobalExists: false, LocalExists: false}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.paths.AnyExists(); got != tt.expect {
				t.Errorf("AnyExists() = %v, want %v", got, tt.expect)
			}
		})
	}
}
