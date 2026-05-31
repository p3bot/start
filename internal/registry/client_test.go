package registry

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelang.org/go/mod/module"
)

type mockRegistry struct {
	fetchFunc    func(ctx context.Context, mv module.Version) (module.SourceLoc, error)
	versionsFunc func(ctx context.Context, path string) ([]string, error)
	fetchCalls   int
}

func (m *mockRegistry) Fetch(ctx context.Context, mv module.Version) (module.SourceLoc, error) {
	m.fetchCalls++
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, mv)
	}
	return module.SourceLoc{}, errors.New("not implemented")
}

func (m *mockRegistry) ModuleVersions(ctx context.Context, path string) ([]string, error) {
	if m.versionsFunc != nil {
		return m.versionsFunc(ctx, path)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRegistry) Requirements(ctx context.Context, mv module.Version) ([]module.Version, error) {
	return nil, nil
}

type mockOSRootFS struct {
	fs.FS
	root string
}

func (m *mockOSRootFS) OSRoot() string {
	return m.root
}

func (m *mockOSRootFS) Open(name string) (fs.File, error) {
	return nil, errors.New("not implemented")
}

func TestSourceLocToPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		loc     module.SourceLoc
		want    string
		wantErr bool
	}{
		{
			name: "fs implements OSRootFS",
			loc: module.SourceLoc{
				FS: &mockOSRootFS{root: "/path/to/module"},
			},
			want:    "/path/to/module",
			wantErr: false,
		},
		{
			name: "fs does not implement OSRootFS",
			loc: module.SourceLoc{
				FS: nil,
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sourceLocToPath(tt.loc)
			if (err != nil) != tt.wantErr {
				t.Errorf("sourceLocToPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("sourceLocToPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetch_RetryLogic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		failCount      int
		retries        int
		wantErr        bool
		wantFetchCalls int
	}{
		{
			name:           "succeeds on first attempt",
			failCount:      0,
			retries:        3,
			wantErr:        false,
			wantFetchCalls: 1,
		},
		{
			name:           "succeeds after one retry",
			failCount:      1,
			retries:        3,
			wantErr:        false,
			wantFetchCalls: 2,
		},
		{
			name:           "succeeds after two retries",
			failCount:      2,
			retries:        3,
			wantErr:        false,
			wantFetchCalls: 3,
		},
		{
			name:           "fails after all retries exhausted",
			failCount:      5,
			retries:        3,
			wantErr:        true,
			wantFetchCalls: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &mockRegistry{
				fetchFunc: func(ctx context.Context, mv module.Version) (module.SourceLoc, error) {
					callCount++
					if callCount <= tt.failCount {
						return module.SourceLoc{}, ociregistry.ErrTooManyRequests
					}
					return module.SourceLoc{
						FS: &mockOSRootFS{root: "/cached/module"},
					}, nil
				},
			}

			client := &client{
				registry: mock,
				retries:  tt.retries,
				baseWait: time.Millisecond,
			}

			ctx := context.Background()
			result, err := client.Fetch(ctx, "github.com/test/module@v0.0.1")

			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Exhausted retries must yield a transient FetchError recording the attempt count.
			if tt.wantErr {
				var fe *FetchError
				if !errors.As(err, &fe) {
					t.Fatalf("Fetch() error = %v, want *FetchError", err)
				}
				if fe.Kind != FetchTransient {
					t.Errorf("Fetch() error Kind = %v, want FetchTransient", fe.Kind)
				}
				if fe.Attempts != tt.retries {
					t.Errorf("Fetch() error Attempts = %d, want %d", fe.Attempts, tt.retries)
				}
			}

			if mock.fetchCalls != tt.wantFetchCalls {
				t.Errorf("Fetch() called registry %d times, want %d", mock.fetchCalls, tt.wantFetchCalls)
			}

			if !tt.wantErr && result.SourceDir != "/cached/module" {
				t.Errorf("Fetch() SourceDir = %v, want /cached/module", result.SourceDir)
			}
		})
	}
}

func TestFetch_ContextCancellation(t *testing.T) {
	t.Parallel()
	mock := &mockRegistry{
		fetchFunc: func(ctx context.Context, mv module.Version) (module.SourceLoc, error) {
			// Transient so the retry loop engages and the cancel interrupts mid-backoff.
			return module.SourceLoc{}, ociregistry.ErrTooManyRequests
		},
	}

	client := &client{
		registry: mock,
		retries:  5,
		baseWait: time.Second, // long wait so the 50 ms cancel always arrives first
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := client.Fetch(ctx, "github.com/test/module@v0.0.1")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Fetch() error = %v, want context.Canceled", err)
	}
}

func TestFetch_InvalidModulePath(t *testing.T) {
	t.Parallel()
	client := &client{
		registry: &mockRegistry{},
		retries:  3,
		baseWait: time.Millisecond,
	}

	tests := []struct {
		name       string
		modulePath string
		wantErr    string
	}{
		{
			name:       "missing version",
			modulePath: "github.com/test/module",
			wantErr:    "parsing module path",
		},
		{
			name:       "empty path",
			modulePath: "",
			wantErr:    "parsing module path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := client.Fetch(ctx, tt.modulePath)
			if err == nil {
				t.Error("Fetch() expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Fetch() error = %v, want to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolveLatestVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		modulePath  string
		versions    []string
		versionsErr error
		want        string
		wantErr     bool
	}{
		{
			name:       "canonical version returned as-is",
			modulePath: "github.com/test/module@v0.0.1",
			versions:   nil,
			want:       "github.com/test/module@v0.0.1",
			wantErr:    false,
		},
		{
			name:       "canonical version with patch",
			modulePath: "github.com/test/module@v1.2.3",
			versions:   nil,
			want:       "github.com/test/module@v1.2.3",
			wantErr:    false,
		},
		{
			name:       "major version resolved to latest",
			modulePath: "github.com/test/module@v0",
			versions:   []string{"v0.0.1", "v0.0.2", "v0.1.0"},
			want:       "github.com/test/module@v0.1.0",
			wantErr:    false,
		},
		{
			name:       "no versions found",
			modulePath: "github.com/test/module@v0",
			versions:   []string{},
			wantErr:    true,
		},
		{
			name:        "versions fetch error",
			modulePath:  "github.com/test/module@v0",
			versionsErr: errors.New("network error"),
			wantErr:     true,
		},
		{
			name:       "module path without @ fails",
			modulePath: "github.com/test/module",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRegistry{
				versionsFunc: func(ctx context.Context, path string) ([]string, error) {
					if tt.versionsErr != nil {
						return nil, tt.versionsErr
					}
					return tt.versions, nil
				},
			}

			client := &client{
				registry: mock,
				retries:  3,
				baseWait: time.Millisecond,
			}

			ctx := context.Background()
			got, err := client.ResolveLatestVersion(ctx, tt.modulePath)

			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveLatestVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("ResolveLatestVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsCanonicalVersion exercises ResolveLatestVersion's inline canonical check
// by observing whether it makes a network call (non-canonical) or not (canonical).
func TestIsCanonicalVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		version     string
		isCanonical bool
	}{
		{"v0.0.1", true},
		{"v1.2.3", true},
		{"v10.20.30", true},
		{"v0.0.1-beta", true},
		{"v0", false},
		{"v1", false},
		{"v0.1", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			versionsCalled := false
			mock := &mockRegistry{
				versionsFunc: func(ctx context.Context, path string) ([]string, error) {
					versionsCalled = true
					return []string{"v0.0.1"}, nil
				},
			}

			client := &client{
				registry: mock,
				retries:  1,
				baseWait: time.Millisecond,
			}

			modulePath := "github.com/test/module@" + tt.version
			ctx := context.Background()
			_, _ = client.ResolveLatestVersion(ctx, modulePath)

			if tt.isCanonical && versionsCalled {
				t.Errorf("version %q should be canonical, but ModuleVersions was called", tt.version)
			}
			if !tt.isCanonical && !versionsCalled {
				t.Errorf("version %q should not be canonical, but ModuleVersions was not called", tt.version)
			}
		})
	}
}
