package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/p3bot/agentdex"
	"github.com/p3bot/agentdex/modelsdev"
	"github.com/p3bot/start/internal/fault"
	"github.com/p3bot/start/internal/orchestration"
)

func TestLaunchAgentdex_JoinedDryRunUsesCatalogBin(t *testing.T) {
	tmpDir := isolateConfigEnv(t)
	chdir(t, tmpDir)
	home := os.Getenv("HOME")
	bin := filepath.Join(home, "bin-claude-code")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeJoinedAgentConfig(t, tmpDir, `agents: {
	"claude-code/interactive": {
		agentdex: "claude-code"
		command: "{{.bin}} --model {{.model}} {{.prompt}}"
		default_model: "sonnet"
		models: { sonnet: "sonnet", haiku: "haiku" }
	}
}
settings: { default_agent: "claude-code/interactive" }
`)

	flags := &Flags{
		DryRun:             true,
		NoRole:             true,
		NoImplicitContexts: true,
		catalogOpts:        fixtureCatalogOpts(t, home, []string{"claude-code"}),
	}
	var stdout, stderr bytes.Buffer
	err := executeStart(&stdout, &stderr, strings.NewReader(""), flags, orchestration.ContextSelection{}, "hi")
	if err != nil {
		t.Fatalf("executeStart: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	cmd := dryRunCommand(t, out)
	if !strings.Contains(cmd, bin) {
		t.Errorf("joined launch should use catalog bin %q, command:\n%s\nsummary:\n%s", bin, cmd, out)
	}
}

func TestLaunchAgentdex_UnknownIDIsUsage(t *testing.T) {
	tmpDir := isolateConfigEnv(t)
	chdir(t, tmpDir)
	home := os.Getenv("HOME")
	writeJoinedAgentConfig(t, tmpDir, `agents: {
	"broken": {
		agentdex: "not-a-catalog-id"
		command: "{{.bin}}"
	}
}
settings: { default_agent: "broken" }
`)
	flags := &Flags{
		DryRun:             true,
		NoRole:             true,
		NoImplicitContexts: true,
		catalogOpts:        fixtureCatalogOpts(t, home, nil),
	}
	err := executeStart(new(bytes.Buffer), new(bytes.Buffer), strings.NewReader(""), flags, orchestration.ContextSelection{}, "")
	if err == nil {
		t.Fatal("expected usage error for unknown agentdex id")
	}
	if got := ExitCodeFromError(err); got != ExitUsage {
		t.Errorf("exit = %d, want %d (usage); err=%v", got, ExitUsage, err)
	}
	if !strings.Contains(err.Error(), "unknown agentdex id") {
		t.Errorf("error = %v", err)
	}
}

func TestLaunchAgentdex_CustomAgentDoesNotNeedCatalog(t *testing.T) {
	tmpDir := setupStartTestConfig(t)
	chdir(t, tmpDir)
	home := os.Getenv("HOME")
	flags := &Flags{
		DryRun:             true,
		NoRole:             true,
		NoImplicitContexts: true,
		catalogOpts: []agentdex.Option{
			agentdex.WithCatalogDir(filepath.Join(home, "missing-catalog")),
			agentdex.WithLookPath(func(string) (string, error) { return "", os.ErrNotExist }),
			agentdex.WithWorkingDir(home),
		},
	}
	err := executeStart(new(bytes.Buffer), new(bytes.Buffer), strings.NewReader(""), flags, orchestration.ContextSelection{}, "hi")
	if err != nil {
		t.Fatalf("custom agent without join key must launch without agentdex: %v", err)
	}
}

func TestLaunchAgentdex_AliasClaudeSlash(t *testing.T) {
	tmpDir := isolateConfigEnv(t)
	chdir(t, tmpDir)
	home := os.Getenv("HOME")
	if err := os.WriteFile(filepath.Join(home, "bin-claude-code"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJoinedAgentConfig(t, tmpDir, `agents: {
	"claude-code/interactive": {
		agentdex: "claude-code"
		command: "{{.bin}} {{.prompt}}"
	}
}
settings: { default_agent: "echo" }
`)
	flags := &Flags{
		DryRun:             true,
		NoRole:             true,
		NoImplicitContexts: true,
		Agent:              []string{"claude/interactive"},
		catalogOpts:        fixtureCatalogOpts(t, home, []string{"claude-code"}),
	}
	var stdout bytes.Buffer
	err := executeStart(&stdout, new(bytes.Buffer), strings.NewReader(""), flags, orchestration.ContextSelection{}, "hi")
	if err != nil {
		t.Fatalf("aliased launch: %v", err)
	}
	if !strings.Contains(stdout.String(), "Agent: claude-code/interactive") {
		t.Errorf("expected aliased recipe, got:\n%s", stdout.String())
	}
}

func TestLaunchAgentdex_LeftoverExactSkipsAlias(t *testing.T) {
	tmpDir := isolateConfigEnv(t)
	chdir(t, tmpDir)
	writeJoinedAgentConfig(t, tmpDir, `agents: {
	"claude/interactive": {
		bin: "echo"
		command: "{{.bin}} leftover {{.prompt}}"
	}
	"claude-code/interactive": {
		agentdex: "claude-code"
		command: "{{.bin}} joined {{.prompt}}"
	}
}
`)
	flags := &Flags{
		DryRun:             true,
		NoRole:             true,
		NoImplicitContexts: true,
		Agent:              []string{"claude/interactive"},
		catalogOpts: []agentdex.Option{
			agentdex.WithCatalogDir(filepath.Join(os.Getenv("HOME"), "missing-catalog")),
			agentdex.WithLookPath(func(string) (string, error) { return "", os.ErrNotExist }),
		},
	}
	var stdout bytes.Buffer
	err := executeStart(&stdout, new(bytes.Buffer), strings.NewReader(""), flags, orchestration.ContextSelection{}, "hi")
	if err != nil {
		t.Fatalf("leftover launch: %v", err)
	}
	if !strings.Contains(stdout.String(), "Agent: claude/interactive") {
		t.Errorf("exact leftover should launch from CUE, got:\n%s", stdout.String())
	}
	if strings.Contains(dryRunCommand(t, stdout.String()), "joined") {
		t.Errorf("leftover must not use the joined recipe command")
	}
}

func TestLaunchAgentdex_ModelOverlayBeatsLive(t *testing.T) {
	cfg := buildTestCfg(t, `{
		agents: {
			"claude-code/interactive": {
				agentdex: "claude-code"
				command: "{{.bin}}"
				models: { sonnet: "sonnet" }
			}
		}
	}`)
	r := newTestResolver(cfg)
	r.catalogOpts = []agentdex.Option{
		agentdex.WithCatalogDir(filepath.Join(t.TempDir(), "missing")),
	}
	agent := orchestration.Agent{
		Agentdex: "claude-code",
		Models:   map[string]string{"sonnet": "sonnet"},
	}
	if got := r.resolveModelName("sonnet", agent); got != "sonnet" {
		t.Errorf("overlay exact = %q, want sonnet", got)
	}
}

func TestResolveModelName_LiveUniqueMatch(t *testing.T) {
	home := t.TempDir()
	opts := liveModelCatalogOpts(t, home, "claude-haiku-4-5", "claude-sonnet-4-5")
	r := newTestResolver(buildTestCfg(t, `{}`))
	r.catalogOpts = opts
	r.workingDir = home
	agent := orchestration.Agent{Agentdex: "claude-code"}

	if got := r.resolveModelName("haiku", agent); got != "claude-haiku-4-5" {
		t.Errorf("live unique haiku = %q, want claude-haiku-4-5", got)
	}
	if got := r.resolveModelName("anthropic/claude-haiku-4-5", agent); got != "claude-haiku-4-5" {
		t.Errorf("live exact canonical = %q, want agent-facing id", got)
	}
	if got := r.resolveLaunchModel("", orchestration.Agent{
		Agentdex:     "claude-code",
		DefaultModel: "haiku",
	}); got != "claude-haiku-4-5" {
		t.Errorf("default_model via live = %q, want claude-haiku-4-5", got)
	}
	if got := r.resolveModelName("claude", agent); got != "claude" {
		t.Errorf("ambiguous live substring must passthrough, got %q", got)
	}
}

func liveModelCatalogOpts(t *testing.T, home string, ids ...string) []agentdex.Option {
	t.Helper()
	anthropicModels := make(map[string]modelsdev.Model, len(ids))
	agnostic := make(map[string]modelsdev.Model, len(ids))
	for _, id := range ids {
		anthropicModels[id] = modelsdev.Model{ID: id, Name: id, Limit: modelsdev.Limit{Context: 200000}}
		canon := "anthropic/" + id
		agnostic[canon] = modelsdev.Model{ID: canon, Name: id, Limit: modelsdev.Limit{Context: 200000}}
	}
	body, err := json.Marshal(modelsdev.Catalog{
		Models: agnostic,
		Providers: map[string]modelsdev.Provider{
			"anthropic": {ID: "anthropic", Name: "Anthropic", Models: anthropicModels},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	opts := fixtureCatalogOpts(t, home, nil)
	return append(opts,
		agentdex.WithModelsURL(srv.URL),
		agentdex.WithCacheDir(t.TempDir()),
		agentdex.WithModelsFetch(agentdex.FetchCached),
	)
}

func TestResolveModelName_CacheOnlySkipsHTTP(t *testing.T) {
	home := t.TempDir()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(srv.Close)

	r := newTestResolver(buildTestCfg(t, `{}`))
	r.workingDir = home
	r.catalogOpts = append(fixtureCatalogOpts(t, home, nil),
		agentdex.WithModelsURL(srv.URL),
		agentdex.WithCacheDir(t.TempDir()),
	)
	agent := orchestration.Agent{Agentdex: "claude-code"}
	if got := r.resolveModelName("haiku", agent); got != "haiku" {
		t.Errorf("cache-only miss must passthrough, got %q", got)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("models.dev HTTP hits = %d, want 0", n)
	}
}

func TestResolveModelName_RefreshFetchesModels(t *testing.T) {
	home := t.TempDir()
	var hits atomic.Int32
	id := "claude-haiku-4-5"
	body, err := json.Marshal(modelsdev.Catalog{
		Models: map[string]modelsdev.Model{
			"anthropic/" + id: {ID: "anthropic/" + id, Name: id, Limit: modelsdev.Limit{Context: 200000}},
		},
		Providers: map[string]modelsdev.Provider{
			"anthropic": {ID: "anthropic", Name: "Anthropic", Models: map[string]modelsdev.Model{
				id: {ID: id, Name: id, Limit: modelsdev.Limit{Context: 200000}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	r := newTestResolver(buildTestCfg(t, `{}`))
	r.flags.Refresh = true
	r.flags.catalogOpts = append(fixtureCatalogOpts(t, home, nil),
		agentdex.WithModelsURL(srv.URL),
		agentdex.WithCacheDir(t.TempDir()),
	)
	r.catalogOpts = r.flags.agentdexOpts()
	r.workingDir = home
	agent := orchestration.Agent{Agentdex: "claude-code"}
	if got := r.resolveModelName("haiku", agent); got != id {
		t.Errorf("refresh live haiku = %q, want %s", got, id)
	}
	if n := hits.Load(); n == 0 {
		t.Error("--refresh must fetch models.dev; got 0 HTTP hits")
	}
}

func TestLaunchAgentdex_ModelPassthroughWhenLiveFails(t *testing.T) {
	r := newTestResolver(buildTestCfg(t, `{}`))
	r.catalogOpts = []agentdex.Option{
		agentdex.WithCatalogDir(filepath.Join(t.TempDir(), "missing")),
	}
	agent := orchestration.Agent{Agentdex: "claude-code"}
	if got := r.resolveModelName("claude-opus-4-7", agent); got != "claude-opus-4-7" {
		t.Errorf("passthrough = %q", got)
	}
}

func TestComputeWantLive_LeftoverClaudeAliasForcesLive(t *testing.T) {
	home := t.TempDir()
	opts := fixtureCatalogOpts(t, home, []string{"claude-code"})
	cfg := buildTestCfg(t, `{
		agents: {
			"claude/interactive": { bin: "echo", command: "{{.bin}}" }
		}
	}`)
	r := newTestResolver(cfg)
	r.catalogOpts = opts
	r.workingDir = home
	flags := &Flags{Agent: []string{"claude"}}

	if r.computeWantLive(baseSurfaces(flags, "claude")) {
		t.Fatal("precondition: unre-written claude substring-matches leftover and must stay cache-gated")
	}

	rewritten := r.prepareLaunchAgent("claude")
	if rewritten != "agents:claude-code" {
		t.Fatalf("prepareLaunchAgent = %q, want agents:claude-code", rewritten)
	}
	if !r.computeWantLive(baseSurfaces(flags, rewritten)) {
		t.Error("leftover claude/interactive plus aliased claude-code must force live when claude-code/* is not installed")
	}
}

func TestApplyLaunchAgentAlias_CaseInsensitive(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"claude", "claude-code"},
		{"Claude", "claude-code"},
		{"CLAUDE", "claude-code"},
		{"claude/interactive", "claude-code/interactive"},
		{"Claude/interactive", "claude-code/interactive"},
		{"Claude/Interactive", "claude-code/interactive"},
		{"agents:Claude", "agents:claude-code"},
		{"agents:Claude/edit", "agents:claude-code/edit"},
		{"claude-code", "claude-code"},
		{"gemini", "gemini"},
	}
	for _, tt := range tests {
		if got := applyLaunchAgentAlias(tt.in); got != tt.want {
			t.Errorf("applyLaunchAgentAlias(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRewriteLaunchAgent_CatalogPrefix(t *testing.T) {
	home := t.TempDir()
	opts := fixtureCatalogOpts(t, home, []string{"claude-code"})
	cfg := buildTestCfg(t, `{
		agents: {
			"claude-code/interactive": { command: "{{.bin}}" }
			"claude-code/edit": { command: "{{.bin}}" }
		}
	}`)
	got := rewriteLaunchAgent("claude-code", cfg.Value, home, opts...)
	if got != "agents:claude-code" {
		t.Errorf("rewrite = %q, want agents:claude-code", got)
	}
	r := newTestResolver(cfg)
	r.catalogOpts = opts
	r.workingDir = home
	name, err := r.resolveAgent("claude-code")
	if err == nil {
		t.Fatalf("non-TTY multi prefix match should be ambiguous, got %q", name)
	}
	if ExitCodeFromError(err) != ExitUsage && !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("want ambiguous, got %v (exit %d)", err, ExitCodeFromError(err))
	}
}

func TestRewriteLaunchAgent_UnknownBareKeepsSubstring(t *testing.T) {
	home := t.TempDir()
	opts := fixtureCatalogOpts(t, home, nil)
	cfg := buildTestCfg(t, `{
		agents: {
			"claude-code/interactive": { command: "{{.bin}}" }
		}
	}`)
	got := rewriteLaunchAgent("inter", cfg.Value, home, opts...)
	if got != "inter" {
		t.Errorf("rewrite = %q, want bare inter (substring fallback)", got)
	}
	r := newTestResolver(cfg)
	r.catalogOpts = opts
	r.workingDir = home
	name, err := r.resolveAgent("inter")
	if err != nil {
		t.Fatalf("substring fallback: %v", err)
	}
	if name != "claude-code/interactive" {
		t.Errorf("resolveAgent(inter) = %q, want claude-code/interactive", name)
	}
}

func TestJoinAgent_UnknownIsUsage(t *testing.T) {
	home := t.TempDir()
	opts := fixtureCatalogOpts(t, home, nil)
	agent := orchestration.Agent{Name: "x", Agentdex: "not-a-catalog-id", Command: "{{.bin}}"}
	_, err := orchestration.JoinAgent(t.Context(), agent, home, opts...)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, fault.ErrUsage) {
		t.Errorf("want usage, got %v", err)
	}
}

func dryRunCommand(t *testing.T, summary string) string {
	t.Helper()
	const prefix = "Files: "
	idx := strings.Index(summary, prefix)
	if idx < 0 {
		t.Fatalf("no Files: line in dry-run summary:\n%s", summary)
	}
	rest := summary[idx+len(prefix):]
	dir, _, _ := strings.Cut(rest, "\n")
	data, err := os.ReadFile(filepath.Join(strings.TrimSpace(dir), "command.txt"))
	if err != nil {
		t.Fatalf("reading command.txt: %v", err)
	}
	return string(data)
}

func TestGetAgent_DefaultModelOverlaySubstring(t *testing.T) {
	tmpDir := isolateConfigEnv(t)
	chdir(t, tmpDir)
	writeJoinedAgentConfig(t, tmpDir, `agents: {
	"echo": {
		bin: "echo"
		command: "{{.bin}} --model {{.model}}"
		default_model: "son"
		models: { sonnet: "claude-sonnet-4-5", opus: "claude-opus-4" }
	}
}
`)
	stdout, stderr, err := runGetCmd(t, "echo")
	if err != nil {
		t.Fatalf("get: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "claude-sonnet-4-5") {
		t.Errorf("default_model son should expand like launch, got: %q", stdout)
	}
	if strings.Contains(stdout, "--model son\n") || strings.HasSuffix(strings.TrimSpace(stdout), "--model son") {
		t.Errorf("unexpanded default_model leaked, got: %q", stdout)
	}
}

func TestDescribe_DefaultModelOverlaySubstring(t *testing.T) {
	tmpDir := isolateConfigEnv(t)
	chdir(t, tmpDir)
	writeJoinedAgentConfig(t, tmpDir, `agents: {
	"echo": {
		bin: "echo"
		command: "{{.bin}} --model {{.model}}"
		default_model: "son"
		models: { sonnet: "claude-sonnet-4-5", opus: "claude-opus-4" }
	}
}
`)
	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"describe", "echo"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("describe: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "claude-sonnet-4-5") {
		t.Errorf("describe Command line should expand default_model son like launch, got:\n%s", stdout)
	}
}

func TestGet_DoesNotApplyLaunchAlias(t *testing.T) {
	tmpDir := isolateConfigEnv(t)
	chdir(t, tmpDir)
	writeJoinedAgentConfig(t, tmpDir, `agents: {
	"claude-code/interactive": {
		agentdex: "claude-code"
		command: "{{.bin}} {{.prompt}}"
	}
}
`)
	stdout, stderr, err := runGetCmd(t, "agents:claude/interactive")
	if err == nil {
		t.Fatalf("get must not alias leftover names, stdout=%s stderr=%s", stdout, stderr)
	}
	if got := ExitCodeFromError(err); got != ExitNotFound {
		t.Errorf("exit = %d, want %d (not-found); err=%v", got, ExitNotFound, err)
	}
}

func TestDescribe_JoinedCatalogUnavailable(t *testing.T) {
	tmpDir := isolateConfigEnv(t)
	chdir(t, tmpDir)
	home := os.Getenv("HOME")
	writeJoinedAgentConfig(t, tmpDir, `agents: {
	"claude-code/interactive": {
		agentdex: "claude-code"
		command: "{{.bin}} {{.prompt}}"
	}
}
`)
	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	ctx := WithSkillCatalogOpts(cmd.Context(),
		agentdex.WithCatalogDir(filepath.Join(home, "missing-catalog")),
		agentdex.WithLookPath(func(string) (string, error) { return "", os.ErrNotExist }),
		agentdex.WithWorkingDir(home),
	)
	cmd.SetContext(ctx)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"describe", "claude-code/interactive"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("describe must fail when join fails, stdout=%s", stdout)
	}
	if !strings.Contains(err.Error(), "agent catalog unavailable") && !strings.Contains(err.Error(), "unknown agentdex") {
		t.Errorf("want catalog join error, got %v", err)
	}
	if got := ExitCodeFromError(err); got != ExitConfig {
		t.Errorf("exit = %d, want %d (invalid catalog dir); err=%v", got, ExitConfig, err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Agent: claude-code/interactive") {
		t.Errorf("describe should still dump the record when join fails, got:\n%s", out)
	}
	if !strings.Contains(out, "agentdex") {
		t.Errorf("dump should include the join key, got:\n%s", out)
	}
}

func TestDescribe_DoesNotApplyLaunchAlias(t *testing.T) {
	tmpDir := isolateConfigEnv(t)
	chdir(t, tmpDir)
	writeJoinedAgentConfig(t, tmpDir, `agents: {
	"claude-code/interactive": {
		agentdex: "claude-code"
		command: "{{.bin}} {{.prompt}}"
	}
}
`)
	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"describe", "agents:claude/interactive"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("describe must not alias leftover names, stdout=%s stderr=%s", stdout, stderr)
	}
	if got := ExitCodeFromError(err); got != ExitNotFound {
		t.Errorf("exit = %d, want %d (not-found); err=%v", got, ExitNotFound, err)
	}
}

func writeJoinedAgentConfig(t *testing.T, tmpDir, body string) {
	t.Helper()
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "settings.cue"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
