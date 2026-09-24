package orchestration

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/p3bot/start/internal/fault"
)

const (
	placeholderPrompt = "{{.prompt}}"
	placeholderResume = "{{.resume}}"
)

// FragmentMode selects how table placeholders are rendered.
type FragmentMode int

const (
	// FragmentLaunch shell-quotes placeholder words and leaves literals raw.
	FragmentLaunch FragmentMode = iota
	// FragmentDisplay leaves {{.prompt}} visible and does not shell-quote.
	// {{.resume}} is replaced only inside the resume id fragment.
	FragmentDisplay
)

// ResumeMode is how --resume was supplied.
type ResumeMode int

const (
	// ResumeAbsent means the flag was omitted.
	ResumeAbsent ResumeMode = iota
	// ResumeLatest is the bare flag.
	ResumeLatest
	// ResumeID is --resume=<id>, including an empty id.
	ResumeID
)

// LaunchFlags is the caller's translation request. Unset string flags insert
// nothing. Print is the exception: false inserts the module's off words when
// that key exists.
type LaunchFlags struct {
	Permission    string
	PermissionSet bool
	Effort        string
	EffortSet     bool
	Output        string
	OutputSet     bool
	Print         bool
	Resume        ResumeMode
	ResumeID      string
}

// FlagFragments are the five raw template slots. Each supplied flag is one
// leading space plus its words; zero words is an empty string.
type FlagFragments struct {
	Permission string
	Effort     string
	Output     string
	Resume     string
	Print      string
}

// WordMap is one flags map (permission, effort, or output). Keys keeps source order.
type WordMap struct {
	Keys  []string
	Words map[string][]string
}

// ListWords is a print or resume side. Present distinguishes a missing side
// from an empty list, which accepts the value and inserts nothing.
type ListWords struct {
	Present bool
	Words   []string
}

// FlagTable is the optional flags struct on an agent module.
// A nil FlagTable means the module has no flags field.
// StructErr is set when flags is present but not a struct, so print cannot be known.
// A per-flag error leaves the other flags usable.
type FlagTable struct {
	StructErr     error
	Permission    *WordMap
	PermissionErr error
	Effort        *WordMap
	EffortErr     error
	Output        *WordMap
	OutputErr     error
	PrintOff      ListWords
	PrintOn       ListWords
	HasPrint      bool
	PrintErr      error
	ResumeLatest  ListWords
	ResumeID      ListWords
	HasResume     bool
	ResumeErr     error
}

// HasEntries reports whether describe has any accepted-value rows to list.
func (t *FlagTable) HasEntries() bool {
	if t == nil || t.StructErr != nil {
		return false
	}
	return t.Permission != nil || t.Effort != nil || t.Output != nil ||
		(t.HasPrint && t.PrintErr == nil) || (t.HasResume && t.ResumeErr == nil)
}

// CommandFragments translates launch into the five raw slots.
// Permission, effort, output, and resume insert an empty slot when that flag
// was not passed. Print inserts the off words when the flag was not passed
// and that list decoded. A print entry with no off list is a usage error in
// that same case.
// A flags value that is not a struct, or a print entry that does not decode,
// is a usage error even when no flag was passed.
// A permission, effort, output, or resume entry that does not decode, a
// missing flag key, or an unlisted value is a usage error only when that
// flag was passed.
func (a Agent) CommandFragments(launch LaunchFlags, prompt string, mode FragmentMode) (FlagFragments, error) {
	if a.Flags != nil && a.Flags.StructErr != nil {
		return FlagFragments{}, flagTableError(a.Name, a.Flags.StructErr)
	}
	var out FlagFragments
	var err error
	out.Permission, err = a.mappedFragment("permission", a.permMap(), launch.PermissionSet, launch.Permission, prompt, launch, mode)
	if err != nil {
		return FlagFragments{}, err
	}
	out.Effort, err = a.mappedFragment("effort", a.effortMap(), launch.EffortSet, launch.Effort, prompt, launch, mode)
	if err != nil {
		return FlagFragments{}, err
	}
	out.Output, err = a.mappedFragment("output", a.outputMap(), launch.OutputSet, launch.Output, prompt, launch, mode)
	if err != nil {
		return FlagFragments{}, err
	}
	out.Print, err = a.printFragment(launch, prompt, mode)
	if err != nil {
		return FlagFragments{}, err
	}
	out.Resume, err = a.resumeFragment(launch, prompt, mode)
	if err != nil {
		return FlagFragments{}, err
	}
	return out, nil
}

func (a Agent) permMap() *WordMap {
	if a.Flags == nil {
		return nil
	}
	return a.Flags.Permission
}

func (a Agent) effortMap() *WordMap {
	if a.Flags == nil {
		return nil
	}
	return a.Flags.Effort
}

func (a Agent) outputMap() *WordMap {
	if a.Flags == nil {
		return nil
	}
	return a.Flags.Output
}

func (a Agent) flagFieldErr(flag string) error {
	if a.Flags == nil {
		return nil
	}
	switch flag {
	case "permission":
		return a.Flags.PermissionErr
	case "effort":
		return a.Flags.EffortErr
	case "output":
		return a.Flags.OutputErr
	default:
		return nil
	}
}

func (a Agent) mappedFragment(flag string, m *WordMap, set bool, value, prompt string, launch LaunchFlags, mode FragmentMode) (string, error) {
	if !set {
		return "", nil
	}
	if err := a.flagFieldErr(flag); err != nil {
		return "", flagTableError(a.Name, err)
	}
	if m == nil {
		return "", flagRejected(a.Name, flag)
	}
	words, ok := m.Words[value]
	if !ok {
		return "", flagValueRejected(a.Name, flag, value, m.Keys)
	}
	return assembleFlagWords(a.Name, flag, words, prompt, launch, mode, false)
}

func (a Agent) printFragment(launch LaunchFlags, prompt string, mode FragmentMode) (string, error) {
	if a.Flags == nil || !a.Flags.HasPrint {
		if launch.Print {
			return "", flagRejected(a.Name, "print")
		}
		return "", nil
	}
	if a.Flags.PrintErr != nil {
		return "", flagTableError(a.Name, a.Flags.PrintErr)
	}
	if launch.Print {
		if !a.Flags.PrintOn.Present {
			return "", flagSideMissing(a.Name, "print", "on")
		}
		return assembleFlagWords(a.Name, "print", a.Flags.PrintOn.Words, prompt, launch, mode, false)
	}
	if !a.Flags.PrintOff.Present {
		return "", flagSideMissing(a.Name, "print", "off")
	}
	return assembleFlagWords(a.Name, "print", a.Flags.PrintOff.Words, prompt, launch, mode, false)
}

func (a Agent) resumeFragment(launch LaunchFlags, prompt string, mode FragmentMode) (string, error) {
	if launch.Resume == ResumeAbsent {
		return "", nil
	}
	if a.Flags != nil && a.Flags.ResumeErr != nil {
		return "", flagTableError(a.Name, a.Flags.ResumeErr)
	}
	if a.Flags == nil || !a.Flags.HasResume {
		return "", flagRejected(a.Name, "resume")
	}
	switch launch.Resume {
	case ResumeLatest:
		if !a.Flags.ResumeLatest.Present {
			return "", flagSideMissing(a.Name, "resume", "latest")
		}
		return assembleFlagWords(a.Name, "resume", a.Flags.ResumeLatest.Words, prompt, launch, mode, false)
	case ResumeID:
		if !a.Flags.ResumeID.Present {
			return "", flagSideMissing(a.Name, "resume", "id")
		}
		return assembleFlagWords(a.Name, "resume", a.Flags.ResumeID.Words, prompt, launch, mode, true)
	default:
		return "", nil
	}
}

func assembleFlagWords(agent, flag string, words []string, prompt string, launch LaunchFlags, mode FragmentMode, showResume bool) (string, error) {
	if len(words) == 0 {
		return "", nil
	}
	parts := make([]string, len(words))
	for i, w := range words {
		switch w {
		case placeholderPrompt:
			parts[i] = renderPlaceholder(placeholderPrompt, prompt, launch, mode, showResume)
		case placeholderResume:
			parts[i] = renderPlaceholder(placeholderResume, prompt, launch, mode, showResume)
		default:
			if strings.Contains(w, "{{") || strings.Contains(w, "}}") {
				return "", fault.Usage(fmt.Errorf("agent %q --%s: flag word %q must be a literal, {{.prompt}}, or {{.resume}}", agent, flag, w))
			}
			parts[i] = w
		}
	}
	return " " + strings.Join(parts, " "), nil
}

func renderPlaceholder(ph, prompt string, launch LaunchFlags, mode FragmentMode, showResume bool) string {
	if mode == FragmentDisplay {
		if ph == placeholderResume && showResume {
			if launch.ResumeID == "" {
				return "''"
			}
			return launch.ResumeID
		}
		return ph
	}
	switch ph {
	case placeholderPrompt:
		return shellWord(prompt)
	default:
		if launch.Resume == ResumeID {
			return shellWord(launch.ResumeID)
		}
		return shellWord("")
	}
}

// shellWord quotes one flag-table placeholder as a shell argument.
// An empty value stays '' so the argument is present. escapeForShell leaves
// an empty string unquoted so optional command slots such as {{.model}} stay
// false under {{if}}.
func shellWord(s string) string {
	if s == "" {
		return "''"
	}
	return escapeForShell(s)
}

func flagRejected(agent, flag string) error {
	return fault.Usage(fmt.Errorf("agent %q does not accept --%s", agent, flag))
}

func flagValueRejected(agent, flag, value string, keys []string) error {
	return fault.Usage(fmt.Errorf("agent %q does not accept --%s %q\n\nAccepted values: %s", agent, flag, value, strings.Join(keys, ", ")))
}

func flagSideMissing(agent, flag, side string) error {
	return fault.Usage(fmt.Errorf("agent %q flags.%s is missing %s", agent, flag, side))
}

func flagTableError(agent string, err error) error {
	return fault.Usage(fmt.Errorf("agent %q flags: %s", agent, err.Error()))
}

func decodeAgentFlagTable(agentVal cue.Value) *FlagTable {
	f := agentVal.LookupPath(cue.ParsePath("flags"))
	if !f.Exists() {
		return nil
	}
	table := &FlagTable{}
	if _, err := f.Fields(); err != nil {
		table.StructErr = fmt.Errorf("must be a struct")
		return table
	}
	decodeFlagFields(f, table)
	return table
}

func decodeFlagFields(f cue.Value, table *FlagTable) {
	var err error
	if table.Permission, err = decodeOptionalWordMap(f, "permission"); err != nil {
		table.Permission = nil
		table.PermissionErr = err
	}
	if table.Effort, err = decodeOptionalWordMap(f, "effort"); err != nil {
		table.Effort = nil
		table.EffortErr = err
	}
	if table.Output, err = decodeOptionalWordMap(f, "output"); err != nil {
		table.Output = nil
		table.OutputErr = err
	}
	printVal := f.LookupPath(cue.ParsePath("print"))
	if printVal.Exists() {
		table.HasPrint = true
		table.PrintOff, table.PrintOn, table.PrintErr = decodeSwitch(printVal, "print", "off", "on")
	}
	resumeVal := f.LookupPath(cue.ParsePath("resume"))
	if resumeVal.Exists() {
		table.HasResume = true
		table.ResumeLatest, table.ResumeID, table.ResumeErr = decodeSwitch(resumeVal, "resume", "latest", "id")
	}
}

func decodeSwitch(v cue.Value, name, left, right string) (ListWords, ListWords, error) {
	if _, err := v.Fields(); err != nil {
		return ListWords{}, ListWords{}, fmt.Errorf("%s: must be a struct", name)
	}
	a, err := decodeOptionalList(v, left)
	if err != nil {
		return ListWords{}, ListWords{}, fmt.Errorf("%s: %w", name, err)
	}
	b, err := decodeOptionalList(v, right)
	if err != nil {
		return ListWords{}, ListWords{}, fmt.Errorf("%s: %w", name, err)
	}
	return a, b, nil
}

func decodeOptionalWordMap(parent cue.Value, field string) (*WordMap, error) {
	v := parent.LookupPath(cue.ParsePath(field))
	if !v.Exists() {
		return nil, nil
	}
	wm, err := decodeWordMap(v)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	return wm, nil
}

func decodeWordMap(v cue.Value) (*WordMap, error) {
	iter, err := v.Fields()
	if err != nil {
		return nil, fmt.Errorf("must be a map of word lists")
	}
	wm := &WordMap{Words: map[string][]string{}}
	for iter.Next() {
		key := iter.Selector().Unquoted()
		words, err := decodeWordList(iter.Value())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		wm.Keys = append(wm.Keys, key)
		wm.Words[key] = words
	}
	return wm, nil
}

func decodeOptionalList(parent cue.Value, field string) (ListWords, error) {
	v := parent.LookupPath(cue.ParsePath(field))
	if !v.Exists() {
		return ListWords{}, nil
	}
	words, err := decodeWordList(v)
	if err != nil {
		return ListWords{}, fmt.Errorf("%s: %w", field, err)
	}
	return ListWords{Present: true, Words: words}, nil
}

func decodeWordList(v cue.Value) ([]string, error) {
	iter, err := v.List()
	if err != nil {
		return nil, fmt.Errorf("must be a list of strings")
	}
	var words []string
	for iter.Next() {
		s, err := iter.Value().String()
		if err != nil {
			return nil, fmt.Errorf("must be a list of strings")
		}
		words = append(words, s)
	}
	if words == nil {
		words = []string{}
	}
	return words, nil
}
