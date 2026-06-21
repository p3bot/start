package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveOrderCategory(t *testing.T) {
	tests := []struct {
		arg   string
		want  string
		known bool
	}{
		{"context", "contexts", true},
		{"contexts", "contexts", true},
		{"role", "roles", true},
		{"roles", "roles", true},
		{"Context", "contexts", true},
		{"Role", "roles", true},
		{"ROLES", "roles", true},
		{"CONTEXTS", "contexts", true},
		{"agent", "", true},
		{"agents", "", true},
		{"task", "", true},
		{"tasks", "", true},
		{"xyz", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		name := tc.arg
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			got, known := resolveOrderCategory(tc.arg)
			if got != tc.want || known != tc.known {
				t.Errorf("resolveOrderCategory(%q) = (%q, %v), want (%q, %v)", tc.arg, got, known, tc.want, tc.known)
			}
		})
	}
}

func TestConfigOrder_NonTerminal_RejectsAllArgs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	for _, arg := range []string{"context", "role", "agent", "task", "xyz"} {
		t.Run(arg, func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetIn(strings.NewReader(""))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"config", "order", arg})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "interactive reordering requires a terminal") {
				t.Errorf("expected terminal error, got %v", err)
			}
		})
	}
}

func TestRunReorderLoop_MoveUp(t *testing.T) {
	order := []string{"alpha", "beta", "gamma"}
	formatItem := func(i int, name string) string {
		return "  " + name
	}

	input := "2\n\n"
	stdout := &bytes.Buffer{}

	result, saved, err := runReorderLoop(stdout, strings.NewReader(input), "Test:", order, formatItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !saved {
		t.Fatal("expected saved=true")
	}

	expected := []string{"beta", "alpha", "gamma"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(result))
	}
	for i, name := range expected {
		if result[i] != name {
			t.Errorf("position %d: expected %q, got %q", i, name, result[i])
		}
	}
}

func TestRunReorderLoop_MoveToTop(t *testing.T) {
	order := []string{"alpha", "beta", "gamma"}
	formatItem := func(i int, name string) string {
		return "  " + name
	}

	input := "3\n2\n\n"
	stdout := &bytes.Buffer{}

	result, saved, err := runReorderLoop(stdout, strings.NewReader(input), "Test:", order, formatItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !saved {
		t.Fatal("expected saved=true")
	}

	expected := []string{"gamma", "alpha", "beta"}
	for i, name := range expected {
		if result[i] != name {
			t.Errorf("position %d: expected %q, got %q", i, name, result[i])
		}
	}
}

func TestRunReorderLoop_Cancel(t *testing.T) {
	order := []string{"alpha", "beta", "gamma"}
	formatItem := func(i int, name string) string {
		return "  " + name
	}

	tests := []struct {
		name  string
		input string
	}{
		{"q", "q\n"},
		{"quit", "quit\n"},
		{"exit", "exit\n"},
		{"Q uppercase", "Q\n"},
		{"QUIT uppercase", "QUIT\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			_, saved, err := runReorderLoop(stdout, strings.NewReader(tt.input), "Test:", order, formatItem)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if saved {
				t.Fatal("expected saved=false for cancel")
			}
		})
	}
}

func TestRunReorderLoop_AlreadyAtTop(t *testing.T) {
	order := []string{"alpha", "beta"}
	formatItem := func(i int, name string) string {
		return "  " + name
	}

	input := "1\n\n"
	stdout := &bytes.Buffer{}

	result, saved, err := runReorderLoop(stdout, strings.NewReader(input), "Test:", order, formatItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !saved {
		t.Fatal("expected saved=true")
	}
	if stdout.String() == "" {
		t.Fatal("expected output")
	}
	if !strings.Contains(stdout.String(), "Already at top") {
		t.Errorf("expected 'Already at top' message, got: %s", stdout.String())
	}
	if result[0] != "alpha" || result[1] != "beta" {
		t.Errorf("order should be unchanged, got: %v", result)
	}
}

func TestRunReorderLoop_InvalidInput(t *testing.T) {
	order := []string{"alpha", "beta"}
	formatItem := func(i int, name string) string {
		return "  " + name
	}

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{"text", "abc\n\n", "Invalid input"},
		{"zero", "0\n\n", "Invalid number"},
		{"too high", "5\n\n", "Invalid number"},
		{"negative", "-1\n\n", "Invalid number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			_, _, err := runReorderLoop(stdout, strings.NewReader(tt.input), "Test:", order, formatItem)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(stdout.String(), tt.contains) {
				t.Errorf("expected %q in output, got: %s", tt.contains, stdout.String())
			}
		})
	}
}

func TestRunReorderLoop_SaveEmpty(t *testing.T) {
	order := []string{"alpha", "beta", "gamma"}
	formatItem := func(i int, name string) string {
		return "  " + name
	}

	input := "\n"
	stdout := &bytes.Buffer{}

	result, saved, err := runReorderLoop(stdout, strings.NewReader(input), "Test:", order, formatItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !saved {
		t.Fatal("expected saved=true")
	}
	expected := []string{"alpha", "beta", "gamma"}
	for i, name := range expected {
		if result[i] != name {
			t.Errorf("position %d: expected %q, got %q", i, name, result[i])
		}
	}
}

func TestRunReorderLoop_DoesNotMutateInput(t *testing.T) {
	order := []string{"alpha", "beta", "gamma"}
	formatItem := func(i int, name string) string {
		return "  " + name
	}

	input := "2\n\n"
	stdout := &bytes.Buffer{}

	_, _, err := runReorderLoop(stdout, strings.NewReader(input), "Test:", order, formatItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if order[0] != "alpha" || order[1] != "beta" || order[2] != "gamma" {
		t.Errorf("original order was mutated: %v", order)
	}
}

// Sequential upserts preserve insertion order rather than re-sorting entries,
// the guarantee the legacy order-slice writer carried.
func TestUpsertContext_PreservesInsertionOrder(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "contexts.cue")

	for _, name := range []string{"zebra", "alpha", "middle"} {
		if err := upsertContext(path, ContextConfig{Name: name, File: name + ".md"}); err != nil {
			t.Fatalf("upsertContext(%q): %v", name, err)
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)
	zebraIdx := strings.Index(contentStr, `zebra:`)
	alphaIdx := strings.Index(contentStr, `alpha:`)
	middleIdx := strings.Index(contentStr, `middle:`)

	if zebraIdx == -1 || alphaIdx == -1 || middleIdx == -1 {
		t.Fatalf("missing contexts in output: %s", contentStr)
	}

	if zebraIdx >= alphaIdx || alphaIdx >= middleIdx {
		t.Errorf("insertion order not preserved: zebra=%d, alpha=%d, middle=%d\n%s",
			zebraIdx, alphaIdx, middleIdx, contentStr)
	}
}

func TestUpsertRole_PreservesInsertionOrder(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roles.cue")

	for _, name := range []string{"zebra", "alpha", "middle"} {
		if err := upsertRole(path, RoleConfig{Name: name, Prompt: name + " role"}); err != nil {
			t.Fatalf("upsertRole(%q): %v", name, err)
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)
	zebraIdx := strings.Index(contentStr, `zebra:`)
	alphaIdx := strings.Index(contentStr, `alpha:`)
	middleIdx := strings.Index(contentStr, `middle:`)

	if zebraIdx == -1 || alphaIdx == -1 || middleIdx == -1 {
		t.Fatalf("missing roles in output: %s", contentStr)
	}

	if zebraIdx >= alphaIdx || alphaIdx >= middleIdx {
		t.Errorf("insertion order not preserved: zebra=%d, alpha=%d, middle=%d\n%s",
			zebraIdx, alphaIdx, middleIdx, contentStr)
	}
}

func TestLoadRolesFromDir_ReturnsOrder(t *testing.T) {
	tmpDir := t.TempDir()

	rolesContent := `roles: {
	"zebra": {
		prompt: "Zebra role"
	}
	"alpha": {
		prompt: "Alpha role"
	}
	"middle": {
		prompt: "Middle role"
	}
}`
	if err := os.WriteFile(filepath.Join(tmpDir, "roles.cue"), []byte(rolesContent), 0644); err != nil {
		t.Fatal(err)
	}

	roles, order, err := loadRolesFromDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(roles) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(roles))
	}

	expected := []string{"zebra", "alpha", "middle"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d order entries, got %d", len(expected), len(order))
	}
	for i, name := range expected {
		if order[i] != name {
			t.Errorf("order[%d]: expected %q, got %q", i, name, order[i])
		}
	}
}

func TestWriteReadRoundTrip_Contexts(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "contexts.cue")

	contexts := []ContextConfig{
		{Name: "gamma", File: "gamma.md", Required: true},
		{Name: "alpha", File: "alpha.md", Default: true},
		{Name: "beta", File: "beta.md"},
	}
	order := []string{"gamma", "alpha", "beta"}

	for _, ctx := range contexts {
		if err := upsertContext(path, ctx); err != nil {
			t.Fatalf("upsertContext(%q): %v", ctx.Name, err)
		}
	}

	loaded, loadedOrder, err := loadContextsFromDir(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loaded) != 3 {
		t.Fatalf("expected 3 contexts, got %d", len(loaded))
	}
	for i, name := range order {
		if loadedOrder[i] != name {
			t.Errorf("order[%d]: expected %q, got %q", i, name, loadedOrder[i])
		}
	}
}

func TestWriteReadRoundTrip_Roles(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roles.cue")

	roles := []RoleConfig{
		{Name: "gamma", Prompt: "Gamma role"},
		{Name: "alpha", Prompt: "Alpha role"},
		{Name: "beta", Prompt: "Beta role"},
	}
	order := []string{"gamma", "alpha", "beta"}

	for _, role := range roles {
		if err := upsertRole(path, role); err != nil {
			t.Fatalf("upsertRole(%q): %v", role.Name, err)
		}
	}

	loaded, loadedOrder, err := loadRolesFromDir(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loaded) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(loaded))
	}
	for i, name := range order {
		if loadedOrder[i] != name {
			t.Errorf("order[%d]: expected %q, got %q", i, name, loadedOrder[i])
		}
	}
}

// Tests use os.Chdir (process-global): do not add t.Parallel() or the working directory races.

func TestConfigContextOrder_Command(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	contextsContent := `contexts: {
	"zebra": {
		file: "zebra.md"
	}
	"alpha": {
		file: "alpha.md"
	}
	"middle": {
		file: "middle.md"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "contexts.cue"), []byte(contextsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	stdout := &bytes.Buffer{}
	if err := reorderContexts(stdout, strings.NewReader("2\n\n"), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Order saved") {
		t.Errorf("expected 'Order saved' in output, got: %s", output)
	}

	content, err := os.ReadFile(filepath.Join(globalDir, "contexts.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	// After moving alpha up: alpha, zebra, middle
	alphaIdx := strings.Index(contentStr, `alpha:`)
	zebraIdx := strings.Index(contentStr, `zebra:`)
	middleIdx := strings.Index(contentStr, `middle:`)

	if alphaIdx >= zebraIdx || zebraIdx >= middleIdx {
		t.Errorf("expected order alpha < zebra < middle, got alpha=%d, zebra=%d, middle=%d\n%s",
			alphaIdx, zebraIdx, middleIdx, contentStr)
	}
}

func TestConfigRoleOrder_Command(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	rolesContent := `roles: {
	"zebra": {
		prompt: "Zebra role"
	}
	"alpha": {
		prompt: "Alpha role"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "roles.cue"), []byte(rolesContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	stdout := &bytes.Buffer{}
	if err := reorderRoles(stdout, strings.NewReader("2\n\n"), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Order saved") {
		t.Errorf("expected 'Order saved' in output, got: %s", output)
	}

	content, err := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	// After moving alpha up: alpha, zebra
	alphaIdx := strings.Index(contentStr, `alpha:`)
	zebraIdx := strings.Index(contentStr, `zebra:`)

	if alphaIdx >= zebraIdx {
		t.Errorf("expected alpha before zebra, got alpha=%d, zebra=%d\n%s",
			alphaIdx, zebraIdx, contentStr)
	}
}

func TestConfigContextOrder_Cancel(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	contextsContent := `contexts: {
	"zebra": {
		file: "zebra.md"
	}
	"alpha": {
		file: "alpha.md"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "contexts.cue"), []byte(contextsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	stdout := &bytes.Buffer{}
	if err := reorderContexts(stdout, strings.NewReader("2\nq\n"), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Cancelled") {
		t.Errorf("expected 'Cancelled' in output, got: %s", output)
	}

	content, err := os.ReadFile(filepath.Join(globalDir, "contexts.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	// Cancel must leave the file byte-for-byte unmodified, including its original
	// quoted-name form.
	zebraIdx := strings.Index(contentStr, `"zebra"`)
	alphaIdx := strings.Index(contentStr, `"alpha"`)

	if zebraIdx >= alphaIdx {
		t.Errorf("file should not have been modified, got zebra=%d, alpha=%d",
			zebraIdx, alphaIdx)
	}
}

func TestConfigContextOrder_NoContexts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	stdout := &bytes.Buffer{}
	if err := reorderContexts(stdout, strings.NewReader(""), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No contexts configured") {
		t.Errorf("expected 'No contexts configured' in output, got: %s", output)
	}
}

func TestConfigContextOrder_SingleItem(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	contextsContent := `contexts: {
	"alpha": {
		file: "alpha.md"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "contexts.cue"), []byte(contextsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	stdout := &bytes.Buffer{}
	if err := reorderContexts(stdout, strings.NewReader("\n"), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Order saved") {
		t.Errorf("expected 'Order saved' in output, got: %s", output)
	}
}

func TestConfigContextAdd_PreservesOrder(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	contextsContent := `contexts: {
	"zebra": {
		file: "zebra.md"
	}
	"alpha": {
		file: "alpha.md"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "contexts.cue"), []byte(contextsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	// New context should append, not sort alphabetically.
	// Prompts: name, description (empty), content choice (Enter=default file), file path, required (N), default (N), tags (skip)
	if err := configContextAdd(slowStdin("beta\n\n\nbeta.md\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(globalDir, "contexts.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	zebraIdx := strings.Index(contentStr, `zebra:`)
	alphaIdx := strings.Index(contentStr, `alpha:`)
	betaIdx := strings.Index(contentStr, `beta:`)

	if zebraIdx >= alphaIdx || alphaIdx >= betaIdx {
		t.Errorf("expected order zebra < alpha < beta, got zebra=%d, alpha=%d, beta=%d\n%s",
			zebraIdx, alphaIdx, betaIdx, contentStr)
	}
}

func TestConfigRoleAdd_PreservesOrder(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	rolesContent := `roles: {
	"zebra": {
		prompt: "Zebra role"
	}
	"alpha": {
		prompt: "Alpha role"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "roles.cue"), []byte(rolesContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	// New role should append, not sort alphabetically.
	// Prompts: name, description (empty), content choice "3" (inline prompt), prompt text, blank line to finish, tags (skip)
	if err := configRoleAdd(slowStdin("beta\n\n3\nBeta role\n\n\n"), &bytes.Buffer{}, false); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	zebraIdx := strings.Index(contentStr, `zebra:`)
	alphaIdx := strings.Index(contentStr, `alpha:`)
	betaIdx := strings.Index(contentStr, `beta:`)

	if zebraIdx >= alphaIdx || alphaIdx >= betaIdx {
		t.Errorf("expected order zebra < alpha < beta, got zebra=%d, alpha=%d, beta=%d\n%s",
			zebraIdx, alphaIdx, betaIdx, contentStr)
	}
}

func TestConfigRoleList_PreservesInjectionOrder(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	rolesContent := `roles: {
	"zebra": {
		prompt: "Zebra role"
		description: "Zebra role (defined first)"
	}
	"alpha": {
		prompt: "Alpha role"
		description: "Alpha role (defined second)"
	}
	"middle": {
		prompt: "Middle role"
		description: "Middle role (defined third)"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "roles.cue"), []byte(rolesContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "role"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()

	zebraIdx := strings.Index(output, "zebra")
	alphaIdx := strings.Index(output, "alpha")
	middleIdx := strings.Index(output, "middle")

	if zebraIdx == -1 || alphaIdx == -1 || middleIdx == -1 {
		t.Fatalf("expected all roles in output, got: %s", output)
	}

	// config list preserves CUE definition order ("injection order"), not alphabetical.
	if zebraIdx >= alphaIdx || alphaIdx >= middleIdx {
		t.Errorf("role list not in injection order (expected zebra < alpha < middle): zebra=%d, alpha=%d, middle=%d\noutput: %s",
			zebraIdx, alphaIdx, middleIdx, output)
	}
}
