package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/start-cli/start/internal/config"
)

// testSchemaSet creates a SchemaSet from inline CUE for testing.
// Matches the production schemas in start-assets/schemas/.
func testSchemaSet(t *testing.T) SchemaSet {
	t.Helper()
	cctx := cuecontext.New()

	schemaSource := `
#UTD: {
	file?:    string
	command?: string
	prompt?:  string
	shell?:   string & !=""
	timeout?: int & >=1 & <=3600
}

#Base: {
	description?: string
	tags?: [...string & =~"^[a-z0-9]+(-[a-z0-9]+)*$"]
	origin?: string
}

#Agent: {
	#Base
	command: string & !=""
	bin?: string & !=""
	default_model?: string
	models?: [string]: string & !=""
}

#Role: {
	#Base
	#UTD
	optional: bool | *false
}

#Context: {
	#Base
	#UTD
	required?: bool
	default?:  bool
}

#Task: {
	#Base
	#UTD
	role?:  string
	agent?: string
}

#Settings: {
	default_agent?: string & !=""
	shell?: string & !=""
	timeout?: int & >0
	assets_index?: string & !=""
}
`

	v := cctx.CompileString(schemaSource)
	if v.Err() != nil {
		t.Fatalf("compiling test schemas: %v", v.Err())
	}

	return SchemaSet{
		Agent:    v.LookupPath(cue.ParsePath("#Agent")),
		Role:     v.LookupPath(cue.ParsePath("#Role")),
		Context:  v.LookupPath(cue.ParsePath("#Context")),
		Task:     v.LookupPath(cue.ParsePath("#Task")),
		Settings: v.LookupPath(cue.ParsePath("#Settings")),
	}
}

// writeConfigFile creates a CUE config file in a directory.
func writeConfigFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckSchemaValidation_ValidAgent(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "agents.cue", `
agents: "claude": {
	command: "claude --model {{.model}} {{.prompt}}"
	bin: "claude"
	description: "Claude by Anthropic"
}
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	if section.Name != "Schema Validation" {
		t.Errorf("Name = %q, want %q", section.Name, "Schema Validation")
	}
	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
	if section.Results[0].Label != "agents.cue" {
		t.Errorf("label = %q, want %q", section.Results[0].Label, "agents.cue")
	}
}

func TestCheckSchemaValidation_EmptyCommand(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "agents.cue", `
agents: "bad": {
	command: ""
}
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	if len(section.Results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	found := false
	for _, r := range section.Results {
		if r.Status == StatusWarn && strings.Contains(r.Message, "agents.bad") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected StatusWarn for agents.bad, got results: %+v", section.Results)
	}
}

func TestCheckSchemaValidation_InvalidTimeout(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "roles.cue", `
roles: "bad": {
	prompt: "test prompt"
	timeout: 9999
}
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	if len(section.Results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	found := false
	for _, r := range section.Results {
		if r.Status == StatusWarn && strings.Contains(r.Message, "roles.bad") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected StatusWarn for roles.bad, got results: %+v", section.Results)
	}
}

func TestCheckSchemaValidation_ExtraFields(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "agents.cue", `
agents: "myagent": {
	command: "test-cmd {{.prompt}}"
	custom_field: "should be allowed"
	another: 42
}
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass (extra fields should be allowed)", section.Results[0].Status)
	}
}

func TestCheckSchemaValidation_InvalidSettings(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "settings.cue", `
settings: {
	shell: ""
}
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	if len(section.Results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	found := false
	for _, r := range section.Results {
		if r.Status == StatusWarn && strings.Contains(r.Message, "settings") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected StatusWarn for settings, got results: %+v", section.Results)
	}
}

func TestCheckSchemaValidation_ValidSettings(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "settings.cue", `
settings: {
	default_agent: "claude"
	shell: "/bin/bash"
	timeout: 120
}
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
}

func TestCheckSchemaValidation_MultipleFiles(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "agents.cue", `
agents: "good": {
	command: "good-cmd"
}
`)
	writeConfigFile(t, tmpDir, "roles.cue", `
roles: "myrole": {
	prompt: "You are helpful"
}
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	if len(section.Results) != 2 {
		t.Fatalf("expected 2 results (one per file), got %d", len(section.Results))
	}
	for _, r := range section.Results {
		if r.Status != StatusPass {
			t.Errorf("file %q: status = %v, want StatusPass", r.Label, r.Status)
		}
	}
}

func TestCheckSchemaValidation_GlobalAndLocal(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global")
	localDir := filepath.Join(tmpDir, "local")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeConfigFile(t, globalDir, "agents.cue", `
agents: "claude": {
	command: "claude {{.prompt}}"
}
`)
	writeConfigFile(t, localDir, "roles.cue", `
roles: "reviewer": {
	prompt: "Review code"
}
`)

	paths := config.Paths{
		Global:       globalDir,
		GlobalExists: true,
		Local:        localDir,
		LocalExists:  true,
	}

	section := CheckSchemaValidation(paths, schemas)

	if len(section.Results) != 2 {
		t.Fatalf("expected 2 results (one per dir), got %d", len(section.Results))
	}
	for _, r := range section.Results {
		if r.Status != StatusPass {
			t.Errorf("file %q: status = %v, want StatusPass", r.Label, r.Status)
		}
	}
}

func TestCheckSchemaValidation_NoConfigDirs(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	paths := config.Paths{
		Global:       filepath.Join(tmpDir, "global"),
		GlobalExists: false,
		Local:        filepath.Join(tmpDir, "local"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusInfo {
		t.Errorf("status = %v, want StatusInfo", section.Results[0].Status)
	}
	if section.Results[0].Label != "No config files to validate" {
		t.Errorf("label = %q, want %q", section.Results[0].Label, "No config files to validate")
	}
}

func TestCheckSchemaValidation_NoRecognisedKeys(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "custom.cue", `
something_else: "not a recognised key"
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	// File with no recognised keys should be silently skipped
	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result (info), got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusInfo {
		t.Errorf("status = %v, want StatusInfo", section.Results[0].Status)
	}
}

func TestCheckSchemaValidation_SyntaxErrorSkipped(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "bad.cue", `this is not valid cue {{{`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	// Syntax errors should be silently skipped (caught by Configuration section)
	for _, r := range section.Results {
		if r.Status == StatusWarn || r.Status == StatusFail {
			t.Errorf("syntax error file should be skipped, got status %v: %s", r.Status, r.Message)
		}
	}
}

func TestCheckSchemaValidation_BadTagFormat(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	writeConfigFile(t, tmpDir, "roles.cue", `
roles: "bad": {
	prompt: "test"
	tags: ["INVALID_TAG"]
}
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	found := false
	for _, r := range section.Results {
		if r.Status == StatusWarn && strings.Contains(r.Message, "roles.bad") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected StatusWarn for bad tag format, got results: %+v", section.Results)
	}
}

func TestCheckSchemaValidation_MixedValidAndInvalid(t *testing.T) {
	t.Parallel()
	schemas := testSchemaSet(t)
	tmpDir := t.TempDir()

	// One file with valid and invalid entries
	writeConfigFile(t, tmpDir, "agents.cue", `
agents: {
	"good": {
		command: "good-cmd"
	}
	"bad": {
		command: ""
	}
}
`)

	paths := config.Paths{
		Global:       tmpDir,
		GlobalExists: true,
		Local:        filepath.Join(tmpDir, "nonexistent"),
		LocalExists:  false,
	}

	section := CheckSchemaValidation(paths, schemas)

	// Should have a warning for the bad agent (no pass since file has errors)
	hasWarn := false
	for _, r := range section.Results {
		if r.Status == StatusWarn && strings.Contains(r.Message, "agents.bad") {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected StatusWarn for agents.bad, got results: %+v", section.Results)
	}
}
