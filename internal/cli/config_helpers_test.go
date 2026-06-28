package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestLookupInstalledName(t *testing.T) {
	items := map[string]string{
		"cwd/dotai/create-role": "Create a role",
		"confluence/read-doc":   "Read Confluence doc",
	}

	tests := []struct {
		name    string
		query   string
		wantKey string
		wantVal string
		wantErr string
	}{
		{
			name:    "exact match",
			query:   "cwd/dotai/create-role",
			wantKey: "cwd/dotai/create-role",
			wantVal: "Create a role",
		},
		{
			// lookupInstalledName is exact-only: its callers pass a name the shared
			// matcher already reduced, so a substring no longer resolves here.
			name:    "substring does not resolve",
			query:   "create-role",
			wantErr: "not found",
		},
		{
			name:    "no match",
			query:   "nonexistent",
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, val, err := lookupInstalledName(items, "task", tt.query)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
			if val != tt.wantVal {
				t.Errorf("val = %q, want %q", val, tt.wantVal)
			}
		})
	}
}

func TestLookupInstalledName_EmptyMap(t *testing.T) {
	items := map[string]string{}

	_, _, err := lookupInstalledName(items, "agent", "anything")
	if err == nil {
		t.Fatal("expected error for empty map")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestParseSelectionInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		count   int
		want    []int
		wantErr string
	}{
		{
			name:  "single number",
			input: "2",
			count: 5,
			want:  []int{1},
		},
		{
			name:  "csv numbers",
			input: "1,3,5",
			count: 5,
			want:  []int{0, 2, 4},
		},
		{
			name:  "range",
			input: "2-4",
			count: 5,
			want:  []int{1, 2, 3},
		},
		{
			name:  "mixed csv and range",
			input: "1,3-5",
			count: 5,
			want:  []int{0, 2, 3, 4},
		},
		{
			name:  "deduplicates",
			input: "1,2,1,2-3",
			count: 5,
			want:  []int{0, 1, 2},
		},
		{
			name:  "whitespace in parts",
			input: " 1 , 3 , 2 - 4 ",
			count: 5,
			want:  []int{0, 2, 1, 3},
		},
		{
			name:  "empty parts skipped",
			input: "1,,3,",
			count: 5,
			want:  []int{0, 2},
		},
		{
			name:  "empty input returns nil",
			input: "",
			count: 5,
			want:  nil,
		},
		{
			name:    "number exceeds count",
			input:   "6",
			count:   5,
			wantErr: "invalid selection",
		},
		{
			name:    "zero is out of range",
			input:   "0",
			count:   5,
			wantErr: "invalid selection",
		},
		{
			name:    "non-numeric input",
			input:   "abc",
			count:   5,
			wantErr: "invalid selection",
		},
		{
			name:    "reversed range",
			input:   "3-1",
			count:   5,
			wantErr: "invalid range",
		},
		{
			name:    "range exceeds count",
			input:   "3-6",
			count:   5,
			wantErr: "invalid range",
		},
		{
			name:    "range starts at zero",
			input:   "0-2",
			count:   5,
			wantErr: "invalid range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSelectionInput(tt.input, tt.count)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPromptSelectFromList(t *testing.T) {
	names := []string{"golang/review/architecture", "golang/review/code", "golang/review/security"}

	t.Run("empty input cancels", func(t *testing.T) {
		w := &bytes.Buffer{}
		selected, err := promptSelectFromList(w, strings.NewReader("\n"), "task", "golang/review", names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected != nil {
			t.Errorf("expected nil (cancelled), got %v", selected)
		}
	})

	t.Run("all selects all", func(t *testing.T) {
		w := &bytes.Buffer{}
		selected, err := promptSelectFromList(w, strings.NewReader("all\n"), "task", "golang/review", names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(selected) != 3 {
			t.Errorf("got %v, want all 3", selected)
		}
	})

	t.Run("comma-separated numbers", func(t *testing.T) {
		w := &bytes.Buffer{}
		selected, err := promptSelectFromList(w, strings.NewReader("1,3\n"), "task", "golang/review", names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(selected) != 2 {
			t.Fatalf("got %v, want 2 entries", selected)
		}
		if selected[0] != "golang/review/architecture" || selected[1] != "golang/review/security" {
			t.Errorf("got %v", selected)
		}
	})

	t.Run("range selection", func(t *testing.T) {
		w := &bytes.Buffer{}
		selected, err := promptSelectFromList(w, strings.NewReader("1-2\n"), "task", "golang/review", names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(selected) != 2 {
			t.Fatalf("got %v, want 2 entries", selected)
		}
		if selected[0] != "golang/review/architecture" || selected[1] != "golang/review/code" {
			t.Errorf("got %v", selected)
		}
	})

	t.Run("invalid number returns error", func(t *testing.T) {
		w := &bytes.Buffer{}
		_, err := promptSelectFromList(w, strings.NewReader("99\n"), "task", "golang/review", names)
		if err == nil {
			t.Fatal("expected error for out-of-range number")
		}
	})

	t.Run("reversed range returns error", func(t *testing.T) {
		w := &bytes.Buffer{}
		_, err := promptSelectFromList(w, strings.NewReader("2-1\n"), "task", "golang/review", names)
		if err == nil {
			t.Fatal("expected error for reversed range")
		}
	})
}

func TestPromptSelectCategory(t *testing.T) {
	categories := []string{"agents", "roles", "contexts", "tasks"}

	tests := []struct {
		name       string
		input      string
		wantResult string
		wantErr    string
	}{
		{
			name:       "first item",
			input:      "1\n",
			wantResult: "agents",
		},
		{
			name:       "middle item",
			input:      "2\n",
			wantResult: "roles",
		},
		{
			name:       "last item",
			input:      "4\n",
			wantResult: "tasks",
		},
		{
			name:       "empty input cancels",
			input:      "\n",
			wantResult: "",
		},
		{
			name:    "zero is out of range",
			input:   "0\n",
			wantErr: "invalid selection",
		},
		{
			name:    "exceeds list length",
			input:   "5\n",
			wantErr: "invalid selection",
		},
		{
			name:    "non-numeric input errors",
			input:   "agents\n",
			wantErr: "invalid selection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &bytes.Buffer{}
			got, err := promptSelectCategory(w, strings.NewReader(tt.input), categories)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error %q, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantResult {
				t.Errorf("got %q, want %q", got, tt.wantResult)
			}
		})
	}
}

func TestPromptSelectOneFromList(t *testing.T) {
	names := []string{"alpha", "beta", "gamma"}

	t.Run("empty list returns empty string", func(t *testing.T) {
		w := &bytes.Buffer{}
		got, err := promptSelectOneFromList(w, strings.NewReader(""), "item", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	tests := []struct {
		name       string
		input      string
		wantResult string
		wantErr    string
	}{
		{
			name:       "first item",
			input:      "1\n",
			wantResult: "alpha",
		},
		{
			name:       "middle item",
			input:      "2\n",
			wantResult: "beta",
		},
		{
			name:       "last item",
			input:      "3\n",
			wantResult: "gamma",
		},
		{
			name:       "empty input cancels",
			input:      "\n",
			wantResult: "",
		},
		{
			name:    "zero is out of range",
			input:   "0\n",
			wantErr: "invalid selection",
		},
		{
			name:    "exceeds list length",
			input:   "4\n",
			wantErr: "invalid selection",
		},
		{
			name:    "non-numeric input errors",
			input:   "beta\n",
			wantErr: "invalid selection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &bytes.Buffer{}
			got, err := promptSelectOneFromList(w, strings.NewReader(tt.input), "item", names)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error %q, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantResult {
				t.Errorf("got %q, want %q", got, tt.wantResult)
			}
		})
	}
}

func TestPromptSelectFromList_EmptyQuery(t *testing.T) {
	names := []string{"alpha", "beta"}

	t.Run("empty query shows plain header", func(t *testing.T) {
		w := &bytes.Buffer{}
		selected, err := promptSelectFromList(w, strings.NewReader("1\n"), "item", "", names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(selected) != 1 || selected[0] != "alpha" {
			t.Errorf("got %v, want [alpha]", selected)
		}
		if strings.Contains(w.String(), "matching") {
			t.Errorf("output should not contain 'matching' for empty query: %s", w.String())
		}
	})

	t.Run("non-empty query shows matching header", func(t *testing.T) {
		w := &bytes.Buffer{}
		_, err := promptSelectFromList(w, strings.NewReader("\n"), "item", "al", names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(w.String(), "matching") {
			t.Errorf("output should contain 'matching' for non-empty query: %s", w.String())
		}
	})
}
