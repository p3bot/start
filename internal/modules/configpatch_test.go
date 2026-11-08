package modules

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue/ast"

	"github.com/p3bot/start/internal/fault"
)

func patchEntry(fields ...string) *ast.StructLit {
	s := &ast.StructLit{}
	for i := 0; i+1 < len(fields); i += 2 {
		s.Elts = append(s.Elts, &ast.Field{Label: ast.NewIdent(fields[i]), Value: ast.NewString(fields[i+1])})
	}
	return s
}

func TestPatchConfigModule(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, stored string
		content      *ast.StructLit
		want         string
	}{
		{
			name: "keeps unmanaged fields and in-entry comments",
			stored: `tasks: {
	good: {
		// why this task needs bash
		shell:   "bash"
		prompt:  "old"
		agent:   "claude"
		timeout: 30
		extra:   true
	}
}
`,
			content: patchEntry("description", "new", "prompt", "body"),
			want: `tasks: good: {
	description: "new"
	// why this task needs bash
	shell:   "bash"
	prompt:  "body"
	agent:   "claude"
	timeout: 30
	extra:   true
}
`,
		},
		{
			name: "removes a cleared managed field",
			stored: `tasks: {
	good: {
		description: "gone"
		prompt:      "body"
		agent:       "claude"
	}
}
`,
			content: patchEntry("prompt", "body"),
			want: `tasks: good: {
	prompt: "body"
	agent:  "claude"
}
`,
		},
		{
			name: "inserts after the nearest earlier managed field",
			stored: `tasks: {
	good: {
		origin: "example.com/x@v1"
		agent:  "claude"
		prompt: "body"
	}
}
`,
			content: patchEntry("origin", "example.com/x@v1", "role", "golang", "prompt", "body"),
			want: `tasks: good: {
	origin: "example.com/x@v1"
	role:   "golang"
	agent:  "claude"
	prompt: "body"
}
`,
		},
		{
			name:    "makes an optional or required managed field regular",
			stored:  "tasks: good: {\n\tdescription?: string\n\tprompt!:      \"old\"\n}\n",
			content: patchEntry("description", "new", "prompt", "body"),
			want:    "tasks: good: {\n\tdescription: \"new\"\n\tprompt:      \"body\"\n}\n",
		},
		{
			name:    "patches a short-form entry",
			stored:  "tasks: good: timeout: 30\n",
			content: patchEntry("prompt", "body"),
			want:    "tasks: good: {\n\tprompt:  \"body\"\n\ttimeout: 30\n}\n",
		},
		{
			name:    "adds an absent entry beside siblings",
			stored:  "tasks: {\n\tother: prompt: \"x\"\n}\n",
			content: patchEntry("prompt", "body"),
			want:    "tasks: {\n\tother: prompt: \"x\"\n\tgood: prompt:  \"body\"\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "tasks.cue")
			if err := os.WriteFile(path, []byte(tt.stored), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := PatchConfigModule(path, "tasks", "good", tt.content); err != nil {
				t.Fatalf("PatchConfigModule: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestPatchConfigModuleRefusesUnsafePatch(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, stored string }{
		{"non-struct", "#Base: {agent: \"claude\"}\ntasks: good: #Base & {prompt: \"x\"}\n"},
		{"embedding", "#Base: {description: \"old\"}\ntasks: good: {\n\t#Base\n\tprompt: \"x\"\n}\n"},
		{"comprehension", "on: true\ntasks: good: {\n\tif on {description: \"old\"}\n\tprompt: \"x\"\n}\n"},
		{"split declaration", "tasks: good: prompt: \"x\"\ntasks: good: description: \"old\"\n"},
		{"entry let", "tasks: good: {\n\tlet p = \"x\"\n\tprompt: p\n}\n"},
		{"file let", "let shared = \"x\"\ntasks: good: prompt: shared\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "tasks.cue")
			if err := os.WriteFile(path, []byte(tt.stored), 0o644); err != nil {
				t.Fatal(err)
			}
			err := PatchConfigModule(path, "tasks", "good", patchEntry("description", "new", "prompt", "x"))
			if !errors.Is(err, fault.ErrUserConfig) {
				t.Fatalf("err = %v, want a user-config fault", err)
			}
			got, _ := os.ReadFile(path)
			if string(got) != tt.stored {
				t.Fatalf("refused patch changed the file:\n%s", got)
			}
		})
	}
}

func TestPatchConfigModuleBuildsWithSiblingFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "roles.cue"), []byte("roles: r: prompt: \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tasks.cue")
	if err := PatchConfigModule(path, "tasks", "good", patchEntry("prompt", "body")); err != nil {
		t.Fatalf("PatchConfigModule into a new file beside a sibling: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.cue"), []byte("tasks: good: description: \"elsewhere\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	err := PatchConfigModule(path, "tasks", "good", patchEntry("description", "new", "prompt", "body"))
	if !errors.Is(err, fault.ErrUserConfig) {
		t.Fatalf("conflict with another file: err = %v, want a user-config fault", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatalf("refused patch changed the file:\n%s", after)
	}
}
