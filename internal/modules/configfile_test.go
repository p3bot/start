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
	if !CategoryHasField("tasks", "min_members") {
		t.Error("tasks should have min_members")
	}
	if CategoryHasField("roles", "min_members") || CategoryHasField("agents", "min_members") {
		t.Error("min_members is a task field only")
	}
	if CategoryHasField("nope", "command") {
		t.Fatal("unknown category should have no fields")
	}
}

func TestCategoryFieldOrderWritesTaskMinMembers(t *testing.T) {
	t.Parallel()
	if !fieldName("min_members", CategoryFieldOrder("tasks")...) {
		t.Fatalf("tasks order %v omits min_members", CategoryFieldOrder("tasks"))
	}
	if fieldName("min_members", CategoryFieldOrder("roles")...) {
		t.Fatal("roles order should not write min_members")
	}
}
