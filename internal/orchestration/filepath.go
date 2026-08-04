package orchestration

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/p3bot/start/internal/temp"
)

// IsFilePath returns true if the string looks like a file path.
// Only bare ~ and ~/ count as tilde paths; ~user is unsupported by ExpandTilde.
func IsFilePath(s string) bool {
	if s == "" {
		return false
	}
	return strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "/") ||
		s == "~" ||
		strings.HasPrefix(s, "~/")
}

// IsRemoteLocator reports whether s is an http(s) URL — a remote locator the
// user supplied at the CLI. Only the two web schemes count; every other scheme
// is left to name resolution. The scheme is matched case-insensitively, as URL
// schemes are per RFC 3986.
func IsRemoteLocator(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// IsLocator reports whether s is a locator the read seam handles directly: a
// local file path or a remote http(s) URL. The resolver's bypass gate and every
// user-supplied file surface test this instead of IsFilePath.
func IsLocator(s string) bool {
	return IsFilePath(s) || IsRemoteLocator(s)
}

// ReadLocator reads a locator's content, dispatching a local path to the
// filesystem read and an http(s) URL to a network fetch under the default
// timeout and size cap.
func ReadLocator(locator string) (string, error) {
	if IsRemoteLocator(locator) {
		return remoteFetcher{}.fetch(locator)
	}
	return ReadFilePath(locator)
}

// ReadRoleLocator reads a --role locator and guarantees an on-disk path, which
// the role surface needs because {{role_file}} is a filesystem path. A local
// locator returns its own expanded on-disk location, unchanged. A remote
// locator's fetched content is materialised through the UTD temp manager
// (.start/temp/) — the same mechanism inline roles use — keyed by the locator so
// a re-run overwrites the same file.
func ReadRoleLocator(tempManager *temp.Manager, locator string) (content, path string, err error) {
	if IsRemoteLocator(locator) {
		content, err = remoteFetcher{}.fetch(locator)
		if err != nil {
			return "", "", err
		}
		path, err = tempManager.WriteUTDFile("role", locator, content)
		if err != nil {
			return "", "", fmt.Errorf("writing role temp file: %w", err)
		}
		return content, path, nil
	}

	content, err = ReadFilePath(locator)
	if err != nil {
		return "", "", err
	}
	path, err = ExpandFilePath(locator)
	if err != nil {
		return "", "", err
	}
	return content, path, nil
}

// defaultFetchTimeout bounds an entire remote fetch including the body read, so
// no surface hangs on a slow or stalled response. defaultFetchMaxBytes bounds
// the response body. Both are generous for human-authored prompt, role, and
// context content while capping a pathological response.
const (
	defaultFetchTimeout  = 30 * time.Second
	defaultFetchMaxBytes = 10 << 20 // 10 MB
)

// remoteFetcher fetches remote locator content over http(s). Its zero value uses
// the package defaults; tests set Timeout and MaxBytes to drive the timeout and
// oversize thresholds against in-process handlers without the production values.
type remoteFetcher struct {
	Timeout  time.Duration
	MaxBytes int64
}

// fetch retrieves the locator's body under the configured timeout and size cap.
// A non-2xx status, a text/html body, or a body exceeding the cap is an error
// naming the locator. The standard http client speaks only http(s) and refuses
// a redirect to any other scheme, so no network read escapes those two schemes.
func (f remoteFetcher) fetch(url string) (string, error) {
	timeout := f.Timeout
	if timeout <= 0 {
		timeout = defaultFetchTimeout
	}
	maxBytes := f.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultFetchMaxBytes
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetching %s: %s", url, resp.Status)
	}

	if err := rejectHTML(url, resp.Header.Get("Content-Type")); err != nil {
		return "", err
	}

	// Read one byte past the cap so an exactly-cap body passes and an over-cap
	// body is detectable without buffering an unbounded response.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("fetching %s: response exceeds %d byte limit", url, maxBytes)
	}

	return string(body), nil
}

// rejectHTML refuses a text/html response, which would otherwise inject a
// rendered file page or a soft-404 served with status 200 verbatim into the
// prompt. The media type is compared case-insensitively with charset and other
// parameters stripped. A malformed parameter must not be an escape hatch, so a
// body whose bare type is text/html is refused even when ParseMediaType reports
// a parameter error; only an unparseable main type falls back to the bare type.
// Every other content type, and a missing header, is read verbatim.
func rejectHTML(url, contentType string) error {
	if contentType == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil && mediaType == "" {
		mediaType, _, _ = strings.Cut(contentType, ";")
		mediaType = strings.TrimSpace(mediaType)
	}
	if !strings.EqualFold(mediaType, "text/html") {
		return nil
	}
	msg := fmt.Sprintf("fetching %s: refusing text/html response (expected plain text or Markdown)", url)
	if hint := rawGitHubHint(url); hint != "" {
		msg += fmt.Sprintf("; try the raw URL %s", hint)
	}
	return errors.New(msg)
}

// rawGitHubHint maps a github.com blob URL to its raw.githubusercontent.com
// equivalent for the text/html rejection message, returning empty for anything
// that is not a github.com /blob/ page. This is a message hint only — the URL is
// never rewritten and re-fetched.
func rawGitHubHint(rawURL string) string {
	u, err := neturl.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Host, "github.com") {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "blob" {
		return ""
	}
	owner, repo := parts[0], parts[1]
	rest := strings.Join(parts[3:], "/")
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", owner, repo, rest)
}

// ExpandTilde expands a leading ~ or ~/ to the user's home directory.
// Only handles bare ~ and ~/path, not ~user syntax.
func ExpandTilde(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// ExpandFilePath expands tilde and converts to absolute path.
func ExpandFilePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	expanded, err := ExpandTilde(path)
	if err != nil {
		return "", err
	}

	return filepath.Abs(expanded)
}

// ReadFilePath reads the content of a file path, expanding tilde if present.
func ReadFilePath(path string) (string, error) {
	expanded, err := ExpandFilePath(path)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(expanded)
	if err != nil {
		return "", err
	}

	return string(content), nil
}
