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

// loadForScope merges global then local entries: a local entry overrides the
// global one of the same name but retains the global entry's position in order.
func loadForScope[T any](
	scope config.Scope,
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

	for _, dir := range paths.ForScope(scope) {
		source := "global"
		if dir == paths.Local {
			source = "local"
		}
		dirItems, dirOrder, err := loadFromDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, err
		}
		for _, name := range dirOrder {
			item := dirItems[name]
			setSource(&item, source)
			items[name] = item
			if !seen[name] {
				order = append(order, name)
				seen[name] = true
			}
		}
	}

	return items, order, nil
}

func promptString(w io.Writer, r io.Reader, label, defaultVal string) (string, error) {
	if base, found := strings.CutSuffix(label, " (optional)"); found {
		fmt.Fprint(w, base)
		fmt.Fprintf(w, " %s", tui.Annotate("optional"))
	} else {
		fmt.Fprint(w, label)
	}
	if defaultVal != "" {
		fmt.Fprintf(w, " %s", tui.Bracket("%s", defaultVal))
	}
	fmt.Fprint(w, ": ")

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

// promptContentSource prompts for a file, command, or inline prompt; exactly
// one of the returned values is non-empty.
func promptContentSource(w io.Writer, r io.Reader, defaultChoice, currentPrompt string) (file, command, prompt string, err error) {
	fmt.Fprintf(w, "\nContent source %s:\n", tui.Annotate("choose one"))
	fmt.Fprintln(w, "  1. File path")
	fmt.Fprintln(w, "  2. Command")
	fmt.Fprintln(w, "  3. Inline prompt")
	fmt.Fprintf(w, "Choice %s: ", tui.Bracket("%s", defaultChoice))

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

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z~]`)

// promptText reads multi-line input: typed lines ending in a blank line, or an
// empty first line to open $EDITOR.
func promptText(w io.Writer, r io.Reader, label, defaultVal string) (string, error) {
	if defaultVal != "" && strings.Contains(defaultVal, "\n") {
		fmt.Fprintf(w, "Current value:\n%s\n\n", defaultVal)
	}

	fmt.Fprint(w, label)
	if defaultVal != "" && !strings.Contains(defaultVal, "\n") {
		fmt.Fprintf(w, " %s", tui.Bracket("%s", defaultVal))
	}
	fmt.Fprintln(w)
	tui.ColorDim.Fprintln(w, "  Type text, then press Enter on a blank line to finish")
	tui.ColorDim.Fprintln(w, "  Or press Enter now to open $EDITOR for full editing")
	tui.ColorDim.Fprintln(w, "  Arrow keys are not supported in this mode")
	tui.ColorSuccess.Fprint(w, "↪ ")

	reader := bufio.NewReader(r)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	firstLine = strings.TrimRight(firstLine, "\r\n")
	firstLine = ansiEscapeRe.ReplaceAllString(firstLine, "")

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

	var lines []string
	lines = append(lines, firstLine)

	for {
		tui.ColorSuccess.Fprint(w, "↪ ")
		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF without trailing newline: keep the partial line.
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

// promptDefaultModel shows a numbered list when models are defined, else falls
// back to free-text input.
func promptDefaultModel(w io.Writer, r io.Reader, current string, models map[string]string) (string, error) {
	if len(models) == 0 {
		return promptString(w, r, "Default model", current)
	}

	// Sort aliases for stable ordering.
	aliases := make([]string, 0, len(models))
	for alias := range models {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	fmt.Fprintln(w, "Default model:")
	for i, alias := range aliases {
		if alias == current {
			fmt.Fprintf(w, "  %d. %s - %s %s\n", i+1, alias, tui.ColorDim.Sprint(models[alias]), tui.Annotate("%s", tui.ColorInstalled.Sprint("current")))
		} else {
			fmt.Fprintf(w, "  %d. %s - %s\n", i+1, alias, tui.ColorDim.Sprint(models[alias]))
		}
	}

	fmt.Fprintln(w)
	if current != "" {
		fmt.Fprintf(w, "Select model %s: ", tui.Annotate("number, alias, or Enter to keep %q", current))
	} else {
		fmt.Fprintf(w, "Select model %s: ", tui.Annotate("number or alias"))
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

	if choice, err := strconv.Atoi(input); err == nil {
		if choice >= 1 && choice <= len(aliases) {
			return aliases[choice-1], nil
		}
		return "", fmt.Errorf("invalid selection: %s (choose 1-%d)", input, len(aliases))
	}

	for _, alias := range aliases {
		if strings.EqualFold(alias, input) {
			return alias, nil
		}
	}

	return "", fmt.Errorf("invalid selection: %q is not a known model alias", input)
}

func promptTags(w io.Writer, r io.Reader, current []string, showCurrent bool) ([]string, error) {
	if showCurrent {
		if len(current) > 0 {
			fmt.Fprintf(w, "Current tags: %s\n", tui.Bracket("%s", strings.Join(current, ", ")))
		} else {
			fmt.Fprintf(w, "Current tags: %s\n", tui.Annotate("none"))
		}
	}
	if showCurrent {
		fmt.Fprintf(w, "Tags %s: ", tui.Annotate("comma-separated, - to clear, Enter to keep"))
	} else {
		fmt.Fprintf(w, "Tags %s: ", tui.Annotate("comma-separated, or Enter to skip"))
	}

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	input = strings.TrimSpace(input)

	if input == "" {
		return current, nil
	}

	if input == "-" {
		return nil, nil
	}

	var tags []string
	for t := range strings.SplitSeq(input, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	return tags, nil
}

// promptModelsAdd collects model aliases for the add flow; returns nil if skipped.
func promptModelsAdd(w io.Writer, r io.Reader) (map[string]string, error) {
	reader := bufio.NewReader(r)
	fmt.Fprintln(w, "Add model aliases (alias=model-id, empty to finish):")
	result, err := readModelAliases(w, reader)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func promptModels(w io.Writer, r io.Reader, current map[string]string) (map[string]string, error) {
	reader := bufio.NewReader(r)

	if len(current) > 0 {
		fmt.Fprintln(w, "Current models:")
		var aliases []string
		for alias := range current {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			fmt.Fprintf(w, "  %s: %s\n", alias, tui.ColorDim.Sprint(current[alias]))
		}
	} else {
		fmt.Fprintf(w, "Current models: %s\n", tui.Annotate("none"))
	}

	fmt.Fprintf(w, "Models: %skeep, %sclear, %sedit %s: ",
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

func promptModelsEdit(w io.Writer, reader *bufio.Reader, current map[string]string) (map[string]string, error) {
	result := make(map[string]string)

	if len(current) > 0 {
		fmt.Fprintln(w, "Edit existing models (Enter to keep, - to delete):")
		var aliases []string
		for alias := range current {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)

		for _, alias := range aliases {
			currentVal := current[alias]
			fmt.Fprintf(w, "  %s %s: ", alias, tui.Bracket("%s", currentVal))

			input, err := reader.ReadString('\n')
			if err != nil {
				return nil, fmt.Errorf("reading input: %w", err)
			}
			input = strings.TrimSpace(input)

			if input == "-" {
				continue
			}
			if input == "" {
				result[alias] = currentVal
			} else {
				result[alias] = input
			}
		}
	}

	fmt.Fprintln(w, "Add new models (alias=model-id, empty to finish):")
	newModels, err := readModelAliases(w, reader)
	if err != nil {
		return nil, err
	}
	maps.Copy(result, newModels)

	return result, nil
}

func readModelAliases(w io.Writer, reader *bufio.Reader) (map[string]string, error) {
	result := make(map[string]string)
	for {
		fmt.Fprint(w, "  ")
		tui.ColorSuccess.Fprint(w, "> ")
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
			fmt.Fprintln(w, "  Invalid format. Use: alias=model-id")
			continue
		}

		alias := strings.TrimSpace(parts[0])
		modelID := strings.TrimSpace(parts[1])
		if alias == "" || modelID == "" {
			fmt.Fprintln(w, "  Invalid format. Use: alias=model-id")
			continue
		}

		result[alias] = modelID
	}
	return result, nil
}

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

func truncatePrompt(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// extractStringList reads a CUE list field as a string slice, returning nil when
// the field is absent or not a list so a missing field round-trips as absent.
func extractStringList(val cue.Value, field string) []string {
	listVal := val.LookupPath(cue.ParsePath(field))
	if !listVal.Exists() {
		return nil
	}
	iter, err := listVal.List()
	if err != nil {
		return nil
	}
	var items []string
	for iter.Next() {
		if s, err := iter.Value().String(); err == nil {
			items = append(items, s)
		}
	}
	return items
}

func scopeString(local bool) string {
	if local {
		return "local"
	}
	return "global"
}

// scoreAndSortNames returns keys matching at least one pattern, sorted by score
// (descending) then name. Each matching pattern adds weight, so keys matching
// more query terms rank higher.
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
				score += 3
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

// resolveAllMatchingNames returns every match for an ambiguous query rather than
// erroring (unlike resolveInstalledName); zero matches returns a "not found" error.
func resolveAllMatchingNames[T any](items map[string]T, typeName, query string) ([]string, error) {
	// Fast path on exact match, but only when the query contains "/" or has no
	// "query/" siblings; if siblings exist (e.g. "claude" alongside "claude/edit"),
	// fall through to the regex search so all are returned.
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
	}

	terms := modules.ParseSearchPatterns(query)
	if len(terms) == 0 {
		return nil, notFoundError(fmt.Errorf("%s %q not found", typeName, query))
	}

	patterns, err := modules.CompileSearchTerms(terms)
	if err != nil {
		return nil, notFoundError(fmt.Errorf("%s %q not found (invalid pattern: %w)", typeName, query, err))
	}

	names := scoreAndSortNames(items, patterns)
	if len(names) == 0 {
		return nil, notFoundError(fmt.Errorf("%s %q not found", typeName, query))
	}
	return names, nil
}

// parseSelectionInput parses comma-separated numbers and/or ranges (e.g.
// "1,3-5") into deduplicated 0-based indices in input order, bounds-checked
// against count.
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

// promptSelectCategory returns the chosen category, or "" and nil if cancelled.
func promptSelectCategory(w io.Writer, r io.Reader, categories []string) (string, error) {
	for i, cat := range categories {
		fmt.Fprintf(w, "  %d. %s\n", i+1, tui.CategoryColor(cat).Sprint(cat))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(categories)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)

	if input == "" {
		fmt.Fprintln(w, "Cancelled.")
		return "", nil
	}

	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(categories) {
		return "", fmt.Errorf("invalid selection %q: enter a number between 1 and %d", input, len(categories))
	}
	return categories[n-1], nil
}

// promptSelectOneFromList returns the picked entry, or "" and nil if cancelled.
func promptSelectOneFromList(w io.Writer, r io.Reader, entityType string, names []string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	fmt.Fprintf(w, "%d %ss:\n\n", len(names), entityType)
	for i, name := range names {
		fmt.Fprintf(w, "  %2d. %s\n", i+1, name)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(names)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)

	if input == "" {
		fmt.Fprintln(w, "Cancelled.")
		return "", nil
	}

	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(names) {
		return "", fmt.Errorf("invalid selection %q: enter a number between 1 and %d", input, len(names))
	}
	return names[n-1], nil
}

// promptSelectFromList accepts comma-separated numbers, ranges, or "all" and
// returns the chosen names in list order, or nil if cancelled.
func promptSelectFromList(w io.Writer, r io.Reader, entityType, query string, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	if query != "" {
		fmt.Fprintf(w, "Found %d %ss matching %q:\n\n", len(names), entityType, query)
	} else {
		fmt.Fprintf(w, "%d %ss:\n\n", len(names), entityType)
	}

	for i, name := range names {
		fmt.Fprintf(w, "  %2d. %s\n", i+1, name)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "CSV %s, range %s, or \"all\" supported\n",
		tui.Annotate("1,2,3"), tui.Annotate("1-3"))
	fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(names)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		fmt.Fprintln(w, "Cancelled.")
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
		fmt.Fprintln(w, "Cancelled.")
		return nil, nil
	}
	selected := make([]string, len(indices))
	for i, idx := range indices {
		selected[i] = names[idx]
	}
	return selected, nil
}

// normalizeCategoryArg maps a singular or plural category arg to its canonical
// singular form, or "" for unknown inputs.
func normalizeCategoryArg(arg string) string {
	singular := strings.TrimSuffix(strings.ToLower(arg), "s")
	switch singular {
	case "agent", "role", "context", "task":
		return singular
	}
	return ""
}

type configMatch struct {
	Name     string
	Category string // "agent", "role", "context", or "task"
}

// searchAllConfigCategories searches all four categories; zero matches is not an
// error, so the returned slice may be empty.
func searchAllConfigCategories(query string, scope config.Scope) ([]configMatch, error) {
	var results []configMatch

	agents, _, err := loadAgentsForScope(scope)
	if err != nil {
		return nil, fmt.Errorf("loading agents: %w", err)
	}
	if names, _ := resolveAllMatchingNames(agents, "agent", query); len(names) > 0 {
		for _, name := range names {
			results = append(results, configMatch{Name: name, Category: "agent"})
		}
	}

	roles, _, err := loadRolesForScope(scope)
	if err != nil {
		return nil, fmt.Errorf("loading roles: %w", err)
	}
	if names, _ := resolveAllMatchingNames(roles, "role", query); len(names) > 0 {
		for _, name := range names {
			results = append(results, configMatch{Name: name, Category: "role"})
		}
	}

	contexts, _, err := loadContextsForScope(scope)
	if err != nil {
		return nil, fmt.Errorf("loading contexts: %w", err)
	}
	if names, _ := resolveAllMatchingNames(contexts, "context", query); len(names) > 0 {
		for _, name := range names {
			results = append(results, configMatch{Name: name, Category: "context"})
		}
	}

	tasks, _, err := loadTasksForScope(scope)
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

// promptSelectConfigMatch returns the picked match, or a zero configMatch{} and
// nil if cancelled.
func promptSelectConfigMatch(w io.Writer, r io.Reader, query string, matches []configMatch) (configMatch, error) {
	if query != "" {
		fmt.Fprintf(w, "Found %d items matching %q:\n\n", len(matches), query)
	} else {
		fmt.Fprintf(w, "%d items:\n\n", len(matches))
	}
	for i, m := range matches {
		fmt.Fprintf(w, "  %2d. %s %s\n", i+1, m.Name, tui.AnnotateCategory(m.Category))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(matches)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return configMatch{}, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		fmt.Fprintln(w, "Cancelled.")
		return configMatch{}, nil
	}
	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(matches) {
		return configMatch{}, fmt.Errorf("invalid selection %q: enter a number between 1 and %d", input, len(matches))
	}
	return matches[n-1], nil
}

// promptSearchQuery loops until a query of at least minLen characters is
// entered; returns "" and nil on empty input, or an error in non-interactive
// mode. Install passes 3 to bound its registry-wide search; uninstall passes 1,
// accepting any non-empty query since it resolves against installed modules.
func promptSearchQuery(w io.Writer, r io.Reader, minLen int) (string, error) {
	if !isTerminal(r) {
		return "", usageError(fmt.Errorf("query required in non-interactive mode"))
	}
	reader := bufio.NewReader(r)
	for {
		fmt.Fprint(w, "Enter a search query: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			return "", nil
		}
		// Measure the floor against the name, excluding any "category:" prefix,
		// to match the non-interactive search/install paths. An unknown category
		// is reported and re-prompted rather than aborting the session.
		_, name, err := modules.SplitCategoryQuery(input)
		if err != nil {
			fmt.Fprintf(w, "%s\n", err)
			continue
		}
		if len(name) < minLen {
			fmt.Fprintf(w, "Query must be at least %d characters\n", minLen)
			continue
		}
		return input, nil
	}
}

// resolveInstalledName resolves a name by exact match then regex search. Zero
// matches returns "not found"; multiple matches returns an "ambiguous" error.
func resolveInstalledName[T any](items map[string]T, typeName, query string) (string, T, error) {
	var zero T

	if val, ok := items[query]; ok {
		return query, val, nil
	}

	terms := modules.ParseSearchPatterns(query)
	if len(terms) == 0 {
		return "", zero, notFoundError(fmt.Errorf("%s %q not found", typeName, query))
	}

	patterns, err := modules.CompileSearchTerms(terms)
	if err != nil {
		return "", zero, notFoundError(fmt.Errorf("%s %q not found (invalid pattern: %w)", typeName, query, err))
	}

	names := scoreAndSortNames(items, patterns)
	switch len(names) {
	case 0:
		return "", zero, notFoundError(fmt.Errorf("%s %q not found", typeName, query))
	case 1:
		return names[0], items[names[0]], nil
	default:
		return "", zero, usageError(fmt.Errorf("ambiguous %s %q matches multiple entries: %s",
			typeName, query, strings.Join(names, ", ")))
	}
}
