package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

func isolateGetHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Cleanup(func() {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return os.Chmod(path, 0o755)
			}
			return os.Chmod(path, 0o644)
		})
	})
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	return dir
}

func writeGetSettings(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, ".start", "settings.cue")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const fieldConfig = `
agents: {
	located: {
		bin: "located"
		command: "{{.bin}} --stay"
		description: "# Title"
		session_locator: ["session_id"]
		tags: ["one", "two"]
		models: sonnet: "claude-sonnet"
		flags: permission: auto: ["--permission-mode", "auto"]
	}
	plain: {
		bin: "plain"
		command: "echo hi"
		description: "Plain"
	}
}
roles: {
	"role-body": {
		file: "/tmp/role.md"
		prompt: "# Hello\n{{.user}}\n"
	}
	helper: prompt: "role helper"
}
contexts: {
	timed: {
		command: "true"
		timeout: 5
		required: false
	}
}
tasks: {
	"named-role": {
		prompt: "body"
		role: "reviewer"
	}
	"inline-role": {
		prompt: "body"
		role: prompt: "inline"
	}
	helper: prompt: "task helper"
}
`

func TestGetFieldSessionLocator(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	stdout, stderr, err := runGetCmd(t, "located", "--field", "session_locator")
	if err != nil {
		t.Fatalf("get: %v\nstderr: %s", err, stderr)
	}
	if stdout != "session_id\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestGetFieldOmittedLocator(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	stdout, stderr, err := runGetCmd(t, "plain", "--field", "session_locator")
	if err != nil {
		t.Fatalf("omitted locator: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
}

func TestGetPlainAgentDoesNotPrintLocator(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	stdout, stderr, err := runGetCmd(t, "located")
	if err != nil {
		t.Fatalf("get: %v\nstderr: %s", err, stderr)
	}
	if stdout != "located --stay\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "session_id") {
		t.Fatalf("plain get printed the locator: %q", stdout)
	}
}

func TestGetFieldCommandIsStoredTemplate(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	stdout, stderr, err := runGetCmd(t, "located", "--field", "command")
	if err != nil {
		t.Fatalf("get: %v\nstderr: %s", err, stderr)
	}
	if stdout != "{{.bin}} --stay\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestGetFieldDescriptionIsUnstyled(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)
	restoreNoColor(t)

	stdout, stderr, err := runGetCmd(t, "located", "--field", "description", "--color=always")
	if err != nil {
		t.Fatalf("get: %v\nstderr: %s", err, stderr)
	}
	if stdout != "# Title\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if hasANSI(stdout) {
		t.Fatalf("description was styled: %q", stdout)
	}
}

func TestGetFieldPromptIsStoredAndUnstyled(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)
	restoreNoColor(t)

	stdout, stderr, err := runGetCmd(t, "role-body", "--field", "prompt", "--color=always")
	if err != nil {
		t.Fatalf("get: %v\nstderr: %s", err, stderr)
	}
	if stdout != "# Hello\n{{.user}}\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if hasANSI(stdout) {
		t.Fatalf("prompt was styled: %q", stdout)
	}
}

func TestGetFieldFileIsPath(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	stdout, stderr, err := runGetCmd(t, "role-body", "--field", "file")
	if err != nil {
		t.Fatalf("get: %v\nstderr: %s", err, stderr)
	}
	if stdout != "/tmp/role.md\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestGetFieldListBoolAndInt(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"located", "--field", "tags"}, "one\ntwo\n"},
		{[]string{"timed", "--field", "timeout"}, "5\n"},
		{[]string{"timed", "--field", "required"}, "false\n"},
		{[]string{"timed", "--field", "default"}, ""},
		{[]string{"named-role", "--field", "role"}, "reviewer\n"},
	}
	for _, tc := range cases {
		stdout, stderr, err := runGetCmd(t, tc.args...)
		if err != nil {
			t.Fatalf("%v: %v\nstderr: %s", tc.args, err, stderr)
		}
		if stdout != tc.want {
			t.Fatalf("%v stdout = %q, want %q", tc.args, stdout, tc.want)
		}
	}
}

func TestGetFieldRefusesShapeAndUnknown(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	cases := [][]string{
		{"located", "--field", "models"},
		{"located", "--field", "flags"},
		{"role-body", "--field", "session_locator"},
		{"inline-role", "--field", "role"},
	}
	for _, args := range cases {
		stdout, _, err := runGetCmd(t, args...)
		if err == nil {
			t.Fatalf("%v: expected error", args)
		}
		if stdout != "" {
			t.Fatalf("%v stdout = %q, want empty", args, stdout)
		}
	}
}

func TestGetFieldMissingModule(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	stdout, _, err := runGetCmd(t, "definitely-not-a-real-module-zzz", "--field", "session_locator")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "definitely-not-a-real-module-zzz") {
		t.Fatalf("error = %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestGetFieldAmbiguous(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	stdout, _, err := runGetCmd(t, "helper", "--field", "description")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	for _, want := range []string{"roles:helper", "tasks:helper"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestGetFieldRejectsLocator(t *testing.T) {
	isolateGetHome(t)
	restoreNoColor(t)

	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("# Body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runGetCmd(t, path, "--field", "description")
	if err == nil {
		t.Fatal("expected error for a file path")
	}
	if ExitCodeFromError(err) != ExitUsage {
		t.Fatalf("exit = %d, err = %v", ExitCodeFromError(err), err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("# Remote\n"))
	}))
	defer srv.Close()

	stdout, _, err = runGetCmd(t, srv.URL+"/note.md", "--field", "description")
	if err == nil {
		t.Fatal("expected error for a URL")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if hits != 0 {
		t.Fatalf("URL was fetched %d times", hits)
	}
}

func TestGetFieldUsage(t *testing.T) {
	isolateGetHome(t)
	cases := [][]string{
		{"--field=", "located"},
		{"--field", "session_locator", "--field", "command", "located"},
		{"located", "--field"},
	}
	for _, args := range cases {
		stdout, _, err := runGetCmd(t, args...)
		if err == nil {
			t.Fatalf("%v: expected usage error", args)
		}
		if ExitCodeFromError(err) != ExitUsage {
			t.Fatalf("%v exit = %d, err = %v", args, ExitCodeFromError(err), err)
		}
		if stdout != "" {
			t.Fatalf("%v stdout = %q", args, stdout)
		}
	}
}

func TestGetFieldSessionLocatorSurvivesRewrite(t *testing.T) {
	dir := isolateGetHome(t)
	path := filepath.Join(dir, ".start", "agents.cue")
	if err := upsertAgent(path, AgentConfig{
		Name:           "located",
		Bin:            "located",
		Command:        "{{.bin}} --stay",
		Description:    "old",
		SessionLocator: []string{"session_id"},
	}); err != nil {
		t.Fatal(err)
	}

	agents, _, err := loadAgentsFromDir(filepath.Join(dir, ".start"))
	if err != nil {
		t.Fatal(err)
	}
	located := agents["located"]
	if len(located.SessionLocator) != 1 || located.SessionLocator[0] != "session_id" {
		t.Fatalf("decoded locator = %#v", located.SessionLocator)
	}
	located.Description = "renamed"
	if err := upsertAgent(path, located); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "session_locator:") || !strings.Contains(text, `"session_id"`) {
		t.Fatalf("rewrite dropped the locator:\n%s", text)
	}
	if !strings.Contains(text, `"renamed"`) {
		t.Fatalf("rewrite did not change the description:\n%s", text)
	}

	stdout, stderr, err := runGetCmd(t, "located", "--field", "session_locator")
	if err != nil {
		t.Fatalf("get: %v\nstderr: %s", err, stderr)
	}
	if stdout != "session_id\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestFormatStoredField(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{
		s: "session_id"
		emptyStr: ""
		kept: "session_id\n"
		multi: "a\nb"
		yes: true
		no: false
		n: 5
		zero: 0
		one: ["session_id"]
		two: ["session_id", "0"]
		empty: []
		obj: { a: "b" }
		nums: [1]
	}`)
	if err := val.Err(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		field string
		want  string
		bad   bool
	}{
		{"s", "session_id\n", false},
		{"emptyStr", "\n", false},
		{"kept", "session_id\n", false},
		{"multi", "a\nb\n", false},
		{"yes", "true\n", false},
		{"no", "false\n", false},
		{"n", "5\n", false},
		{"zero", "0\n", false},
		{"one", "session_id\n", false},
		{"two", "session_id\n0\n", false},
		{"empty", "", false},
		{"obj", "", true},
		{"nums", "", true},
	}
	for _, tc := range cases {
		got, err := formatStoredField(val.LookupPath(cue.ParsePath(tc.field)))
		if tc.bad {
			if err == nil {
				t.Errorf("%s: expected error", tc.field)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tc.field, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.field, got, tc.want)
		}
	}
}
