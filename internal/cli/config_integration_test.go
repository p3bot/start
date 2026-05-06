package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Integration tests for config commands that test the full workflow.
//
// Note: Tests below use os.Chdir (process-global state). Do not add t.Parallel()
// to any test that calls os.Chdir — it will cause data races on the working directory.

func TestConfigAgent_FullWorkflow(t *testing.T) {
	// Setup isolated environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("add agent interactively", func(t *testing.T) {
		// Prompts: name, bin, command, models(skip), defaultModel="sonnet", description, tags(skip)
		stdout := &bytes.Buffer{}
		if err := configAgentAdd(slowStdin("claude\nclaude\n"+`claude --model {{.model}} "{{.prompt}}"`+"\n\nsonnet\nAnthropic Claude\n\n"), stdout, false); err != nil {
			t.Fatalf("add failed: %v", err)
		}

		// Verify file exists
		agentsPath := filepath.Join(globalDir, "agents.cue")
		if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
			t.Fatal("agents.cue was not created")
		}

		content, _ := os.ReadFile(agentsPath)
		if !strings.Contains(string(content), `"claude"`) {
			t.Errorf("agents.cue missing claude: %s", content)
		}
	})

	t.Run("list shows added agent", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "list", "agent"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "claude") {
			t.Errorf("list output missing claude: %s", output)
		}
		if !strings.Contains(output, "Anthropic Claude") {
			t.Errorf("list output missing description: %s", output)
		}
	})

	t.Run("info displays agent details", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "info", "claude"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("info failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "agents/claude") {
			t.Errorf("info output missing agent name: %s", output)
		}
		if !strings.Contains(output, "Default Model: sonnet") {
			t.Errorf("info output missing default model: %s", output)
		}
	})

	t.Run("add second agent", func(t *testing.T) {
		// Prompts: name, bin, command template, default model (empty), description (empty), tags (skip)
		if err := configAgentAdd(slowStdin("gemini\ngemini\n"+`gemini "{{.prompt}}"`+"\n\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("add gemini failed: %v", err)
		}
	})

	t.Run("list shows both agents", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "list", "agent"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "claude") {
			t.Errorf("list output missing claude: %s", output)
		}
		if !strings.Contains(output, "gemini") {
			t.Errorf("list output missing gemini: %s", output)
		}
	})

	t.Run("set default agent via settings", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "settings", "default_agent", "claude"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("settings set failed: %v", err)
		}

		// Verify settings.cue was created with default_agent
		configPath := filepath.Join(globalDir, "settings.cue")
		content, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read settings.cue: %v", err)
		}
		if !strings.Contains(string(content), `default_agent:`) {
			t.Errorf("settings.cue missing default_agent: %s", content)
		}
	})

	t.Run("show default agent via settings", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "settings", "default_agent"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("settings show failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "claude") {
			t.Errorf("expected 'claude' in output, got: %s", output)
		}
	})

	t.Run("remove agent with --yes", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "gemini", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("remove failed: %v", err)
		}

		// Verify gemini is removed
		content, _ := os.ReadFile(filepath.Join(globalDir, "agents.cue"))
		if strings.Contains(string(content), `"gemini"`) {
			t.Errorf("gemini should be removed: %s", content)
		}
		if !strings.Contains(string(content), `"claude"`) {
			t.Errorf("claude should still exist: %s", content)
		}
	})

	t.Run("remove with -y short flag", func(t *testing.T) {
		// Re-add gemini for this test
		if err := configAgentAdd(slowStdin("gemini\ngemini\n"+`gemini "{{.prompt}}"`+"\n\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("re-add gemini failed: %v", err)
		}

		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "gemini", "-y"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("remove with -y failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "agents.cue"))
		if strings.Contains(string(content), `"gemini"`) {
			t.Errorf("gemini should be removed: %s", content)
		}
	})

	t.Run("add duplicate fails", func(t *testing.T) {
		// Provide full add responses; duplicate check fires after all prompts
		err := configAgentAdd(slowStdin("claude\nclaude\n"+`claude "{{.prompt}}"`+"\n\n\n\n\n"), &bytes.Buffer{}, false)
		if err == nil {
			t.Fatal("expected error for duplicate agent")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("expected 'already exists' error, got: %v", err)
		}
	})
}

func TestConfigRole_FullWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("add role with file", func(t *testing.T) {
		// Prompts: name, description, content choice (Enter→"1"→file), file path, optional (N), tags (skip)
		stdout := &bytes.Buffer{}
		if err := configRoleAdd(slowStdin("go-expert\nGo programming expert\n\n~/.config/start/roles/go-expert.md\n\n\n"), stdout, false); err != nil {
			t.Fatalf("add failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
		if !strings.Contains(string(content), `"go-expert"`) {
			t.Errorf("roles.cue missing go-expert: %s", content)
		}
	})

	t.Run("add role with prompt", func(t *testing.T) {
		// Prompts: name, description, content choice "3"→inline, prompt text, blank line to finish, tags (skip)
		if err := configRoleAdd(slowStdin("reviewer\nCode review expert\n3\nYou are a code reviewer. Review code for bugs and style issues.\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("add failed: %v", err)
		}
	})

	t.Run("list shows roles", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "list", "role"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "go-expert") {
			t.Errorf("list missing go-expert: %s", output)
		}
		if !strings.Contains(output, "reviewer") {
			t.Errorf("list missing reviewer: %s", output)
		}
	})

	t.Run("info role details", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "info", "reviewer"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("info failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "roles/reviewer") {
			t.Errorf("info missing role name: %s", output)
		}
		if !strings.Contains(output, "Prompt:") {
			t.Errorf("info missing prompt: %s", output)
		}
	})

	t.Run("remove role with --yes", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "reviewer", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("remove failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
		if strings.Contains(string(content), "reviewer") {
			t.Errorf("reviewer should be removed: %s", content)
		}
		if !strings.Contains(string(content), "go-expert") {
			t.Errorf("go-expert should still exist: %s", content)
		}
	})
}

func TestConfigContext_FullWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("add required context", func(t *testing.T) {
		// Prompts: name, description, content choice (Enter→file), file path, required "y", tags (skip)
		stdout := &bytes.Buffer{}
		if err := configContextAdd(slowStdin("project\nProject documentation\n\nPROJECT.md\ny\n\n"), stdout, false); err != nil {
			t.Fatalf("add failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "contexts.cue"))
		if !strings.Contains(string(content), `"project"`) {
			t.Errorf("contexts.cue missing project: %s", content)
		}
		if !strings.Contains(string(content), "required: true") {
			t.Errorf("contexts.cue missing required flag: %s", content)
		}
	})

	t.Run("add default context", func(t *testing.T) {
		// Prompts: name, description (empty), content choice (Enter→file), file path,
		//          required "n", default "y", tags (skip)
		if err := configContextAdd(slowStdin("readme\n\n\nREADME.md\nn\ny\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("add failed: %v", err)
		}
	})

	t.Run("list shows contexts with markers", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "list", "context"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "[required]") {
			t.Errorf("list missing [required] marker: %s", output)
		}
		if !strings.Contains(output, "[default]") {
			t.Errorf("list missing [default] marker: %s", output)
		}
	})

	t.Run("info context details", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "info", "project"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("info failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "contexts/project") {
			t.Errorf("info missing context name: %s", output)
		}
		if !strings.Contains(output, "Required: true") {
			t.Errorf("info missing required field: %s", output)
		}
	})

	t.Run("remove context with --yes", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "readme", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("remove failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "contexts.cue"))
		if strings.Contains(string(content), "readme") {
			t.Errorf("readme should be removed: %s", content)
		}
		if !strings.Contains(string(content), "project") {
			t.Errorf("project should still exist: %s", content)
		}
	})
}

func TestConfigTask_FullWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("add task with prompt and role", func(t *testing.T) {
		// Prompts: name, description, content choice (Enter→"3"→inline), prompt text,
		//          blank line to finish, role, tags (skip)
		stdout := &bytes.Buffer{}
		if err := configTaskAdd(slowStdin("review\nCode review task\n\nReview this code for bugs and improvements\n\ncode-reviewer\n\n"), stdout, false); err != nil {
			t.Fatalf("add failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "tasks.cue"))
		if !strings.Contains(string(content), `"review"`) {
			t.Errorf("tasks.cue missing review: %s", content)
		}
		if !strings.Contains(string(content), `role:`) {
			t.Errorf("tasks.cue missing role: %s", content)
		}
	})

	t.Run("list shows tasks", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "list", "task"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "review") {
			t.Errorf("list missing review: %s", output)
		}
	})

	t.Run("info task details", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "info", "review"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("info failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "tasks/review") {
			t.Errorf("info missing task name: %s", output)
		}
		if !strings.Contains(output, "Role: code-reviewer") {
			t.Errorf("info missing role: %s", output)
		}
	})

	t.Run("remove task with --yes", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "review", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("remove failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "tasks.cue"))
		if strings.Contains(string(content), `"review"`) {
			t.Errorf("review should be removed: %s", content)
		}
	})
}

func TestConfigLocal_Isolation(t *testing.T) {
	// Test that --local flag properly isolates local and global configs
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	// Create a project directory
	projectDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}

	// Create global config
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("add global agent", func(t *testing.T) {
		if err := configAgentAdd(slowStdin("global-agent\nglobal\n"+`global "{{.prompt}}"`+"\n\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("add global failed: %v", err)
		}
	})

	t.Run("add local agent", func(t *testing.T) {
		if err := configAgentAdd(slowStdin("local-agent\nlocal\n"+`local "{{.prompt}}"`+"\n\n\n\n\n"), &bytes.Buffer{}, true); err != nil {
			t.Fatalf("add local failed: %v", err)
		}

		// Verify local config was created
		localPath := filepath.Join(projectDir, ".start", "agents.cue")
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			t.Fatal("local agents.cue was not created")
		}
	})

	t.Run("list shows both in merged view", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "list", "agent"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "global-agent") {
			t.Errorf("list missing global-agent: %s", output)
		}
		if !strings.Contains(output, "local-agent") {
			t.Errorf("list missing local-agent: %s", output)
		}
	})

	t.Run("list --local shows only local", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "list", "agent", "--local"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list failed: %v", err)
		}

		output := stdout.String()
		if strings.Contains(output, "global-agent") {
			t.Errorf("list --local should not show global-agent: %s", output)
		}
		if !strings.Contains(output, "local-agent") {
			t.Errorf("list --local missing local-agent: %s", output)
		}
	})
}

func TestConfigTask_SubstringResolution(t *testing.T) {
	// Setup isolated environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Add tasks with namespace-style names
	// Prompts: name, description (empty), content choice (Enter→"3"→inline), prompt text,
	//          blank line to finish, role (empty), tags (skip)
	for _, tc := range []struct{ name, prompt string }{
		{"cwd/dotai/create-role", "Create a role"},
		{"confluence/read-doc", "Read a doc"},
		{"golang/review/architecture", "Review arch"},
		{"golang/review/code", "Review code"},
	} {
		if err := configTaskAdd(slowStdin(tc.name+"\n\n\n"+tc.prompt+"\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("add task %s failed: %v", tc.name, err)
		}
	}

	t.Run("info with unique substring", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "info", "create-role"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("info with substring failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "cwd/dotai/create-role") {
			t.Errorf("expected resolved name in output: %s", output)
		}
	})

	t.Run("info with exact match still works", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "info", "confluence/read-doc"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("info with exact name failed: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "confluence/read-doc") {
			t.Errorf("expected exact name in output: %s", output)
		}
	})

	t.Run("info with ambiguous substring in non-interactive mode errors", func(t *testing.T) {
		// "review" matches golang/review/architecture and golang/review/code
		// In non-interactive mode, this should return an error about ambiguity
		cmd := NewRootCmd()
		cmd.SetIn(strings.NewReader("")) // non-interactive stdin
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "info", "review"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for ambiguous match in non-interactive mode")
		}
		if !strings.Contains(err.Error(), "ambiguous") {
			t.Errorf("expected 'ambiguous' error, got: %v", err)
		}
	})

	t.Run("info with no match errors", func(t *testing.T) {
		cmd := NewRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "info", "nonexistent"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for no match")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got: %v", err)
		}
	})

	t.Run("edit with substring interactively", func(t *testing.T) {
		// Interactive edit of "confluence/read-doc":
		// Prompts: description, keep current content? (Enter=Y), role (Enter=keep ""), tags (Enter=keep nil)
		stdout := &bytes.Buffer{}
		if err := configTaskEdit(slowStdin("Updated description\n\n\n\n"), stdout, false, "read-doc"); err != nil {
			t.Fatalf("edit with substring failed: %v", err)
		}

		// Verify the update was applied to the correct task
		cmd2 := NewRootCmd()
		stdout2 := &bytes.Buffer{}
		cmd2.SetOut(stdout2)
		cmd2.SetErr(&bytes.Buffer{})
		cmd2.SetArgs([]string{"config", "info", "confluence/read-doc"})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("info after edit failed: %v", err)
		}
		output := stdout2.String()
		if !strings.Contains(output, "Updated description") {
			t.Errorf("expected updated description in output: %s", output)
		}
	})

	t.Run("remove with exact name and --yes", func(t *testing.T) {
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "cwd/dotai/create-role", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("remove failed: %v", err)
		}

		// Verify it was removed
		content, _ := os.ReadFile(filepath.Join(globalDir, "tasks.cue"))
		if strings.Contains(string(content), "create-role") {
			t.Errorf("create-role should be removed: %s", content)
		}
	})

	t.Run("remove with ambiguous query and --yes removes all matches", func(t *testing.T) {
		// "review" matches golang/review/architecture and golang/review/code
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "golang/review", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("ambiguous remove with --yes failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "tasks.cue"))
		s := string(content)
		if strings.Contains(s, "golang/review/architecture") {
			t.Errorf("architecture should be removed: %s", s)
		}
		if strings.Contains(s, "golang/review/code") {
			t.Errorf("code should be removed: %s", s)
		}
	})

	t.Run("remove with no match errors", func(t *testing.T) {
		cmd := NewRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "does-not-exist", "--yes"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for no match")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got: %v", err)
		}
	})

	t.Run("remove with ambiguous query in non-interactive mode without --yes errors", func(t *testing.T) {
		// Re-add some tasks first
		if err := configTaskAdd(slowStdin("golang/review/security\n\n\nReview security.\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("re-add failed: %v", err)
		}
		if err := configTaskAdd(slowStdin("golang/review/perf\n\n\nReview perf.\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("re-add failed: %v", err)
		}

		cmd := NewRootCmd()
		cmd.SetIn(strings.NewReader("")) // non-interactive
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "golang/review"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for ambiguous remove without --yes in non-interactive mode")
		}
		if !strings.Contains(err.Error(), "--yes") {
			t.Errorf("expected '--yes' hint in error, got: %v", err)
		}
	})
}

func TestConfigRemove_MultipleArgs(t *testing.T) {
	t.Run("task remove ambiguous query with --yes expands all", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		chdir(t, tmpDir)

		globalDir := filepath.Join(tmpDir, "start")
		if err := os.MkdirAll(globalDir, 0755); err != nil {
			t.Fatal(err)
		}

		for _, tc := range []struct{ name, prompt string }{
			{"golang/review/architecture", "Arch"},
			{"golang/review/code", "Code"},
			{"golang/review/security", "Security"},
			{"confluence/read-doc", "Read"},
		} {
			if err := configTaskAdd(slowStdin(tc.name+"\n\n\n"+tc.prompt+"\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
				t.Fatalf("add %s failed: %v", tc.name, err)
			}
		}

		// Ambiguous query with --yes should remove all three golang/review/* tasks
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "golang/review", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("ambiguous remove with --yes failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "tasks.cue"))
		s := string(content)
		if strings.Contains(s, "golang/review/architecture") {
			t.Errorf("architecture should be removed: %s", s)
		}
		if strings.Contains(s, "golang/review/code") {
			t.Errorf("code should be removed: %s", s)
		}
		if strings.Contains(s, "golang/review/security") {
			t.Errorf("security should be removed: %s", s)
		}
		if !strings.Contains(s, "confluence/read-doc") {
			t.Errorf("read-doc should still exist: %s", s)
		}
	})

	t.Run("cross-category remove with --yes removes all matches", func(t *testing.T) {
		// Create an agent and role both named "shared" to test cross-category removal
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		chdir(t, tmpDir)

		globalDir := filepath.Join(tmpDir, "start")
		if err := os.MkdirAll(globalDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Add an agent named "shared"
		if err := configAgentAdd(slowStdin("shared\nshared\n"+`shared "{{.prompt}}"`+"\n\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("add agent failed: %v", err)
		}
		// Add a role named "shared"
		if err := configRoleAdd(slowStdin("shared\n\n3\nShared role prompt.\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("add role failed: %v", err)
		}

		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "shared", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("cross-category remove failed: %v", err)
		}

		// Verify both were removed
		agentContent, _ := os.ReadFile(filepath.Join(globalDir, "agents.cue"))
		if strings.Contains(string(agentContent), `"shared"`) {
			t.Errorf("shared agent should be removed: %s", agentContent)
		}

		roleContent, _ := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
		if strings.Contains(string(roleContent), `"shared"`) {
			t.Errorf("shared role should be removed: %s", roleContent)
		}
	})

	t.Run("agent remove with --yes", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		chdir(t, tmpDir)

		globalDir := filepath.Join(tmpDir, "start")
		if err := os.MkdirAll(globalDir, 0755); err != nil {
			t.Fatal(err)
		}

		for _, name := range []string{"alpha", "beta", "gamma"} {
			if err := configAgentAdd(slowStdin(name+"\n"+name+"\n"+name+` "{{.prompt}}"`+"\n\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
				t.Fatalf("add %s failed: %v", name, err)
			}
		}

		// Remove alpha
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "alpha", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("remove alpha failed: %v", err)
		}

		// Remove beta
		cmd2 := NewRootCmd()
		cmd2.SetOut(&bytes.Buffer{})
		cmd2.SetErr(&bytes.Buffer{})
		cmd2.SetArgs([]string{"config", "remove", "beta", "--yes"})

		if err := cmd2.Execute(); err != nil {
			t.Fatalf("remove beta failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(globalDir, "agents.cue"))
		s := string(content)
		if strings.Contains(s, `"alpha"`) {
			t.Errorf("alpha should be removed: %s", s)
		}
		if strings.Contains(s, `"beta"`) {
			t.Errorf("beta should be removed: %s", s)
		}
		if !strings.Contains(s, `"gamma"`) {
			t.Errorf("gamma should still exist: %s", s)
		}
	})

	t.Run("remove in non-interactive mode without --yes errors", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		chdir(t, tmpDir)

		globalDir := filepath.Join(tmpDir, "start")
		if err := os.MkdirAll(globalDir, 0755); err != nil {
			t.Fatal(err)
		}

		if err := configAgentAdd(slowStdin("testagent\ntest\n"+`test "{{.prompt}}"`+"\n\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("add failed: %v", err)
		}

		// Non-interactive mode without --yes should error
		cmd := NewRootCmd()
		cmd.SetIn(strings.NewReader("")) // non-interactive
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "remove", "testagent"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error requiring --yes in non-interactive mode")
		}
		if !strings.Contains(err.Error(), "--yes") {
			t.Errorf("expected '--yes' in error, got: %v", err)
		}

		// Verify the agent was NOT removed
		content, _ := os.ReadFile(filepath.Join(globalDir, "agents.cue"))
		if !strings.Contains(string(content), "testagent") {
			t.Errorf("testagent should still exist after failed remove: %s", content)
		}
	})
}

func TestConfigAgentAdd_WithModelsAndTags(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Prompts: name, bin, command,
	//          models: "fast=claude-3-haiku\n\n" (one alias then empty to finish),
	//          defaultModel: "1" (select first from list),
	//          description, tags: "ai,coding\n"
	stdout := &bytes.Buffer{}
	err := configAgentAdd(slowStdin("myagent\nmybin\n"+`mybin "{{.prompt}}"`+"\nfast=claude-3-haiku\n\n1\nMy agent\nai,coding\n"), stdout, false)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(globalDir, "agents.cue"))
	s := string(content)
	if !strings.Contains(s, `"fast"`) || !strings.Contains(s, `"claude-3-haiku"`) {
		t.Errorf("agents.cue missing models: %s", s)
	}
	if !strings.Contains(s, `"ai"`) || !strings.Contains(s, `"coding"`) {
		t.Errorf("agents.cue missing tags: %s", s)
	}
}

func TestConfigAgentAdd_NoModelsWhenNoDefaultModel(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Prompts: name, bin, command, models(empty=skip), defaultModel(empty=skip), description, tags(skip)
	stdout := &bytes.Buffer{}
	err := configAgentAdd(slowStdin("nomodel\nbin\n"+`bin "{{.prompt}}"`+"\n\n\nA desc\n\n"), stdout, false)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(globalDir, "agents.cue"))
	s := string(content)
	if strings.Contains(s, "models:") {
		t.Errorf("agents.cue should not have models when no default model: %s", s)
	}
}

func TestConfigRoleAdd_OptionalWithFileSource(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Prompts: name, description, content choice (Enter→file), file path, optional "y", tags (skip)
	err := configRoleAdd(slowStdin("opt-role\nOptional role\n\n~/.config/start/roles/opt.md\ny\n\n"), &bytes.Buffer{}, false)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
	if !strings.Contains(string(content), "optional: true") {
		t.Errorf("roles.cue missing optional: true: %s", content)
	}
}

func TestConfigRoleAdd_NoOptionalWithPromptSource(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Prompts: name, description, content choice "3"→inline, prompt text, blank line, tags (skip)
	// No optional prompt since content source is not file
	err := configRoleAdd(slowStdin("prompt-role\nA role\n3\nDo stuff.\n\n\n"), &bytes.Buffer{}, false)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
	if strings.Contains(string(content), "optional") {
		t.Errorf("roles.cue should not have optional for prompt-based role: %s", content)
	}
}

func TestConfigRoleEdit_OptionalPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Add a role with file source, optional=false
	err := configRoleAdd(slowStdin("edit-role\nEditable\n\n~/.config/start/roles/edit.md\n\n\n"), &bytes.Buffer{}, false)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	// Edit: description (keep), keep content (Y), optional "y", tags (keep)
	err = configRoleEdit(slowStdin("\n\ny\n\n"), &bytes.Buffer{}, false, "edit-role")
	if err != nil {
		t.Fatalf("edit failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(globalDir, "roles.cue"))
	if !strings.Contains(string(content), "optional: true") {
		t.Errorf("roles.cue missing optional: true after edit: %s", content)
	}
}

func TestConfigContextAdd_WithTags(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Prompts: name, description, content choice (Enter→file), file path, required "n", default "n", tags "docs,ref"
	err := configContextAdd(slowStdin("tagged-ctx\nA context\n\nctx.md\nn\nn\ndocs,ref\n"), &bytes.Buffer{}, false)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(globalDir, "contexts.cue"))
	s := string(content)
	if !strings.Contains(s, `"docs"`) || !strings.Contains(s, `"ref"`) {
		t.Errorf("contexts.cue missing tags: %s", s)
	}
}

func TestConfigTaskAdd_WithTags(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Prompts: name, description, content choice (Enter→"3"→inline), prompt, blank line, role (empty), tags "review,go"
	err := configTaskAdd(slowStdin("tagged-task\nA task\n\nDo this.\n\n\nreview,go\n"), &bytes.Buffer{}, false)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(globalDir, "tasks.cue"))
	s := string(content)
	if !strings.Contains(s, `"review"`) || !strings.Contains(s, `"go"`) {
		t.Errorf("tasks.cue missing tags: %s", s)
	}
}

func TestConfigListAll_GroupsCategories(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Seed one item in each category
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(`agents: { "my-agent": { command: "a" } }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "roles.cue"), []byte(`roles: { "my-role": { prompt: "role" } }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "contexts.cue"), []byte(`contexts: { "my-context": { file: "f.md" } }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "tasks.cue"), []byte(`tasks: { "my-task": { prompt: "task" } }`), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config list failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "agents") {
		t.Errorf("expected 'agents' in output: %s", output)
	}
	if !strings.Contains(output, "roles") {
		t.Errorf("expected 'roles' in output: %s", output)
	}
	if !strings.Contains(output, "contexts") {
		t.Errorf("expected 'contexts' in output: %s", output)
	}
	if !strings.Contains(output, "tasks") {
		t.Errorf("expected 'tasks' in output: %s", output)
	}
	if !strings.Contains(output, "my-agent") {
		t.Errorf("expected 'my-agent' in output: %s", output)
	}
}

func TestConfigListPluralAliases(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(`agents: { "myagent": { command: "a" } }`), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	for _, plural := range []string{"agents", "roles", "contexts", "tasks"} {
		t.Run("config list "+plural, func(t *testing.T) {
			cmd := NewRootCmd()
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"config", "list", plural})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("config list %s failed: %v", plural, err)
			}
		})
	}
}

func TestConfigRemovedCommands(t *testing.T) {
	// Verify that the old noun-group command paths return errors.
	for _, tc := range []struct {
		args []string
	}{
		{[]string{"config", "agent"}},
		{[]string{"config", "agent", "add"}},
		{[]string{"config", "agent", "edit", "claude"}},
		{[]string{"config", "agent", "default", "claude"}},
		{[]string{"config", "role", "add"}},
		{[]string{"config", "role", "list"}},
		{[]string{"config", "context", "order"}},
		{[]string{"config", "task", "remove", "review"}},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetIn(strings.NewReader(""))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Errorf("expected error for removed command %v, got nil", tc.args)
			}
		})
	}
}

func TestConfigInfo_ZeroMatch(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	for _, name := range []string{"edit", "remove", "info"} {
		t.Run("config "+name+" not found", func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			args := []string{"config", name, "name-that-doesnt-exist"}
			if name == "remove" {
				args = append(args, "--yes")
			}
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error for not-found item")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' in error, got: %v", err)
			}
		})
	}
}
