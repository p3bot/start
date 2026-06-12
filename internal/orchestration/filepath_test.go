package orchestration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/start-cli/start/internal/temp"
)

func TestIsFilePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "relative dot slash", input: "./file.md", want: true},
		{name: "relative nested", input: "./path/to/file.md", want: true},
		{name: "absolute path", input: "/usr/local/file.md", want: true},
		{name: "absolute root", input: "/file.md", want: true},
		{name: "tilde home", input: "~/file.md", want: true},
		{name: "tilde nested", input: "~/path/to/file.md", want: true},
		{name: "just dot slash", input: "./", want: true},
		{name: "just slash", input: "/", want: true},
		{name: "just tilde", input: "~", want: true},
		{name: "tilde user syntax", input: "~user/bin", want: false},
		{name: "tilde without slash", input: "~foo", want: false},

		{name: "simple name", input: "go-expert", want: false},
		{name: "namespaced", input: "golang/code-review", want: false},
		{name: "with hyphen", input: "pre-commit-review", want: false},
		{name: "with underscore", input: "my_role", want: false},
		{name: "relative without dot", input: "path/to/file.md", want: false},
		{name: "empty string", input: "", want: false},
		{name: "single dot", input: ".", want: false},
		{name: "double dot", input: "..", want: false},
		{name: "hidden file style", input: ".hidden", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsFilePath(tt.input)
			if got != tt.want {
				t.Errorf("IsFilePath(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExpandFilePath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "absolute path unchanged",
			input: "/usr/local/file.md",
			want:  "/usr/local/file.md",
		},
		{
			name:  "tilde expands to home",
			input: "~/file.md",
			want:  filepath.Join(homeDir, "file.md"),
		},
		{
			name:  "tilde nested path",
			input: "~/path/to/file.md",
			want:  filepath.Join(homeDir, "path/to/file.md"),
		},
		{
			name:  "relative path becomes absolute",
			input: "./file.md",
			want:  filepath.Join(cwd, "file.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandFilePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandFilePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandFilePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestReadFilePath(t *testing.T) {
	tmpDir := t.TempDir()

	testContent := "This is test content.\nLine 2."
	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "read existing file",
			path: testFile,
			want: testContent,
		},
		{
			name:    "read non-existent file",
			path:    filepath.Join(tmpDir, "missing.md"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFilePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadFilePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ReadFilePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsRemoteLocator(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "http", input: "http://example.com/role.md", want: true},
		{name: "https", input: "https://example.com/role.md", want: true},
		{name: "uppercase scheme", input: "HTTPS://example.com", want: true},
		{name: "mixed case scheme", input: "HtTp://example.com", want: true},
		{name: "ftp scheme", input: "ftp://example.com/x", want: false},
		{name: "file scheme", input: "file:///etc/passwd", want: false},
		{name: "local relative", input: "./role.md", want: false},
		{name: "local absolute", input: "/etc/role.md", want: false},
		{name: "bare name", input: "go-expert", want: false},
		{name: "empty", input: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRemoteLocator(tt.input); got != tt.want {
				t.Errorf("IsRemoteLocator(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsLocator(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "relative path", input: "./role.md", want: true},
		{name: "absolute path", input: "/abs/role.md", want: true},
		{name: "tilde path", input: "~/role.md", want: true},
		{name: "https url", input: "https://example.com/role.md", want: true},
		{name: "http url", input: "http://example.com/role.md", want: true},
		{name: "bare name", input: "go-expert", want: false},
		{name: "category qualified", input: "tasks:review", want: false},
		{name: "empty", input: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLocator(tt.input); got != tt.want {
				t.Errorf("IsLocator(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestReadLocator_RemoteSuccess(t *testing.T) {
	const body = "# Remote role\n\nYou are helpful.\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	got, err := ReadLocator(srv.URL + "/role.md")
	if err != nil {
		t.Fatalf("ReadLocator() error = %v", err)
	}
	if got != body {
		t.Errorf("ReadLocator() = %q, want %q", got, body)
	}
}

func TestReadLocator_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	url := srv.URL + "/missing.md"
	_, err := ReadLocator(url)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), url) || !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want it to name the URL and status 404", err.Error())
	}
}

func TestRemoteFetcher_SizeCap(t *testing.T) {
	body := strings.Repeat("x", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	url := srv.URL + "/big.md"

	// A body over the cap is an error naming the locator and the limit.
	if _, err := (remoteFetcher{MaxBytes: 10}).fetch(url); err == nil {
		t.Error("expected oversize error")
	} else if !strings.Contains(err.Error(), url) || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %q, want it to name the URL and mention the limit", err.Error())
	}

	// A body exactly at the cap passes (LimitReader reads one past the cap).
	if got, err := (remoteFetcher{MaxBytes: 100}).fetch(url); err != nil {
		t.Errorf("at-cap fetch error = %v, want success", err)
	} else if got != body {
		t.Errorf("at-cap fetch = %q, want full body", got)
	}
}

func TestRemoteFetcher_Timeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // block until released so the client-side timeout fires first
	}))
	// LIFO: close(release) runs before srv.Close(), so Close does not deadlock
	// waiting on the still-blocked handler.
	defer srv.Close()
	defer close(release)

	url := srv.URL + "/slow.md"
	_, err := (remoteFetcher{Timeout: 20 * time.Millisecond}).fetch(url)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), url) {
		t.Errorf("error = %q, want it to name the URL", err.Error())
	}
}

func TestReadLocator_RejectsHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "<html><body>soft 404</body></html>")
	}))
	defer srv.Close()

	url := srv.URL + "/page"
	_, err := ReadLocator(url)
	if err == nil {
		t.Fatal("expected text/html rejection")
	}
	if !strings.Contains(err.Error(), url) || !strings.Contains(err.Error(), "text/html") {
		t.Errorf("error = %q, want it to name the URL and mention text/html", err.Error())
	}
}

func TestRejectHTML(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantErr     bool
	}{
		{name: "plain text", contentType: "text/plain; charset=utf-8", wantErr: false},
		{name: "markdown", contentType: "text/markdown", wantErr: false},
		{name: "octet stream", contentType: "application/octet-stream", wantErr: false},
		{name: "json", contentType: "application/json", wantErr: false},
		{name: "missing header", contentType: "", wantErr: false},
		{name: "unparseable", contentType: "text/", wantErr: false},
		{name: "html lowercase", contentType: "text/html", wantErr: true},
		{name: "html with charset", contentType: "text/html; charset=utf-8", wantErr: true},
		{name: "html uppercase", contentType: "TEXT/HTML", wantErr: true},
		{name: "html malformed params", contentType: "text/html;;", wantErr: true},
		{name: "html dangling param", contentType: "text/html; charset", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectHTML("https://example.com/x", tt.contentType)
			if (err != nil) != tt.wantErr {
				t.Errorf("rejectHTML(%q) err = %v, wantErr %v", tt.contentType, err, tt.wantErr)
			}
		})
	}
}

func TestRawGitHubHint(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "blob nested path",
			url:  "https://github.com/owner/repo/blob/main/path/role.md",
			want: "https://raw.githubusercontent.com/owner/repo/main/path/role.md",
		},
		{
			name: "blob root file",
			url:  "https://github.com/owner/repo/blob/main/role.md",
			want: "https://raw.githubusercontent.com/owner/repo/main/role.md",
		},
		{name: "non-blob path", url: "https://github.com/owner/repo/tree/main", want: ""},
		{name: "already raw", url: "https://raw.githubusercontent.com/owner/repo/main/role.md", want: ""},
		{name: "other host", url: "https://example.com/owner/repo/blob/main/role.md", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rawGitHubHint(tt.url); got != tt.want {
				t.Errorf("rawGitHubHint(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestRejectHTML_GitHubHint(t *testing.T) {
	err := rejectHTML("https://github.com/owner/repo/blob/main/role.md", "text/html")
	if err == nil {
		t.Fatal("expected html rejection")
	}
	want := "https://raw.githubusercontent.com/owner/repo/main/role.md"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to suggest %q", err.Error(), want)
	}
}

func TestReadRoleLocator_Remote(t *testing.T) {
	const body = "remote role body\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	workingDir := t.TempDir()
	mgr := temp.NewUTDManager(workingDir)
	url := srv.URL + "/role.md"

	content, path, err := ReadRoleLocator(mgr, url)
	if err != nil {
		t.Fatalf("ReadRoleLocator() error = %v", err)
	}
	if content != body {
		t.Errorf("content = %q, want %q", content, body)
	}

	wantDir := filepath.Join(workingDir, ".start", "temp")
	if filepath.Dir(path) != wantDir {
		t.Errorf("materialised dir = %q, want %q", filepath.Dir(path), wantDir)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading materialised file: %v", err)
	}
	if string(onDisk) != body {
		t.Errorf("materialised content = %q, want %q", string(onDisk), body)
	}

	// A re-run overwrites the same deterministic path in place.
	_, path2, err := ReadRoleLocator(mgr, url)
	if err != nil {
		t.Fatalf("second ReadRoleLocator() error = %v", err)
	}
	if path2 != path {
		t.Errorf("second path = %q, want same as first %q", path2, path)
	}
}

func TestReadRoleLocator_Local(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "role.md")
	const body = "local role\n"
	if err := os.WriteFile(file, []byte(body), 0644); err != nil {
		t.Fatalf("writing role file: %v", err)
	}

	mgr := temp.NewUTDManager(t.TempDir())
	content, path, err := ReadRoleLocator(mgr, file)
	if err != nil {
		t.Fatalf("ReadRoleLocator() error = %v", err)
	}
	if content != body {
		t.Errorf("content = %q, want %q", content, body)
	}
	if path != file {
		t.Errorf("path = %q, want its own on-disk path %q", path, file)
	}
}
