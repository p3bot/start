package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/p3bot/start/internal/registry"
	"github.com/p3bot/start/internal/skills"
)

func TestGetIDPrintsResolvedAddress(t *testing.T) {
	setupGetTestConfig(t)

	cases := []struct {
		query string
		want  string
	}{
		{"task-prompt", "tasks:task-prompt\n"},
		{"tasks:task-prompt", "tasks:task-prompt\n"},
		{"task-p", "tasks:task-prompt\n"},
		{"tasks:Task-Prompt", "tasks:task-prompt\n"},
		{"claude", "agents:claude\n"},
		{"role-prompt", "roles:role-prompt\n"},
		{"role-file", "roles:role-file\n"},
		{"ctx-cmd", "contexts:ctx-cmd\n"},
		{"role-empty", "roles:role-empty\n"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			stdout, stderr, err := runGetCmd(t, "--id", tc.query)
			if err != nil {
				t.Fatalf("get --id %s: %v\nstderr: %s", tc.query, err, stderr)
			}
			if stdout != tc.want {
				t.Errorf("stdout = %q, want %q", stdout, tc.want)
			}
			for _, banned := range []string{"Hello", "Role file contents", "cmd-output", "{{.bin}}", "Type:", "Name:"} {
				if strings.Contains(stdout, banned) {
					t.Errorf("stdout contains %q: %q", banned, stdout)
				}
			}
		})
	}

	stdout, _, err := runGetCmd(t, "--id", "tasks:task")
	if err == nil {
		t.Fatal("expected prefix ambiguity")
	}
	for _, want := range []string{"tasks:task-prompt", "tasks:task-cmd", "category-qualified name"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("prefix error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "run interactively") {
		t.Errorf("prefix error offers a menu: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestGetIDVerboseStaysOnStderr(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "--verbose", "--id", "role-prompt")
	if err != nil {
		t.Fatalf("get --id --verbose: %v\nstderr: %s", err, stderr)
	}
	if stdout != "roles:role-prompt\n" {
		t.Errorf("stdout = %q, want the id line", stdout)
	}
	for _, banned := range []string{"Type:", "Name:", "Hello"} {
		if strings.Contains(stdout, banned) {
			t.Errorf("stdout contains %q: %q", banned, stdout)
		}
	}
	for _, want := range []string{"Type: Role\n", "Name: role-prompt\n"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q\n%s", want, stderr)
		}
	}

	stdout, stderr, err = runGetCmd(t, "--verbose", "--id", "ctx-cmd")
	if err != nil {
		t.Fatalf("get --id --verbose ctx-cmd: %v\nstderr: %s", err, stderr)
	}
	if stdout != "contexts:ctx-cmd\n" {
		t.Errorf("stdout = %q, want the id line", stdout)
	}
	if !strings.Contains(stderr, "Type: Context\n") || !strings.Contains(stderr, "Command: printf 'cmd-output'\n") {
		t.Errorf("stderr missing command metadata:\n%s", stderr)
	}

	stdout, stderr, err = runGetCmd(t, "--verbose", "--id", "role-traced")
	if err != nil {
		t.Fatalf("get --id --verbose role-traced: %v\nstderr: %s", err, stderr)
	}
	if stdout != "roles:role-traced\n" {
		t.Errorf("stdout = %q, want the id line", stdout)
	}
	if strings.Contains(stdout, "traced contents") {
		t.Errorf("stdout rendered the file: %q", stdout)
	}
	for _, want := range []string{"Type: Role\n", "Name: role-traced\n", "Origin: github.com/example/start-library/roles/traced@v1.2.3\n", "Path: "} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q\n%s", want, stderr)
		}
	}

	stdout, stderr, err = runGetCmd(t, "--verbose", "--id", "claude")
	if err != nil {
		t.Fatalf("get --id --verbose claude: %v\nstderr: %s", err, stderr)
	}
	if stdout != "agents:claude\n" {
		t.Errorf("stdout = %q, want the id line", stdout)
	}
	for _, want := range []string{"Type: Agent\n", "Name: claude\n"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q\n%s", want, stderr)
		}
	}
}

func TestGetIDInstallsRegistryModule(t *testing.T) {
	home, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
	stub.SetFetch(sentinelModuleBase, registry.FetchResult{
		SourceDir: buildModuleFixture(t, "agent", agentFixtureBody),
	}, nil)

	stdout, stderr, err := runGetWithStub(t, stub, "--verbose", "--id", sentinelAgentName)
	if err != nil {
		t.Fatalf("get --id: %v\nstderr: %s", err, stderr)
	}
	if stdout != "agents:"+sentinelAgentName+"\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "{{.bin}}") || strings.Contains(stdout, "sentinel run") {
		t.Fatalf("stdout rendered the agent: %q", stdout)
	}
	for _, want := range []string{
		"Type: Agent\n",
		"Name: " + sentinelAgentName + "\n",
		"Origin: github.com/p3bot/library/agents/" + sentinelAgentName + "@" + stubVersion + "\n",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q\n%s", want, stderr)
		}
	}
	installed, err := os.ReadFile(filepath.Join(home, "start", "agents.cue"))
	if err != nil {
		t.Fatalf("reading installed agent: %v", err)
	}
	if !strings.Contains(string(installed), sentinelAgentName) {
		t.Errorf("global config missing %s:\n%s", sentinelAgentName, installed)
	}
	if !strings.Contains(string(installed), "@"+stubVersion) {
		t.Errorf("installed origin missing %s:\n%s", stubVersion, installed)
	}
}

func TestGetIDInstallFailurePrintsNoID(t *testing.T) {
	home, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
	stub.SetFetch(sentinelModuleBase, registry.FetchResult{}, fmt.Errorf("fetch failed"))

	stdout, _, err := runGetWithStub(t, stub, "--id", sentinelAgentName)
	if err == nil {
		t.Fatal("expected the install to fail")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if _, statErr := os.Stat(filepath.Join(home, "start", "agents.cue")); !os.IsNotExist(statErr) {
		data, _ := os.ReadFile(filepath.Join(home, "start", "agents.cue"))
		if strings.Contains(string(data), sentinelAgentName) {
			t.Fatalf("failed install wrote %s:\n%s", sentinelAgentName, data)
		}
	}
}

func runGetWithStub(t *testing.T, stub *registryStub, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	ctx := WithProvider(cmd.Context(), func() (registry.Client, error) {
		return stub, nil
	})
	ctx = withResolverSource(ctx, injectedIndexSource{index: stub.idx, client: stub})
	cmd.SetContext(ctx)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"get"}, args...))
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestGetIDVerboseReportsBrokenPath(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "--verbose", "--id", "role-module-no-origin")
	if err == nil {
		t.Fatal("expected the missing-origin error")
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if strings.Contains(stderr, "Type:") || strings.Contains(stderr, "Name:") {
		t.Errorf("stderr looks like successful metadata:\n%s", stderr)
	}
	if !strings.Contains(err.Error(), "@module/ file path but no origin field") {
		t.Errorf("error = %v", err)
	}
	if IsSilentError(err) {
		t.Fatal("the path error must reach stderr through main")
	}
	if ExitCodeFromError(err) != ExitFailure {
		t.Errorf("exit = %d, want %d", ExitCodeFromError(err), ExitFailure)
	}

	stdout, stderr, err = runGetCmd(t, "--id", "role-module-no-origin")
	if err != nil {
		t.Fatalf("id without verbose: %v", err)
	}
	if stdout != "roles:role-module-no-origin\n" {
		t.Errorf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}

	stdout, stderr, err = runGetCmd(t, "--verbose", "--id", "role-empty")
	if err != nil {
		t.Fatalf("empty module: %v\nstderr: %s", err, stderr)
	}
	if stdout != "roles:role-empty\n" {
		t.Errorf("stdout = %q", stdout)
	}
	for _, want := range []string{"Type: Role\n", "Name: role-empty\n"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q\n%s", want, stderr)
		}
	}
}

func TestGetIDRejectsAmbiguousMissingAndUsage(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)

	stdout, stderr, err := runGetCmd(t, "--id", "helper")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if ExitCodeFromError(err) != ExitUsage {
		t.Fatalf("exit = %d, err = %v", ExitCodeFromError(err), err)
	}
	if IsSilentError(err) {
		t.Fatal("ambiguity must reach stderr through main's error printer")
	}
	for _, want := range []string{"roles:helper", "tasks:helper", "exact name", "category-qualified name"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "run interactively") || strings.Contains(err.Error(), "Select") {
		t.Errorf("error offers a menu: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if strings.Contains(stderr, "Select") || strings.Contains(stderr, "Found") {
		t.Errorf("stderr opened a menu:\n%s", stderr)
	}

	stdout, _, err = runGetCmd(t, "helper")
	if err == nil || !strings.Contains(err.Error(), "run interactively") {
		t.Fatalf("non-id ambiguity should keep the interactive hint, got %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}

	stdout, _, err = runGetCmd(t, "--id", "definitely-not-a-real-module-zzz")
	if err == nil {
		t.Fatal("expected not-found")
	}
	if ExitCodeFromError(err) != ExitNotFound {
		t.Fatalf("exit = %d, err = %v", ExitCodeFromError(err), err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}

	stdout, _, err = runGetCmd(t, "--id", "ab")
	if err == nil || !strings.Contains(err.Error(), "3 characters") {
		t.Fatalf("short query: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}

	stdout, _, err = runGetCmd(t, "--id")
	if err == nil || !strings.Contains(err.Error(), "non-interactive") {
		t.Fatalf("missing name: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}

	for _, args := range [][]string{
		{"--id", "--field", "description", "located"},
		{"--field", "description", "--id"},
	} {
		stdout, _, err = runGetCmd(t, args...)
		if err == nil || !strings.Contains(err.Error(), "--id and --field") {
			t.Fatalf("%v: %v", args, err)
		}
		if ExitCodeFromError(err) != ExitUsage {
			t.Fatalf("%v exit = %d", args, ExitCodeFromError(err))
		}
		if stdout != "" {
			t.Fatalf("%v stdout = %q", args, stdout)
		}
	}
}

func TestGetIDRejectsLocatorWithoutReading(t *testing.T) {
	dir := isolateGetHome(t)
	restoreNoColor(t)

	body := "Direct file body that must not be printed.\n"
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runGetCmd(t, "--id", path)
	if err == nil || !strings.Contains(err.Error(), "--id does not apply") {
		t.Fatalf("file path: %v", err)
	}
	if ExitCodeFromError(err) != ExitUsage {
		t.Fatalf("exit = %d, err = %v", ExitCodeFromError(err), err)
	}
	if stdout != "" || strings.Contains(stdout, "Direct file") {
		t.Fatalf("stdout = %q", stdout)
	}

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("# Remote\n"))
	}))
	defer srv.Close()

	stdout, _, err = runGetCmd(t, "--id", srv.URL+"/note.md")
	if err == nil || !strings.Contains(err.Error(), "--id does not apply") {
		t.Fatalf("url: %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if hits != 0 {
		t.Fatalf("URL was fetched %d times", hits)
	}

	stdout, _, err = runGetCmd(t, "--id", "--field", "description", srv.URL+"/note.md")
	if err == nil || !strings.Contains(err.Error(), "--id and --field") {
		t.Fatalf("combined locator: %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if hits != 0 {
		t.Fatalf("combined flags fetched the URL %d times", hits)
	}
}

func TestGetIDSkillLeafDoesNotFetch(t *testing.T) {
	home, stub, run := setupSkillCmd(t, []string{"agy"})
	stub.SetFetch(skillModulePath, registry.FetchResult{}, fmt.Errorf("id must not fetch"))

	for _, query := range []string{"skills:workflows/one-by-one", "workflows/one-by-one", "skills:one-by-one"} {
		stdout, stderr, err := run("get", "--id", query)
		if err != nil {
			t.Fatalf("get --id %s: %v\nstderr: %s\nstdout: %s", query, err, stderr, stdout)
		}
		if stdout != "skills:workflows/one-by-one\n" {
			t.Errorf("get --id %s stdout = %q", query, stdout)
		}
		if strings.Contains(stdout, "Walk findings") {
			t.Errorf("get --id %s printed the skill body:\n%s", query, stdout)
		}
	}

	stdout, stderr, err := run("get", "--id", "--verbose", "skills:one-by-one")
	if err != nil {
		t.Fatalf("verbose skill id: %v\n%s", err, stderr)
	}
	if stdout != "skills:workflows/one-by-one\n" {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "Skill: workflows/one-by-one\n") {
		t.Errorf("stderr missing skill metadata:\n%s", stderr)
	}
	for _, banned := range []string{"Type:", "Name:"} {
		if strings.Contains(stdout, banned) {
			t.Errorf("stdout contains %q: %q", banned, stdout)
		}
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "start", "skills.cue")); !os.IsNotExist(err) {
		t.Fatal("get --id must not write skills.cue")
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "one-by-one")); !os.IsNotExist(err) {
		t.Fatal("get --id must not create dest dirs")
	}
}

func TestGetIDSkillAmbiguousLeaf(t *testing.T) {
	_, stub, run := setupSkillCmd(t, []string{"agy"})
	if _, _, err := run("install", "skills:workflows/one-by-one"); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "start")
	if err := skills.Upsert(configDir, "other/one-by-one", "github.com/example/other@v1", "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	stub.SetFetch(skillModulePath, registry.FetchResult{}, fmt.Errorf("id must not fetch"))

	stdout, stderr, err := run("get", "--id", "skills:one-by-one")
	if err == nil {
		t.Fatal("expected ambiguous leaf")
	}
	for _, want := range []string{"skills:other/one-by-one", "skills:workflows/one-by-one", "category-qualified name"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "run interactively") {
		t.Errorf("error offers an interactive rerun: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q", stdout)
	}
	if strings.Contains(stderr, "Select") {
		t.Errorf("stderr opened a menu:\n%s", stderr)
	}
}

func TestGetIDHelpAndAgents(t *testing.T) {
	stdout, _, err := runGetCmd(t, "--help")
	if err != nil {
		t.Fatalf("get --help: %v", err)
	}
	if !strings.Contains(stdout, "Print the resolved category:name instead of the module text") {
		t.Errorf("help missing --id description:\n%s", stdout)
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	agents, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(agents)
	if !strings.Contains(text, "start get <name> --id") || !strings.Contains(text, "resolved category:name") {
		t.Error("AGENTS.md does not document start get --id")
	}
}
