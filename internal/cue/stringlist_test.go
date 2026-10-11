package cue

import (
	"reflect"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestStringList(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()
	val := ctx.CompileString(`{
		session_locator: ["session_id", "0"]
		empty: []
		bad: { id: "x" }
		mixed: ["session_id", 1]
	}`)
	if err := val.Err(); err != nil {
		t.Fatal(err)
	}

	got, err := StringList(val, "session_locator")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"session_id", "0"}) {
		t.Fatalf("locator = %#v", got)
	}

	absent, err := StringList(val, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if absent != nil {
		t.Fatalf("absent = %#v, want nil", absent)
	}

	empty, err := StringList(val, "empty")
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty = %#v, want non-nil empty", empty)
	}

	_, err = StringList(val, "bad")
	if err == nil || !strings.Contains(err.Error(), "as list") {
		t.Fatalf("struct error = %v", err)
	}
	_, err = StringList(val, "mixed")
	if err == nil || !strings.Contains(err.Error(), "mixed.1") || !strings.Contains(err.Error(), "as string") {
		t.Fatalf("mixed error = %v", err)
	}
}
