package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/start-cli/start/internal/tui"
)

// Shared per-category metadata writers. Each takes a strongly-typed value
// and an io.Writer and emits the labelled metadata lines for that category
// in a fixed order, with the existing skip-when-empty rules.
//
// Each writer owns its own leading blank line: when any field would be
// emitted, the writer prints a single `\n` before the first field; when
// nothing would be emitted, the writer is a no-op. Callers do not need
// (and must not add) a separator before invoking the writer.
//
// For agents, a blank line is also emitted before the `Models:` block when
// prior fields (Description / Bin / Default Model / Tags) emitted content;
// when `Models:` is the only field, the writer's leading `\n` already
// provides the separator.

// writeAgentMetadata writes Description, Bin, Default Model, Tags, then a
// blank line, then the Models block. AgentConfig.Command is intentionally
// not emitted here — both consumers render it outside the metadata block
// (config_get as a header line; describe via ExtractUTDFields).
func writeAgentMetadata(w io.Writer, agent AgentConfig) {
	// hasHeader gates the inner blank line before `Models:`. Keep it in
	// sync with the field emissions below if a new pre-Models field is
	// added to AgentConfig.
	hasHeader := agent.Description != "" || agent.Bin != "" ||
		agent.DefaultModel != "" || len(agent.Tags) > 0
	hasModels := len(agent.Models) > 0
	if !hasHeader && !hasModels {
		return
	}

	label := tui.ColorDim.Sprint
	fmt.Fprintln(w)

	if agent.Description != "" {
		fmt.Fprintf(w, "%s %s\n", label("Description:"), agent.Description)
	}
	if agent.Bin != "" {
		fmt.Fprintf(w, "%s %s\n", label("Bin:"), agent.Bin)
	}
	if agent.DefaultModel != "" {
		fmt.Fprintf(w, "%s %s\n", label("Default Model:"), agent.DefaultModel)
	}
	if len(agent.Tags) > 0 {
		fmt.Fprintf(w, "%s %s\n", label("Tags:"), strings.Join(agent.Tags, ", "))
	}
	if hasModels {
		if hasHeader {
			fmt.Fprintln(w)
		}
		tui.ColorDim.Fprintln(w, "Models:")
		aliases := make([]string, 0, len(agent.Models))
		for alias := range agent.Models {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			fmt.Fprintf(w, "  %s ", alias)
			tui.ColorBlue.Fprint(w, "->")
			fmt.Fprint(w, " ")
			tui.ColorDim.Fprintf(w, "%s\n", agent.Models[alias])
		}
	}
}

// writeRoleMetadata writes Description, File, Command, Prompt,
// Optional (only when true), Tags.
func writeRoleMetadata(w io.Writer, role RoleConfig) {
	if role.Description == "" && role.File == "" && role.Command == "" &&
		role.Prompt == "" && !role.Optional && len(role.Tags) == 0 {
		return
	}

	label := tui.ColorDim.Sprint
	fmt.Fprintln(w)

	if role.Description != "" {
		fmt.Fprintf(w, "%s %s\n", label("Description:"), role.Description)
	}
	if role.File != "" {
		fmt.Fprintf(w, "%s %s\n", label("File:"), role.File)
	}
	if role.Command != "" {
		fmt.Fprintf(w, "%s %s\n", label("Command:"), role.Command)
	}
	if role.Prompt != "" {
		fmt.Fprintf(w, "%s %s\n", label("Prompt:"), truncatePrompt(role.Prompt, 100))
	}
	if role.Optional {
		fmt.Fprintf(w, "%s true\n", label("Optional:"))
	}
	if len(role.Tags) > 0 {
		fmt.Fprintf(w, "%s %s\n", label("Tags:"), strings.Join(role.Tags, ", "))
	}
}

// writeContextMetadata writes Description, File, Command, Prompt,
// Required (always), Default (always), Tags. Required and Default print
// unconditionally, so this writer always produces output.
func writeContextMetadata(w io.Writer, ctx ContextConfig) {
	label := tui.ColorDim.Sprint
	fmt.Fprintln(w)

	if ctx.Description != "" {
		fmt.Fprintf(w, "%s %s\n", label("Description:"), ctx.Description)
	}
	if ctx.File != "" {
		fmt.Fprintf(w, "%s %s\n", label("File:"), ctx.File)
	}
	if ctx.Command != "" {
		fmt.Fprintf(w, "%s %s\n", label("Command:"), ctx.Command)
	}
	if ctx.Prompt != "" {
		fmt.Fprintf(w, "%s %s\n", label("Prompt:"), truncatePrompt(ctx.Prompt, 100))
	}
	fmt.Fprintf(w, "%s %t\n", label("Required:"), ctx.Required)
	fmt.Fprintf(w, "%s %t\n", label("Default:"), ctx.Default)
	if len(ctx.Tags) > 0 {
		fmt.Fprintf(w, "%s %s\n", label("Tags:"), strings.Join(ctx.Tags, ", "))
	}
}

// writeTaskMetadata writes Description, File, Command, Prompt, Role, Tags.
func writeTaskMetadata(w io.Writer, task TaskConfig) {
	if task.Description == "" && task.File == "" && task.Command == "" &&
		task.Prompt == "" && task.Role == "" && len(task.Tags) == 0 {
		return
	}

	label := tui.ColorDim.Sprint
	fmt.Fprintln(w)

	if task.Description != "" {
		fmt.Fprintf(w, "%s %s\n", label("Description:"), task.Description)
	}
	if task.File != "" {
		fmt.Fprintf(w, "%s %s\n", label("File:"), task.File)
	}
	if task.Command != "" {
		fmt.Fprintf(w, "%s %s\n", label("Command:"), task.Command)
	}
	if task.Prompt != "" {
		fmt.Fprintf(w, "%s %s\n", label("Prompt:"), truncatePrompt(task.Prompt, 100))
	}
	if task.Role != "" {
		fmt.Fprintf(w, "%s %s\n", label("Role:"), task.Role)
	}
	if len(task.Tags) > 0 {
		fmt.Fprintf(w, "%s %s\n", label("Tags:"), strings.Join(task.Tags, ", "))
	}
}
