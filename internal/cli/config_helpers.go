package cli

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/tui"
)

// loadForScope loads entities from the appropriate scope using a generic merge strategy.
// Returns the entity map, names in definition order, and any error.
// Order: global entries first (in definition order), then local entries (in definition order).
// Local entries override global entries with the same name but retain their global position.
func loadForScope[T any](
	localOnly bool,
	loadFromDir func(string) (map[string]T, []string, error),
	setSource func(*T, string),
) (map[string]T, []string, error) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return nil, nil, fmt.Errorf("resolving config paths: %w", err)
	}

	items := make(map[string]T)
	var order []string
	seen := make(map[string]bool)

	if localOnly {
		if paths.LocalExists {
			localItems, localOrder, err := loadFromDir(paths.Local)
			if err != nil && !os.IsNotExist(err) {
				return nil, nil, err
			}
			for _, name := range localOrder {
				item := localItems[name]
				setSource(&item, "local")
				items[name] = item
				order = append(order, name)
			}
		}
	} else {
		if paths.GlobalExists {
			globalItems, globalOrder, err := loadFromDir(paths.Global)
			if err != nil && !os.IsNotExist(err) {
				return nil, nil, err
			}
			for _, name := range globalOrder {
				item := globalItems[name]
				setSource(&item, "global")
				items[name] = item
				order = append(order, name)
				seen[name] = true
			}
		}
		if paths.LocalExists {
			localItems, localOrder, err := loadFromDir(paths.Local)
			if err != nil && !os.IsNotExist(err) {
				return nil, nil, err
			}
			for _, name := range localOrder {
				item := localItems[name]
				setSource(&item, "local")
				items[name] = item
				// Only add to order if not already present from global
				if !seen[name] {
					order = append(order, name)
				}
			}
		}
	}

	return items, order, nil
}

// promptString prompts for a string value with a default.
func promptString(w io.Writer, r io.Reader, label, defaultVal string) (string, error) {
	// Print label with cyan () delimiters for "(optional)"
	if base, found := strings.CutSuffix(label, " (optional)"); found {
		_, _ = fmt.Fprint(w, base)
		_, _ = fmt.Fprintf(w, " %s", tui.Annotate("optional"))
	} else {
		_, _ = fmt.Fprint(w, label)
	}
	if defaultVal != "" {
		_, _ = fmt.Fprintf(w, " %s", tui.Bracket("%s", defaultVal))
	}
	_, _ = fmt.Fprint(w, ": ")

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal, nil
	}
	return input, nil
}

// promptContentSource prompts the user to choose a content source (file, command, or inline prompt).
// defaultChoice is the default menu option ("1" for file, "3" for inline prompt).
// currentPrompt is passed to promptText as the default value for option 3.
// Returns the selected file, command, and prompt values (only one will be non-empty).
func promptContentSource(w io.Writer, r io.Reader, defaultChoice, currentPrompt string) (file, command, prompt string, err error) {
	_, _ = fmt.Fprintf(w, "\nContent source %s:\n", tui.Annotate("choose one"))
	_, _ = fmt.Fprintln(w, "  1. File path")
	_, _ = fmt.Fprintln(w, "  2. Command")
	_, _ = fmt.Fprintln(w, "  3. Inline prompt")
	_, _ = fmt.Fprintf(w, "Choice %s: ", tui.Bracket("%s", defaultChoice))

	reader := bufio.NewReader(r)
	choice, err := reader.ReadString('\n')
	if err != nil {
		return "", "", "", fmt.Errorf("reading input: %w", err)
	}
	choice = strings.TrimSpace(choice)
	if choice == "" {
		choice = defaultChoice
	}

	switch choice {
	case "1":
		file, err = promptString(w, r, "File path", "")
		if err != nil {
			return "", "", "", err
		}
	case "2":
		command, err = promptString(w, r, "Command", "")
		if err != nil {
			return "", "", "", err
		}
	case "3":
		prompt, err = promptText(w, r, "Prompt text", currentPrompt)
		if err != nil {
			return "", "", "", err
		}
	default:
		return "", "", "", fmt.Errorf("invalid choice: %s", choice)
	}

	return file, command, prompt, nil
}

// ansiEscapeRe matches ANSI CSI escape sequences (arrow keys, home, end, etc.).
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z~]`)

// promptText prompts for multi-line text input.
// Users can type text directly (finish with a blank line) or press Enter
// to open $EDITOR for longer input.
func promptText(w io.Writer, r io.Reader, label, defaultVal string) (string, error) {
	// Show current value if editing a multi-line default
	if defaultVal != "" && strings.Contains(defaultVal, "\n") {
		_, _ = fmt.Fprintf(w, "Current value:\n%s\n\n", defaultVal)
	}

	_, _ = fmt.Fprint(w, label)
	if defaultVal != "" && !strings.Contains(defaultVal, "\n") {
		_, _ = fmt.Fprintf(w, " %s", tui.Bracket("%s", defaultVal))
	}
	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorDim.Fprintln(w, "  Type text, then press Enter on a blank line to finish")
	_, _ = tui.ColorDim.Fprintln(w, "  Or press Enter now to open $EDITOR for full editing")
	_, _ = tui.ColorDim.Fprintln(w, "  Arrow keys are not supported in this mode")
	_, _ = tui.ColorSuccess.Fprint(w, "↪ ")

	reader := bufio.NewReader(r)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	firstLine = strings.TrimRight(firstLine, "\r\n")
	firstLine = ansiEscapeRe.ReplaceAllString(firstLine, "")

	// Empty first line: open editor
	if firstLine == "" {
		tmpFile, err := os.CreateTemp("", "start-prompt-*.md")
		if err != nil {
			return defaultVal, nil
		}
		tmpPath := tmpFile.Name()
		defer func() { _ = os.Remove(tmpPath) }()

		if defaultVal != "" {
			_, _ = tmpFile.WriteString(defaultVal)
		}
		_ = tmpFile.Close()

		if err := openInEditor(tmpPath); err != nil {
			return defaultVal, nil
		}

		content, err := os.ReadFile(tmpPath)
		if err != nil {
			return defaultVal, nil
		}

		result := strings.TrimRight(string(content), " \t\r\n")
		if result == "" {
			return defaultVal, nil
		}
		return result, nil
	}

	// User typed text: read lines until blank line
	var lines []string
	lines = append(lines, firstLine)

	for {
		_, _ = tui.ColorSuccess.Fprint(w, "↪ ")
		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF without newline - include what we have
			line = strings.TrimRight(line, "\r\n")
			line = ansiEscapeRe.ReplaceAllString(line, "")
			if line != "" {
				lines = append(lines, line)
			}
			break
		}
		line = strings.TrimRight(line, "\r\n")
		line = ansiEscapeRe.ReplaceAllString(line, "")
		if strings.TrimSpace(line) == "" {
			break
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n"), nil
}

// promptDefaultModel prompts for a default model selection.
// When models are defined, displays a numbered list for selection.
// Falls back to free-text input when no models are defined.
func promptDefaultModel(w io.Writer, r io.Reader, current string, models map[string]string) (string, error) {
	if len(models) == 0 {
		return promptString(w, r, "Default model", current)
	}

	// Sort aliases for stable ordering
	aliases := make([]string, 0, len(models))
	for alias := range models {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	_, _ = fmt.Fprintln(w, "Default model:")
	for i, alias := range aliases {
		if alias == current {
			_, _ = fmt.Fprintf(w, "  %d. %s - %s %s\n", i+1, alias, tui.ColorDim.Sprint(models[alias]), tui.Annotate("%s", tui.ColorInstalled.Sprint("current")))
		} else {
			_, _ = fmt.Fprintf(w, "  %d. %s - %s\n", i+1, alias, tui.ColorDim.Sprint(models[alias]))
		}
	}

	_, _ = fmt.Fprintln(w)
	if current != "" {
		_, _ = fmt.Fprintf(w, "Select model %s: ", tui.Annotate("number, alias, or Enter to keep %q", current))
	} else {
		_, _ = fmt.Fprintf(w, "Select model %s: ", tui.Annotate("number or alias"))
	}

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return current, nil
	}

	// Try parsing as number
	if choice, err := strconv.Atoi(input); err == nil {
		if choice >= 1 && choice <= len(aliases) {
			return aliases[choice-1], nil
		}
		return "", fmt.Errorf("invalid selection: %s (choose 1-%d)", input, len(aliases))
	}

	// Try matching by alias
	for _, alias := range aliases {
		if strings.EqualFold(alias, input) {
			return alias, nil
		}
	}

	return "", fmt.Errorf("invalid selection: %q is not a known model alias", input)
}

// promptTags prompts for editing a slice of tags.
// Shows current tags (when showCurrent is true) and allows: comma-separated input to replace, empty to clear, Enter to keep.
func promptTags(w io.Writer, r io.Reader, current []string, showCurrent bool) ([]string, error) {
	if showCurrent {
		if len(current) > 0 {
			_, _ = fmt.Fprintf(w, "Current tags: %s\n", tui.Bracket("%s", strings.Join(current, ", ")))
		} else {
			_, _ = fmt.Fprintf(w, "Current tags: %s\n", tui.Annotate("none"))
		}
	}
	if showCurrent {
		_, _ = fmt.Fprintf(w, "Tags %s: ", tui.Annotate("comma-separated, - to clear, Enter to keep"))
	} else {
		_, _ = fmt.Fprintf(w, "Tags %s: ", tui.Annotate("comma-separated, or Enter to skip"))
	}

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	input = strings.TrimSpace(input)

	// Enter keeps current
	if input == "" {
		return current, nil
	}

	// "-" clears tags
	if input == "-" {
		return nil, nil
	}

	// Parse comma-separated tags
	var tags []string
	for t := range strings.SplitSeq(input, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	return tags, nil
}

// promptModelsAdd prompts for adding model aliases in the add flow (no existing models).
// Returns the map (may be nil if user skips).
func promptModelsAdd(w io.Writer, r io.Reader) (map[string]string, error) {
	reader := bufio.NewReader(r)
	_, _ = fmt.Fprintln(w, "Add model aliases (alias=model-id, empty to finish):")
	result, err := readModelAliases(w, reader)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// promptModels prompts for editing a map of model aliases.
// Offers options: (k)eep, (c)lear, (e)dit.
func promptModels(w io.Writer, r io.Reader, current map[string]string) (map[string]string, error) {
	reader := bufio.NewReader(r)

	if len(current) > 0 {
		_, _ = fmt.Fprintln(w, "Current models:")
		var aliases []string
		for alias := range current {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			_, _ = fmt.Fprintf(w, "  %s: %s\n", alias, tui.ColorDim.Sprint(current[alias]))
		}
	} else {
		_, _ = fmt.Fprintf(w, "Current models: %s\n", tui.Annotate("none"))
	}

	_, _ = fmt.Fprintf(w, "Models: %skeep, %sclear, %sedit %s: ",
		tui.Annotate("k"), tui.Annotate("c"), tui.Annotate("e"),
		tui.Bracket("k"))
	choice, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}
	choice = strings.TrimSpace(strings.ToLower(choice))

	switch choice {
	case "", "k", "keep":
		return current, nil
	case "c", "clear":
		return promptModelsEdit(w, reader, nil)
	case "e", "edit":
		return promptModelsEdit(w, reader, current)
	default:
		return nil, fmt.Errorf("invalid choice: %s", choice)
	}
}

// promptModelsEdit handles the edit mode for models.
func promptModelsEdit(w io.Writer, reader *bufio.Reader, current map[string]string) (map[string]string, error) {
	result := make(map[string]string)

	// Edit existing models
	if len(current) > 0 {
		_, _ = fmt.Fprintln(w, "Edit existing models (Enter to keep, - to delete):")
		var aliases []string
		for alias := range current {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)

		for _, alias := range aliases {
			currentVal := current[alias]
			_, _ = fmt.Fprintf(w, "  %s %s: ", alias, tui.Bracket("%s", currentVal))

			input, err := reader.ReadString('\n')
			if err != nil {
				return nil, fmt.Errorf("reading input: %w", err)
			}
			input = strings.TrimSpace(input)

			if input == "-" {
				// Delete this model
				continue
			}
			if input == "" {
				// Keep current value
				result[alias] = currentVal
			} else {
				// Update value
				result[alias] = input
			}
		}
	}

	// Add new models
	_, _ = fmt.Fprintln(w, "Add new models (alias=model-id, empty to finish):")
	newModels, err := readModelAliases(w, reader)
	if err != nil {
		return nil, err
	}
	maps.Copy(result, newModels)

	return result, nil
}

// readModelAliases reads alias=model-id pairs from the reader until an empty line.
func readModelAliases(w io.Writer, reader *bufio.Reader) (map[string]string, error) {
	result := make(map[string]string)
	for {
		_, _ = fmt.Fprint(w, "  ")
		_, _ = tui.ColorSuccess.Fprint(w, "> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("reading input: %w", err)
		}
		input = strings.TrimSpace(input)

		if input == "" {
			break
		}

		parts := strings.SplitN(input, "=", 2)
		if len(parts) != 2 {
			_, _ = fmt.Fprintln(w, "  Invalid format. Use: alias=model-id")
			continue
		}

		alias := strings.TrimSpace(parts[0])
		modelID := strings.TrimSpace(parts[1])
		if alias == "" || modelID == "" {
			_, _ = fmt.Fprintln(w, "  Invalid format. Use: alias=model-id")
			continue
		}

		result[alias] = modelID
	}
	return result, nil
}

// openInEditor opens a file in the user's editor.
func openInEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// truncatePrompt truncates a prompt for display.
func truncatePrompt(s string, max int) string {
	// Replace newlines with spaces
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// writeCUETags writes a CUE tags array field into a strings.Builder.
func writeCUETags(sb *strings.Builder, tags []string) {
	if len(tags) == 0 {
		return
	}
	sb.WriteString("\t\ttags: [")
	for i, tag := range tags {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(sb, "%q", tag)
	}
	sb.WriteString("]\n")
}

// writeCUEPrompt writes a CUE prompt field into a strings.Builder,
// using triple-quote syntax for long or multi-line prompts.
func writeCUEPrompt(sb *strings.Builder, prompt string) {
	if prompt == "" {
		return
	}
	if strings.Contains(prompt, "\n") || len(prompt) > 80 {
		sb.WriteString("\t\tprompt: \"\"\"\n")
		for line := range strings.SplitSeq(prompt, "\n") {
			fmt.Fprintf(sb, "\t\t\t%s\n", line)
		}
		sb.WriteString("\t\t\t\"\"\"\n")
	} else {
		fmt.Fprintf(sb, "\t\tprompt: %q\n", prompt)
	}
}

// extractTags extracts a string slice from the "tags" field of a CUE value.
func extractTags(val cue.Value) []string {
	tagsVal := val.LookupPath(cue.ParsePath("tags"))
	if !tagsVal.Exists() {
		return nil
	}
	tagIter, err := tagsVal.List()
	if err != nil {
		return nil
	}
	var tags []string
	for tagIter.Next() {
		if s, err := tagIter.Value().String(); err == nil {
			tags = append(tags, s)
		}
	}
	return tags
}

// confirmMultiRemoval prompts the user to confirm removal of one or more config entities.
// For a single name it mirrors the old single-item prompt. For multiple names it lists
// them all and asks once. Requires --yes flag in non-interactive mode.
func confirmMultiRemoval(w io.Writer, r io.Reader, entityType string, names []string, local bool) (bool, error) {
	isTTY := isTerminal(r)

	if !isTTY {
		return false, fmt.Errorf("--yes flag required in non-interactive mode")
	}

	scope := scopeString(local)
	if len(names) == 1 {
		_, _ = fmt.Fprintf(w, "Remove %s %q from %s config? %s ", entityType, names[0], scope, tui.Bracket("y/N"))
	} else {
		_, _ = fmt.Fprintf(w, "Remove the following %ss from %s config?\n", entityType, scope)
		for _, name := range names {
			_, _ = fmt.Fprintf(w, "  - %s\n", name)
		}
		_, _ = fmt.Fprintf(w, "%s ", tui.Bracket("y/N"))
	}

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input != "y" && input != "yes" {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return false, nil
	}
	return true, nil
}

// scopeString returns "local" or "global" based on the flag.
func scopeString(local bool) string {
	if local {
		return "local"
	}
	return "global"
}

// scoreAndSortNames scores each map key against the compiled patterns and
// returns matching keys sorted by score descending then name ascending.
// Each pattern that matches a key contributes 3 to its score, so keys
// matching more query terms rank higher. Keys with score zero are excluded.
func scoreAndSortNames[T any](items map[string]T, patterns []*regexp.Regexp) []string {
	type match struct {
		name  string
		score int
	}
	var matches []match

	for name := range items {
		score := 0
		for _, pattern := range patterns {
			if pattern.MatchString(name) {
				score += 3 // Each matching pattern adds weight; higher score = more query terms matched
			}
		}
		if score > 0 {
			matches = append(matches, match{name: name, score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].name < matches[j].name
	})

	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m.name
	}
	return names
}

// resolveAllMatchingNames resolves a query to all matching names in a map.
// Unlike resolveInstalledName, it returns every match when a query is ambiguous
// rather than erroring. On zero matches it returns a "not found" error.
// Results are sorted by score descending then name ascending.
func resolveAllMatchingNames[T any](items map[string]T, typeName, query string) ([]string, error) {
	// Fast path: exact match — but only when the query contains "/" or has no
	// "query/" siblings. If siblings exist (e.g. "claude" matched alongside
	// "claude/edit"), fall through to the regex search so all are returned.
	if _, ok := items[query]; ok {
		if strings.Contains(query, "/") {
			return []string{query}, nil
		}
		prefix := query + "/"
		hasSiblings := false
		for name := range items {
			if strings.HasPrefix(name, prefix) {
				hasSiblings = true
				break
			}
		}
		if !hasSiblings {
			return []string{query}, nil
		}
		// Has siblings — fall through so the regex search returns all matches.
	}

	terms := modules.ParseSearchPatterns(query)
	if len(terms) == 0 {
		return nil, fmt.Errorf("%s %q not found", typeName, query)
	}

	patterns, err := modules.CompileSearchTerms(terms)
	if err != nil {
		return nil, fmt.Errorf("%s %q not found (invalid pattern: %w)", typeName, query, err)
	}

	names := scoreAndSortNames(items, patterns)
	if len(names) == 0 {
		return nil, fmt.Errorf("%s %q not found", typeName, query)
	}
	return names, nil
}

// parseSelectionInput parses a user selection string containing comma-separated
// numbers and/or ranges (e.g. "1,3,5" or "1-3" or "1,3-5") and returns
// deduplicated 0-based indices in input order. count is the total number of
// selectable items (used for bounds validation).
func parseSelectionInput(input string, count int) ([]int, error) {
	seen := make(map[int]bool)
	var indices []int

	for part := range strings.SplitSeq(input, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if dashIdx := strings.Index(part, "-"); dashIdx > 0 {
			start, err1 := strconv.Atoi(strings.TrimSpace(part[:dashIdx]))
			end, err2 := strconv.Atoi(strings.TrimSpace(part[dashIdx+1:]))
			if err1 != nil || err2 != nil || start < 1 || end > count || start > end {
				return nil, fmt.Errorf("invalid range %q: enter numbers between 1 and %d", part, count)
			}
			for i := start; i <= end; i++ {
				if !seen[i] {
					seen[i] = true
					indices = append(indices, i-1)
				}
			}
		} else {
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 || n > count {
				return nil, fmt.Errorf("invalid selection %q: enter a number between 1 and %d", part, count)
			}
			if !seen[n] {
				seen[n] = true
				indices = append(indices, n-1)
			}
		}
	}

	return indices, nil
}

// promptSelectCategory displays a colour-coded numbered list of config categories
// and returns the chosen category name. Returns "" and nil if the user cancels
// (empty input).
func promptSelectCategory(w io.Writer, r io.Reader, categories []string) (string, error) {
	for i, cat := range categories {
		_, _ = fmt.Fprintf(w, "  %d. %s\n", i+1, tui.CategoryColor(cat).Sprint(cat))
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(categories)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)

	if input == "" {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return "", nil
	}

	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(categories) {
		return "", fmt.Errorf("invalid selection %q: enter a number between 1 and %d", input, len(categories))
	}
	return categories[n-1], nil
}

// promptSelectOneFromList displays a numbered list and lets the user pick a
// single entry by number. Returns "" and nil if the user cancels (empty input).
func promptSelectOneFromList(w io.Writer, r io.Reader, entityType string, names []string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	_, _ = fmt.Fprintf(w, "%d %ss:\n\n", len(names), entityType)
	for i, name := range names {
		_, _ = fmt.Fprintf(w, "  %2d. %s\n", i+1, name)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(names)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)

	if input == "" {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return "", nil
	}

	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(names) {
		return "", fmt.Errorf("invalid selection %q: enter a number between 1 and %d", input, len(names))
	}
	return names[n-1], nil
}

// promptSelectFromList displays a numbered list of candidates and lets the user
// choose which to include. The user may enter comma-separated numbers, ranges
// (e.g. "1-3"), or "all". Returns the chosen names in list order, or nil if
// the user cancels (empty input).
func promptSelectFromList(w io.Writer, r io.Reader, entityType, query string, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	if query != "" {
		_, _ = fmt.Fprintf(w, "Found %d %ss matching %q:\n\n", len(names), entityType, query)
	} else {
		_, _ = fmt.Fprintf(w, "%d %ss:\n\n", len(names), entityType)
	}

	for i, name := range names {
		_, _ = fmt.Fprintf(w, "  %2d. %s\n", i+1, name)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "CSV %s, range %s, or \"all\" supported\n",
		tui.Annotate("1,2,3"), tui.Annotate("1-3"))
	_, _ = fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(names)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return nil, nil
	}
	if input == "all" {
		return names, nil
	}

	indices, err := parseSelectionInput(input, len(names))
	if err != nil {
		return nil, err
	}
	if len(indices) == 0 {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return nil, nil
	}
	selected := make([]string, len(indices))
	for i, idx := range indices {
		selected[i] = names[idx]
	}
	return selected, nil
}

// normalizeCategoryArg converts a category argument (singular or plural) to a
// canonical singular category name ("agent", "role", "context", "task").
// Returns "" for unknown inputs.
func normalizeCategoryArg(arg string) string {
	singular := strings.TrimSuffix(strings.ToLower(arg), "s")
	switch singular {
	case "agent", "role", "context", "task":
		return singular
	}
	return ""
}

// configMatch represents a single cross-category search result.
type configMatch struct {
	Name     string
	Category string // "agent", "role", "context", or "task"
}

// searchAllConfigCategories searches all four config categories for a query string.
// Returns all matches across categories tagged with their category name.
// Zero matches in a category is not an error; the returned slice may be empty.
func searchAllConfigCategories(query string, local bool) ([]configMatch, error) {
	var results []configMatch

	agents, _, err := loadAgentsForScope(local)
	if err != nil {
		return nil, fmt.Errorf("loading agents: %w", err)
	}
	if names, _ := resolveAllMatchingNames(agents, "agent", query); len(names) > 0 {
		for _, name := range names {
			results = append(results, configMatch{Name: name, Category: "agent"})
		}
	}

	roles, _, err := loadRolesForScope(local)
	if err != nil {
		return nil, fmt.Errorf("loading roles: %w", err)
	}
	if names, _ := resolveAllMatchingNames(roles, "role", query); len(names) > 0 {
		for _, name := range names {
			results = append(results, configMatch{Name: name, Category: "role"})
		}
	}

	contexts, _, err := loadContextsForScope(local)
	if err != nil {
		return nil, fmt.Errorf("loading contexts: %w", err)
	}
	if names, _ := resolveAllMatchingNames(contexts, "context", query); len(names) > 0 {
		for _, name := range names {
			results = append(results, configMatch{Name: name, Category: "context"})
		}
	}

	tasks, _, err := loadTasksForScope(local)
	if err != nil {
		return nil, fmt.Errorf("loading tasks: %w", err)
	}
	if names, _ := resolveAllMatchingNames(tasks, "task", query); len(names) > 0 {
		for _, name := range names {
			results = append(results, configMatch{Name: name, Category: "task"})
		}
	}

	return results, nil
}

// promptSelectConfigMatch shows a numbered list of cross-category matches and
// lets the user pick one. Items are displayed as "name (category)".
// Returns a zero-value configMatch{} and nil if the user cancels.
func promptSelectConfigMatch(w io.Writer, r io.Reader, query string, matches []configMatch) (configMatch, error) {
	if query != "" {
		_, _ = fmt.Fprintf(w, "Found %d items matching %q:\n\n", len(matches), query)
	} else {
		_, _ = fmt.Fprintf(w, "%d items:\n\n", len(matches))
	}
	for i, m := range matches {
		_, _ = fmt.Fprintf(w, "  %2d. %s %s\n", i+1, m.Name, tui.Annotate("%s", m.Category))
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(matches)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return configMatch{}, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return configMatch{}, nil
	}
	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(matches) {
		return configMatch{}, fmt.Errorf("invalid selection %q: enter a number between 1 and %d", input, len(matches))
	}
	return matches[n-1], nil
}

// promptSelectConfigMatchesFromList shows a numbered multi-select list of cross-category
// matches and returns the user's selection. Items are displayed as "name (category)".
// Returns nil and nil if the user cancels.
func promptSelectConfigMatchesFromList(w io.Writer, r io.Reader, query string, matches []configMatch) ([]configMatch, error) {
	if len(matches) == 0 {
		return nil, nil
	}
	if query != "" {
		_, _ = fmt.Fprintf(w, "Found %d items matching %q:\n\n", len(matches), query)
	} else {
		_, _ = fmt.Fprintf(w, "%d items:\n\n", len(matches))
	}
	for i, m := range matches {
		_, _ = fmt.Fprintf(w, "  %2d. %s %s\n", i+1, m.Name, tui.Annotate("%s", m.Category))
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "CSV %s, range %s, or \"all\" supported\n",
		tui.Annotate("1,2,3"), tui.Annotate("1-3"))
	_, _ = fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(matches)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return nil, nil
	}
	if input == "all" {
		return matches, nil
	}

	indices, err := parseSelectionInput(input, len(matches))
	if err != nil {
		return nil, err
	}
	if len(indices) == 0 {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return nil, nil
	}
	selected := make([]configMatch, len(indices))
	for i, idx := range indices {
		selected[i] = matches[idx]
	}
	return selected, nil
}

// promptSearchQuery checks if stdin is a TTY and prompts the user for a search query.
// Loops until a valid query (at least 3 characters) is entered.
// Returns the entered query string, or an error in non-interactive mode.
// Returns empty string and nil if the user presses Enter without input.
func promptSearchQuery(w io.Writer, r io.Reader) (string, error) {
	if !isTerminal(r) {
		return "", fmt.Errorf("query required in non-interactive mode")
	}
	reader := bufio.NewReader(r)
	for {
		_, _ = fmt.Fprint(w, "Enter a search query: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			return "", nil
		}
		if len(input) < 3 {
			_, _ = fmt.Fprintln(w, "Query must be at least 3 characters")
			continue
		}
		return input, nil
	}
}

// resolveInstalledName resolves a name from a map using exact match first,
// then regex-based search. Returns the resolved key and value.
// On zero matches, returns a "not found" error.
// On multiple matches, returns an "ambiguous" error listing the matches.
func resolveInstalledName[T any](items map[string]T, typeName, query string) (string, T, error) {
	var zero T

	// Fast path: exact match
	if val, ok := items[query]; ok {
		return query, val, nil
	}

	// Regex-based search across map keys
	terms := modules.ParseSearchPatterns(query)
	if len(terms) == 0 {
		return "", zero, fmt.Errorf("%s %q not found", typeName, query)
	}

	patterns, err := modules.CompileSearchTerms(terms)
	if err != nil {
		return "", zero, fmt.Errorf("%s %q not found (invalid pattern: %w)", typeName, query, err)
	}

	names := scoreAndSortNames(items, patterns)
	switch len(names) {
	case 0:
		return "", zero, fmt.Errorf("%s %q not found", typeName, query)
	case 1:
		return names[0], items[names[0]], nil
	default:
		return "", zero, fmt.Errorf("ambiguous %s %q matches multiple entries: %s",
			typeName, query, strings.Join(names, ", "))
	}
}
