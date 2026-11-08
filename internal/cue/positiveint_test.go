package cue

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestPositiveInt(t *testing.T) {
	t.Parallel()
	val := cuecontext.New().CompileString(`{
		set: 2
		zero: 0
		negative: -1
		fraction: 1.5
		text: "2"
	}`)
	if err := val.Err(); err != nil {
		t.Fatal(err)
	}

	if got, err := PositiveInt(val, "set"); err != nil || got != 2 {
		t.Fatalf("set = %d, %v; want 2, nil", got, err)
	}
	if got, err := PositiveInt(val, "missing"); err != nil || got != 0 {
		t.Fatalf("missing = %d, %v; want 0, nil", got, err)
	}
	for _, field := range []string{"zero", "negative", "fraction", "text"} {
		if _, err := PositiveInt(val, field); err == nil {
			t.Errorf("%s: want an error", field)
		}
	}
}
