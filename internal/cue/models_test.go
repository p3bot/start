package cue

import (
	"reflect"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestAgentModels(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	tests := []struct {
		name string
		src  string
		want map[string]string
	}{
		{
			name: "simple string form",
			src:  `models: {sonnet: "claude-sonnet", opus: "claude-opus"}`,
			want: map[string]string{"sonnet": "claude-sonnet", "opus": "claude-opus"},
		},
		{
			name: "object id form",
			src:  `models: {sonnet: {id: "claude-sonnet"}}`,
			want: map[string]string{"sonnet": "claude-sonnet"},
		},
		{
			name: "mixed simple and object forms",
			src:  `models: {sonnet: "claude-sonnet", opus: {id: "claude-opus"}}`,
			want: map[string]string{"sonnet": "claude-sonnet", "opus": "claude-opus"},
		},
		{
			name: "malformed entries skipped",
			src:  `models: {good: "claude-sonnet", noID: {name: "x"}, notString: 42}`,
			want: map[string]string{"good": "claude-sonnet"},
		},
		{
			name: "no models field yields nil",
			src:  `bin: "claude"`,
			want: nil,
		},
		{
			name: "empty models map yields non-nil empty",
			src:  `models: {}`,
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ctx.CompileString(tt.src)
			if err := v.Err(); err != nil {
				t.Fatalf("CompileString(%q) error = %v", tt.src, err)
			}

			got := AgentModels(v)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AgentModels() = %#v, want %#v", got, tt.want)
			}
			// nil and empty map are distinct contracts; DeepEqual above
			// distinguishes them, but assert explicitly for clarity.
			if (got == nil) != (tt.want == nil) {
				t.Errorf("AgentModels() nil = %v, want nil = %v", got == nil, tt.want == nil)
			}
		})
	}
}
