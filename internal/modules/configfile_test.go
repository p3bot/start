package modules

import "testing"

func TestCategoryHasField(t *testing.T) {
	t.Parallel()
	if !CategoryHasField("agents", "session_locator") {
		t.Fatal("agents should have session_locator")
	}
	if CategoryHasField("roles", "session_locator") || CategoryHasField("agents", "file") {
		t.Fatal("session_locator is an agent field, file is not")
	}
	for _, tc := range []struct{ category, name string }{
		{"roles", "file"},
		{"roles", "shell"},
		{"contexts", "timeout"},
		{"tasks", "role"},
		{"tasks", "agent"},
		{"skills", "file"},
		{"agents", "origin"},
	} {
		if !CategoryHasField(tc.category, tc.name) {
			t.Errorf("%s should have %s", tc.category, tc.name)
		}
	}
	if CategoryHasField("nope", "command") {
		t.Fatal("unknown category should have no fields")
	}
}
