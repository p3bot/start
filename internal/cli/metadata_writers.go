package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/p3bot/start/internal/tui"
)

// Each writer owns its own leading blank line: when any field would be
// emitted it prints a single `\n` before the first field; otherwise it is a
// no-op. Callers must not add a separator before invoking the writer.

// writeAgentMetadata does not emit AgentConfig.Command: both consumers render
// it outside the metadata block (config_get as a header line; describe via
// ExtractUTDFields).
func writeAgentMetadata(w io.Writer, agent AgentConfig) {
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

// writeContextMetadata always produces output: Required and Default print
// unconditionally.
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
