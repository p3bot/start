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
		// orderable — singular and plural, lowercase
		{"context", "contexts", true},
		{"contexts", "contexts", true},
		{"role", "roles", true},
		{"roles", "roles", true},
		// orderable — mixed case
		{"Context", "contexts", true},
		{"Role", "roles", true},
		{"ROLES", "roles", true},
		{"CONTEXTS", "contexts", true},
		// non-orderable known categories
		{"agent", "", true},
		{"agents", "", true},
		{"task", "", true},
		{"tasks", "", true},
		// truly unknown
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

	// Input: move item 2 up, then save
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

	// Move gamma (3) up twice to reach top, then save
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

	// Position 1 is already at top, then save
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
	// Order should be unchanged
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

	// Immediately save without changes
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

	// Original order should be unchanged
	if order[0] != "alpha" || order[1] != "beta" || order[2] != "gamma" {
		t.Errorf("original order was mutated: %v", order)
	}
}

func TestWriteContextsFile_PreservesOrder(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "contexts.cue")

	contexts := map[string]ContextConfig{
		"zebra": {
			Name: "zebra",
			File: "zebra.md",
		},
		"alpha": {
			Name: "alpha",
			File: "alpha.md",
		},
		"middle": {
			Name: "middle",
			File: "middle.md",
		},
	}

	// Write in non-alphabetical order
	order := []string{"zebra", "alpha", "middle"}
	err := writeContextsFile(path, contexts, order)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)
	zebraIdx := strings.Index(contentStr, `"zebra"`)
	alphaIdx := strings.Index(contentStr, `"alpha"`)
	middleIdx := strings.Index(contentStr, `"middle"`)

	if zebraIdx == -1 || alphaIdx == -1 || middleIdx == -1 {
		t.Fatalf("missing contexts in output: %s", contentStr)
	}

	if zebraIdx >= alphaIdx || alphaIdx >= middleIdx {
		t.Errorf("order not preserved: zebra=%d, alpha=%d, middle=%d\n%s",
			zebraIdx, alphaIdx, middleIdx, contentStr)
	}
}

func TestWriteRolesFile_PreservesOrder(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roles.cue")

	roles := map[string]RoleConfig{
		"zebra": {
			Name:   "zebra",
			Prompt: "Zebra role",
		},
		"alpha": {
			Name:   "alpha",
			Prompt: "Alpha role",
		},
		"middle": {
			Name:   "middle",
			Prompt: "Middle role",
		},
	}

	order := []string{"zebra", "alpha", "middle"}
	err := writeRolesFile(path, roles, order)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)
	zebraIdx := strings.Index(contentStr, `"zebra"`)
	alphaIdx := strings.Index(contentStr, `"alpha"`)
	middleIdx := strings.Index(contentStr, `"middle"`)

	if zebraIdx == -1 || alphaIdx == -1 || middleIdx == -1 {
		t.Fatalf("missing roles in output: %s", contentStr)
	}

	if zebraIdx >= alphaIdx || alphaIdx >= middleIdx {
		t.Errorf("order not preserved: zebra=%d, alpha=%d, middle=%d\n%s",
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

	contexts := map[string]ContextConfig{
		"gamma": {Name: "gamma", File: "gamma.md", Required: true},
		"alpha": {Name: "alpha", File: "alpha.md", Default: true},
		"beta":  {Name: "beta", File: "beta.md"},
	}
	order := []string{"gamma", "alpha", "beta"}

	if err := writeContextsFile(path, contexts, order); err != nil {
		t.Fatalf("write failed: %v", err)
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

	roles := map[string]RoleConfig{
		"gamma": {Name: "gamma", Prompt: "Gamma role"},
		"alpha": {Name: "alpha", Prompt: "Alpha role"},
		"beta":  {Name: "beta", Prompt: "Beta role"},
	}
	order := []string{"gamma", "alpha", "beta"}

	if err := writeRolesFile(path, roles, order); err != nil {
		t.Fatalf("write failed: %v", err)
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

// Note: Tests below use os.Chdir (process-global state). Do not add t.Parallel()
// to any test that calls os.Chdir — it will cause data races on the working directory.

func TestConfigContextOrder_Command(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write contexts in specific order
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

	// Move item 2 (alpha) up, then save
	stdout := &bytes.Buffer{}
	if err := reorderContexts(stdout, strings.NewReader("2\n\n"), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Order saved") {
		t.Errorf("expected 'Order saved' in output, got: %s", output)
	}

	// Verify the file was rewritten with new order
	content, err := os.ReadFile(filepath.Join(globalDir, "contexts.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	// After moving alpha up: alpha, zebra, middle
	alphaIdx := strings.Index(contentStr, `"alpha"`)
	zebraIdx := strings.Index(contentStr, `"zebra"`)
	middleIdx := strings.Index(contentStr, `"middle"`)

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

	// Move item 2 (alpha) up, then save
	stdout := &bytes.Buffer{}
	if err := reorderRoles(stdout, strings.NewReader("2\n\n"), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Order saved") {
		t.Errorf("expected 'Order saved' in output, got: %s", output)
	}

	// Verify the file was rewritten with new order
	content, err := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	// After moving alpha up: alpha, zebra
	alphaIdx := strings.Index(contentStr, `"alpha"`)
	zebraIdx := strings.Index(contentStr, `"zebra"`)

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

	// Move item 2 up, then cancel
	stdout := &bytes.Buffer{}
	if err := reorderContexts(stdout, strings.NewReader("2\nq\n"), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Cancelled") {
		t.Errorf("expected 'Cancelled' in output, got: %s", output)
	}

	// Verify the file was NOT rewritten (original order preserved)
	content, err := os.ReadFile(filepath.Join(globalDir, "contexts.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	// Original order: zebra, alpha
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

	// Test reorder with single item (save immediately)
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

	// Write contexts in specific order: zebra, alpha
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

	// Add a new context "beta" - should appear at end
	// Prompts: name, description (empty), content choice (Enter=default file), file path, required (N), default (N), tags (skip)
	if err := configContextAdd(slowStdin("beta\n\n\nbeta.md\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	// Verify order: zebra, alpha, beta (not alphabetical)
	content, err := os.ReadFile(filepath.Join(globalDir, "contexts.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	zebraIdx := strings.Index(contentStr, `"zebra"`)
	alphaIdx := strings.Index(contentStr, `"alpha"`)
	betaIdx := strings.Index(contentStr, `"beta"`)

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

	// Write roles in specific order: zebra, alpha
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

	// Add a new role "beta"
	// Prompts: name, description (empty), content choice "3" (inline prompt), prompt text, blank line to finish, tags (skip)
	if err := configRoleAdd(slowStdin("beta\n\n3\nBeta role\n\n\n"), &bytes.Buffer{}, false); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	// Verify order: zebra, alpha, beta (not alphabetical)
	content, err := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	contentStr := string(content)

	zebraIdx := strings.Index(contentStr, `"zebra"`)
	alphaIdx := strings.Index(contentStr, `"alpha"`)
	betaIdx := strings.Index(contentStr, `"beta"`)

	if zebraIdx >= alphaIdx || alphaIdx >= betaIdx {
		t.Errorf("expected order zebra < alpha < beta, got zebra=%d, alpha=%d, beta=%d\n%s",
			zebraIdx, alphaIdx, betaIdx, contentStr)
	}
}

func TestConfigRoleList_SortsAlphabetically(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Define roles in non-alphabetical order
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

	// Alphabetical order: alpha < middle < zebra
	if alphaIdx >= middleIdx || middleIdx >= zebraIdx {
		t.Errorf("role list not sorted alphabetically (expected alpha < middle < zebra): alpha=%d, middle=%d, zebra=%d\noutput: %s",
			alphaIdx, middleIdx, zebraIdx, output)
	}
}
