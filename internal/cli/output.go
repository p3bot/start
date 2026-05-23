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

// writeJSON encodes v as indented JSON to w with HTML escaping disabled.
// Use this instead of json.MarshalIndent to prevent shell characters like
// >, <, and & from being escaped to \u003e, \u003c, and \u0026.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printWarning prints a warning message in yellow with exactly one trailing
// newline regardless of whether the caller included one in the format string.
func printWarning(w io.Writer, format string, args ...any) {
	tui.ColorWarning.Fprint(w, "Warning: ")
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	fmt.Fprintln(w, msg)
}

// printHeader prints a header/title in green with a leading blank line.
func printHeader(w io.Writer, text string) {
	fmt.Fprintln(w)
	tui.ColorHeader.Fprintln(w, text)
}

// printSeparator prints a separator line in magenta.
func printSeparator(w io.Writer) {
	tui.ColorSeparator.Fprintln(w, strings.Repeat("─", 79))
}

// printContextTable prints contexts in a table format.
// Shows all contexts (loaded, skipped, and failed) with status indicator.
func printContextTable(w io.Writer, contexts []orchestration.Context, selection orchestration.ContextSelection) {
	if len(contexts) == 0 {
		return
	}

	// Build selection label from criteria (exclude file paths)
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

	// Calculate column widths
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
		// Status: ✓ for loaded, ○ for skipped/error
		status := "✓"
		if ctx.Status == "skipped" || ctx.Status == "error" {
			status = "○"
		}

		// Tags: combine required, default, and tags
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

		// File: show basename, add error info if failed
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

		// Update widths
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

	// Print header
	tui.ColorDim.Fprintf(w, "  %-*s  %s  %-*s  %s\n",
		nameWidth, "Name", "Status", tagsWidth, "Tags", "File")

	// Print rows
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

// printAgentModel prints the Agent and Model lines with colour formatting.
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

// printRoleTable prints the role resolution chain in a table format.
// Shows status indicator: ✓ for loaded, ○ for skipped/error.
func printRoleTable(w io.Writer, resolutions []orchestration.RoleResolution) {
	if len(resolutions) == 0 {
		return
	}

	tui.ColorRoles.Fprint(w, "Role:")
	fmt.Fprintln(w)

	// Calculate column widths
	nameWidth := 4 // "Name" header
	for _, r := range resolutions {
		if len(r.Name) > nameWidth {
			nameWidth = len(r.Name)
		}
	}

	// Print header
	tui.ColorDim.Fprintf(w, "  %-*s  %s  %s\n", nameWidth, "Name", "Status", "File")

	// Print rows
	for _, r := range resolutions {
		// Determine status symbol
		status := "○"
		if r.Status == "loaded" {
			status = "✓"
		}

		// Determine file display
		file := filepath.Base(r.File)
		if file == "" || file == "." {
			file = "-"
		}

		// Add status info for non-loaded roles
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

		// Print row
		fmt.Fprint(w, "  ")
		fmt.Fprintf(w, "%-*s  ", nameWidth, r.Name)
		if status == "✓" {
			tui.ColorSuccess.Fprintf(w, "%s", status)
		} else {
			tui.ColorTasks.Fprint(w, status)
		}
		fmt.Fprintf(w, "       %s\n", file)
	}
	fmt.Fprintln(w)
}
