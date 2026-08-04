package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"github.com/p3bot/start/internal/modules"
)

// TestUpsert_FreshFileWritesManagedHeader covers requirement 5: a file created
// fresh by the add path carries the same managed header install writes.
func TestUpsert_FreshFileWritesManagedHeader(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "roles.cue")

	if err := upsertRole(path, RoleConfig{Name: "go-expert", Prompt: "Go expert."}); err != nil {
		t.Fatalf("upsertRole: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"// start configuration", "// Managed by 'start install'"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("fresh file missing managed header line %q, got:\n%s", want, got)
		}
	}
}

// TestUpsertAdd_PreservesHeaderAndSiblingComments covers requirement 1: adding an
// entry retains the comment header, existing entries, and their comments.
func TestUpsertAdd_PreservesHeaderAndSiblingComments(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "contexts.cue")
	initial := `// start configuration
// Managed by 'start install'
contexts: {
	// alpha is first
	alpha: {
		file: "alpha.md"
	}
	// publisher pulls in extra modules
	publisher: {
		file:  "pub.md"
		uses: ["contexts:start/library/publishing"]
	}
}
`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := upsertContext(path, ContextConfig{Name: "beta", File: "beta.md"}); err != nil {
		t.Fatalf("upsertContext: %v", err)
	}

	got := readString(t, path)
	for _, want := range []string{
		"// Managed by 'start install'",
		"// alpha is first",
		"// publisher pulls in extra modules",
		"beta:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("after add, output missing %q:\n%s", want, got)
		}
	}
}

// TestUpsertEdit_PreservesHeaderAndSiblingComments covers requirement 2: editing
// one entry retains the header and sibling entries with their comments.
func TestUpsertEdit_PreservesHeaderAndSiblingComments(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "contexts.cue")
	initial := `// start configuration
// Managed by 'start install'
contexts: {
	alpha: {
		file: "alpha.md"
	}
	// keep the publisher comment
	publisher: {
		file: "pub.md"
	}
}
`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := upsertContext(path, ContextConfig{Name: "alpha", File: "alpha.md", Description: "Edited."}); err != nil {
		t.Fatalf("upsertContext: %v", err)
	}

	got := readString(t, path)
	for _, want := range []string{
		"// Managed by 'start install'",
		"// keep the publisher comment",
		`description: "Edited."`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("after edit, output missing %q:\n%s", want, got)
		}
	}
}

// TestInstallOrderEdit_RoundTripPreservesComments covers the acceptance criterion
// that install → order → edit leaves comments intact at every step.
func TestInstallOrderEdit_RoundTripPreservesComments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "roles.cue")

	// Install (fresh file) writes the managed header.
	if err := modules.UpsertConfigModule(path, "roles", "go-expert", roleEntry(RoleConfig{Name: "go-expert", Prompt: "Go expert."})); err != nil {
		t.Fatalf("install go-expert: %v", err)
	}
	// Install a second module so order has something to reorder.
	if err := modules.UpsertConfigModule(path, "roles", "reviewer", roleEntry(RoleConfig{Name: "reviewer", Prompt: "Reviewer."})); err != nil {
		t.Fatalf("install reviewer: %v", err)
	}

	// Hand-add a user comment on reviewer to track preservation through later steps.
	withComment := strings.Replace(readString(t, path), "reviewer:", "// user note on reviewer\n\treviewer:", 1)
	if err := os.WriteFile(path, []byte(withComment), 0o644); err != nil {
		t.Fatal(err)
	}

	// Order: move reviewer ahead of go-expert.
	if err := modules.ReorderConfigCategory(path, "roles", []string{"reviewer", "go-expert"}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	afterOrder := readString(t, path)
	if !strings.Contains(afterOrder, "// Managed by 'start install'") || !strings.Contains(afterOrder, "// user note on reviewer") {
		t.Fatalf("comments lost after order:\n%s", afterOrder)
	}
	// go-expert keeps its quotes (the hyphen is not a bare identifier); reviewer
	// simplifies to an unquoted label.
	noteIdx := strings.Index(afterOrder, "// user note on reviewer")
	reviewerIdx := strings.Index(afterOrder, "reviewer:")
	goExpertIdx := strings.Index(afterOrder, `"go-expert"`)
	if reviewerIdx >= goExpertIdx {
		t.Errorf("reorder did not move reviewer ahead:\n%s", afterOrder)
	}
	// The comment must travel with its node: it should sit immediately above
	// reviewer (after the move it precedes both reviewer and go-expert), proving
	// the reorder moved the AST node rather than leaving the comment behind.
	if noteIdx >= reviewerIdx || noteIdx >= goExpertIdx {
		t.Errorf("user comment detached from reviewer after reorder:\n%s", afterOrder)
	}

	// Edit go-expert; reviewer's user comment and the header must survive.
	if err := upsertRole(path, RoleConfig{Name: "go-expert", Prompt: "Edited expert."}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	afterEdit := readString(t, path)
	for _, want := range []string{"// Managed by 'start install'", "// user note on reviewer", "Edited expert."} {
		if !strings.Contains(afterEdit, want) {
			t.Errorf("after edit, missing %q:\n%s", want, afterEdit)
		}
	}
}

// TestPromptHeredoc_RoundTrip covers the prompt acceptance criteria: a multi-line
// prompt is stored as a """ heredoc (not an escaped single-line string) and
// round-trips to its exact value through both the add/edit and install paths,
// including backslashes, \(...) interpolation, and embedded """.
func TestPromptHeredoc_RoundTrip(t *testing.T) {
	t.Parallel()
	const tricky = "line one\nbackslash \\n and interp \\(foo)\nembedded \"\"\" marker\nlast line"

	t.Run("add path", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "roles.cue")
		if err := upsertRole(path, RoleConfig{Name: "go-expert", Prompt: tricky}); err != nil {
			t.Fatalf("upsertRole: %v", err)
		}
		assertHeredocRoundTrip(t, path, tricky)
	})

	t.Run("install path", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "roles.cue")
		v := cuecontext.New().Encode(map[string]any{"prompt": tricky})
		entry, err := modules.FormatModuleStruct(v, "roles", "github.com/x/roles/go@v0.1.0", "")
		if err != nil {
			t.Fatalf("FormatModuleStruct: %v", err)
		}
		if err := modules.UpsertConfigModule(path, "roles", "go-expert", entry); err != nil {
			t.Fatalf("UpsertConfigModule: %v", err)
		}
		assertHeredocRoundTrip(t, path, tricky)
	})
}

// TestSingleLinePrompt_NotHeredoc confirms short single-line prompts stay quoted
// strings rather than gratuitously becoming heredocs.
func TestSingleLinePrompt_NotHeredoc(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "roles.cue")
	if err := upsertRole(path, RoleConfig{Name: "go-expert", Prompt: "One line prompt."}); err != nil {
		t.Fatalf("upsertRole: %v", err)
	}
	got := readString(t, path)
	if strings.Contains(got, `"""`) {
		t.Errorf("single-line prompt should not use a heredoc:\n%s", got)
	}
	if !strings.Contains(got, `prompt: "One line prompt."`) {
		t.Errorf("single-line prompt not stored as quoted string:\n%s", got)
	}
}

// TestEntryByteIdenticalToInstall covers the acceptance criterion that, for scalar
// and list fields, the add/edit writers produce byte-identical output to install
// for the same content. Both sides render through the shared CategoryFieldOrder and
// field encoders, so the formatted entry structs must match exactly.
func TestEntryByteIdenticalToInstall(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	t.Run("agent", func(t *testing.T) {
		t.Parallel()
		const origin = "github.com/x/agents/claude@v0.1.0"
		// No models: agents are the only category mixing scalar fields with the
		// sorted-models exception, so omitting models keeps full byte-identity in
		// play across bin/command/default_model/description/tags/uses.
		v := ctx.CompileString(`{
			description: "Claude agent"
			tags: ["claude", "anthropic"]
			uses: ["contexts:start/library/publishing"]
			bin: "claude"
			command: "claude --model {{model}}"
			default_model: "claude-opus-4-8"
		}`)
		installStruct, err := modules.FormatModuleStruct(v, "agents", origin, "")
		if err != nil {
			t.Fatalf("FormatModuleStruct: %v", err)
		}
		add := agentEntry(AgentConfig{
			Name:         "claude",
			Origin:       origin,
			Description:  "Claude agent",
			Tags:         []string{"claude", "anthropic"},
			Uses:         []string{"contexts:start/library/publishing"},
			Bin:          "claude",
			Command:      "claude --model {{model}}",
			DefaultModel: "claude-opus-4-8",
		})
		assertSameBytes(t, installStruct, add)
	})

	t.Run("role", func(t *testing.T) {
		t.Parallel()
		const origin = "github.com/x/roles/go@v0.1.0"
		v := ctx.CompileString(`{
			description: "Go expert"
			tags: ["go", "expert"]
			uses: ["contexts:start/library/publishing"]
			file: "go.md"
			optional: true
		}`)
		installStruct, err := modules.FormatModuleStruct(v, "roles", origin, "")
		if err != nil {
			t.Fatalf("FormatModuleStruct: %v", err)
		}
		add := roleEntry(RoleConfig{
			Name:        "go",
			Origin:      origin,
			Description: "Go expert",
			Tags:        []string{"go", "expert"},
			Uses:        []string{"contexts:start/library/publishing"},
			File:        "go.md",
			Optional:    true,
		})
		assertSameBytes(t, installStruct, add)
	})

	t.Run("context", func(t *testing.T) {
		t.Parallel()
		const origin = "github.com/x/contexts/cwd@v0.1.0"
		v := ctx.CompileString(`{
			description: "CWD context"
			tags: ["cwd"]
			file: "AGENTS.md"
			required: true
		}`)
		installStruct, err := modules.FormatModuleStruct(v, "contexts", origin, "")
		if err != nil {
			t.Fatalf("FormatModuleStruct: %v", err)
		}
		add := contextEntry(ContextConfig{
			Name:        "cwd",
			Origin:      origin,
			Description: "CWD context",
			Tags:        []string{"cwd"},
			File:        "AGENTS.md",
			Required:    true,
		})
		assertSameBytes(t, installStruct, add)
	})

	t.Run("task", func(t *testing.T) {
		t.Parallel()
		const origin = "github.com/x/tasks/review@v0.1.0"
		v := ctx.CompileString(`{
			description: "Review changes"
			tags: ["review", "git"]
			role: "code-reviewer"
			command: "git diff --staged"
		}`)
		installStruct, err := modules.FormatModuleStruct(v, "tasks", origin, "")
		if err != nil {
			t.Fatalf("FormatModuleStruct: %v", err)
		}
		add := taskEntry(TaskConfig{
			Name:        "review",
			Origin:      origin,
			Description: "Review changes",
			Tags:        []string{"review", "git"},
			Role:        "code-reviewer",
			Command:     "git diff --staged",
		})
		assertSameBytes(t, installStruct, add)
	})
}

func assertHeredocRoundTrip(t *testing.T, path, want string) {
	t.Helper()
	got := readString(t, path)
	if !strings.Contains(got, `"""`) {
		t.Errorf("multi-line prompt not stored as heredoc:\n%s", got)
	}
	roles, _, err := loadRolesFromDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	role, ok := roles["go-expert"]
	if !ok {
		t.Fatalf("go-expert missing after write:\n%s", got)
	}
	if role.Prompt != want {
		t.Errorf("prompt did not round-trip:\n got: %q\nwant: %q", role.Prompt, want)
	}
}

func assertSameBytes(t *testing.T, installStruct, addStruct *ast.StructLit) {
	t.Helper()
	installBytes, err := format.Node(installStruct, format.Simplify())
	if err != nil {
		t.Fatalf("format install struct: %v", err)
	}
	addBytes, err := format.Node(addStruct, format.Simplify())
	if err != nil {
		t.Fatalf("format add struct: %v", err)
	}
	if string(installBytes) != string(addBytes) {
		t.Errorf("add output not byte-identical to install\n--- install ---\n%s\n--- add ---\n%s", installBytes, addBytes)
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
