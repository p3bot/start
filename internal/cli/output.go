package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/tui"
)

// writeJSON disables HTML escaping so shell characters are not escaped (unlike
// json.MarshalIndent), e.g. > stays > rather than becoming its \u form.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printWarning emits exactly one trailing newline regardless of the format string.
func printWarning(w io.Writer, format string, args ...any) {
	tui.ColorWarning.Fprint(w, "Warning: ")
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	fmt.Fprintln(w, msg)
}

func printHeader(w io.Writer, text string) {
	fmt.Fprintln(w)
	tui.ColorHeader.Fprintln(w, text)
}

func printSeparator(w io.Writer) {
	tui.ColorSeparator.Fprintln(w, strings.Repeat("─", 79))
}

func printContextTable(w io.Writer, contexts []orchestration.Context, selection orchestration.ContextSelection) {
	if len(contexts) == 0 {
		return
	}

	var parts []string
	if selection.IncludeRequired {
		parts = append(parts, "required")
	}
	if selection.IncludeDefaults && len(selection.Tags) == 0 {
		parts = append(parts, "default")
	}
	for _, tag := range selection.Tags {
		if !orchestration.IsFilePath(tag) {
			parts = append(parts, tag)
		}
	}

	tui.ColorContexts.Fprint(w, "Context:")
	if len(parts) > 0 {
		fmt.Fprintf(w, " %s", tui.Annotate("%s", strings.Join(parts, ", ")))
	}
	fmt.Fprintln(w)

	nameWidth := 4 // "Name" header
	tagsWidth := 4 // "Tags" header
	fileWidth := 4 // "File" header

	type row struct {
		name   string
		status string
		tags   string
		file   string
	}

	rows := make([]row, len(contexts))
	for i, ctx := range contexts {
		status := "✓"
		if ctx.Status == "skipped" || ctx.Status == "error" {
			status = "○"
		}

		var tags []string
		if ctx.Required {
			tags = append(tags, "required")
		}
		if ctx.Default {
			tags = append(tags, "default")
		}
		tags = append(tags, ctx.Tags...)
		tagStr := strings.Join(tags, ", ")
		if tagStr == "" {
			tagStr = "-"
		}

		file := ctx.File
		if file != "" {
			file = filepath.Base(file)
		} else {
			file = "-"
		}
		if ctx.Error != "" {
			file += " (not found)"
		}

		rows[i] = row{
			name:   ctx.Name,
			status: status,
			tags:   tagStr,
			file:   file,
		}

		if len(ctx.Name) > nameWidth {
			nameWidth = len(ctx.Name)
		}
		if len(tagStr) > tagsWidth {
			tagsWidth = len(tagStr)
		}
		if len(file) > fileWidth {
			fileWidth = len(file)
		}
	}

	tui.ColorDim.Fprintf(w, "  %-*s  %s  %-*s  %s\n",
		nameWidth, "Name", "Status", tagsWidth, "Tags", "File")

	for _, r := range rows {
		fmt.Fprint(w, "  ")
		fmt.Fprintf(w, "%-*s  ", nameWidth, r.name)
		if r.status == "✓" {
			tui.ColorSuccess.Fprintf(w, "%s", r.status)
		} else {
			tui.ColorTasks.Fprint(w, r.status)
		}
		fmt.Fprintf(w, "       %-*s  %s\n", tagsWidth, r.tags, r.file)
	}
	fmt.Fprintln(w)
}

func printAgentModel(w io.Writer, agent orchestration.Agent, model, modelSource string) {
	tui.ColorAgents.Fprint(w, "Agent:")
	fmt.Fprintf(w, " %s\n", agent.Name)
	tui.ColorAgents.Fprint(w, "Model:")
	if model != "" {
		fmt.Fprintf(w, " %s %s\n", model, tui.Annotate("via %s", modelSource))
	} else {
		fmt.Fprintln(w, " -")
	}
	fmt.Fprintln(w)
}

// printRoleTable renders the role header section. It is a pure switch over
// outcome.State set by the producer; it must not re-derive the section state
// from len(resolutions) or any CLI flag. resolutions is rendered only for
// SectionListed.
func printRoleTable(w io.Writer, outcome orchestration.SectionOutcome, resolutions []orchestration.RoleResolution) {
	tui.ColorRoles.Fprint(w, "Role:")
	switch outcome.State {
	case orchestration.SectionListed:
		fmt.Fprintln(w)
		printRoleRows(w, resolutions)
	case orchestration.SectionSkipped:
		fmt.Fprintf(w, " skipped %s\n", tui.Annotate("via %s", outcome.Reason))
	default: // SectionNone and any unstamped state degrade to the neutral line.
		fmt.Fprintln(w, " none")
	}
	fmt.Fprintln(w)
}

// printRoleRows renders the role resolution table body for SectionListed.
func printRoleRows(w io.Writer, resolutions []orchestration.RoleResolution) {
	nameWidth := 4 // "Name" header
	for _, r := range resolutions {
		if len(r.Name) > nameWidth {
			nameWidth = len(r.Name)
		}
	}

	tui.ColorDim.Fprintf(w, "  %-*s  %s  %s\n", nameWidth, "Name", "Status", "File")

	for _, r := range resolutions {
		status := "○"
		if r.Status == "loaded" {
			status = "✓"
		}

		file := filepath.Base(r.File)
		if file == "" || file == "." {
			file = "-"
		}

		switch r.Status {
		case "skipped":
			file = "skipped"
		case "error":
			if r.Error != "" {
				file = r.Error
			} else {
				file = "not found"
			}
		}

		fmt.Fprint(w, "  ")
		fmt.Fprintf(w, "%-*s  ", nameWidth, r.Name)
		if status == "✓" {
			tui.ColorSuccess.Fprintf(w, "%s", status)
		} else {
			tui.ColorTasks.Fprint(w, status)
		}
		fmt.Fprintf(w, "       %s\n", file)
	}
}
