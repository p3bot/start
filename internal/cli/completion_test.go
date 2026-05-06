package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompletionBash(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"completion", "bash"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "bash completion") {
		t.Error("expected bash completion script header")
	}
	if !strings.Contains(output, "__start_") {
		t.Error("expected start-specific completion functions")
	}
}

func TestCompletionZsh(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"completion", "zsh"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("completion zsh failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "#compdef start") {
		t.Error("expected zsh compdef header")
	}
	if !strings.Contains(output, "_start") {
		t.Error("expected _start completion function")
	}
}

func TestCompletionFish(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"completion", "fish"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("completion fish failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "fish completion") {
		t.Error("expected fish completion script header")
	}
	if !strings.Contains(output, "__start_") {
		t.Error("expected start-specific completion functions")
	}
}

func TestCompletionHelp(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"completion", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("completion --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "bash") {
		t.Error("expected bash in help output")
	}
	if !strings.Contains(output, "zsh") {
		t.Error("expected zsh in help output")
	}
	if !strings.Contains(output, "fish") {
		t.Error("expected fish in help output")
	}
}

func TestCompletionNoArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{"bash with args", []string{"completion", "bash", "extra"}},
		{"zsh with args", []string{"completion", "zsh", "extra"}},
		{"fish with args", []string{"completion", "fish", "extra"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Error("expected error for extra arguments")
			}
		})
	}
}
