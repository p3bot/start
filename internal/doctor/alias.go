package doctor

import (
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/start-cli/start/internal/config"
)

// CheckAliases validates the global alias store. An absent store reports zero
// aliases (not an issue). reserved is the live set of top-level subcommand names
// and cobra aliases, so a name that would be shadowed by a command is flagged.
func CheckAliases(storePath string, reserved map[string]bool) SectionResult {
	section := SectionResult{Name: "Aliases"}

	ctx := cuecontext.New()
	v, exists, err := config.CompileAliasStore(ctx, storePath)
	if err != nil {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusFail,
			Label:   "Store",
			Message: fmt.Sprintf("cannot read %s: %v", shortenPath(storePath), err),
			Fix:     "Check permissions on the alias store",
		})
		return section
	}
	if !exists {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusInfo,
			Label:   "None set",
			Message: "0 aliases",
		})
		return section
	}
	if v.Err() != nil {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusFail,
			Label:   "Parse",
			Message: fmt.Sprintf("%v", v.Err()),
			Fix:     "Fix or remove the alias store with 'start alias open'",
		})
		return section
	}
	if config.HasNonAliasTopLevelKeys(v) {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusFail,
			Label:   "Content",
			Message: "non-aliases top-level keys present",
			Fix:     "Edit the store so it contains only the aliases field ('start alias open')",
		})
		return section
	}

	// A non-struct aliases field (e.g. a hand-edited `aliases: "oops"`) cannot be
	// enumerated. AliasNames is lenient and returns nil for it, so check the type
	// explicitly here rather than silently reporting zero aliases.
	if field := v.LookupPath(cue.ParsePath(config.KeyAliases)); field.Exists() && field.Kind() != cue.StructKind {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusFail,
			Label:   "Content",
			Message: "aliases field must be a map of name to token list",
			Fix:     "Fix the aliases field with 'start alias open'",
		})
		return section
	}

	names := config.AliasNames(v)
	sort.Strings(names)

	count := 0
	for _, name := range names {
		tokens, _, err := config.AliasEntryTokens(v, name)
		if err != nil {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusFail,
				Label:   name,
				Message: fmt.Sprintf("%v", err),
				Fix:     fmt.Sprintf("Fix %q with 'start alias set %s ...'", name, name),
			})
			continue
		}
		if len(tokens) == 0 {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusFail,
				Label:   name,
				Message: "empty value",
				Fix:     fmt.Sprintf("Give %q a value with 'start alias set %s ...'", name, name),
			})
			continue
		}
		if reserved[strings.ToLower(name)] {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusWarn,
				Label:   name,
				Message: "collides with a built-in command and will not resolve",
				Fix:     fmt.Sprintf("Rename %q to a non-command name", name),
			})
			continue
		}
		count++
		section.Results = append(section.Results, CheckResult{
			Status:  StatusPass,
			Label:   name,
			Message: aliasCommandPreview(tokens),
		})
	}

	if len(names) == 0 {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusInfo,
			Label:   "None set",
			Message: "0 aliases",
		})
	}

	section.Summary = fmt.Sprintf("%d valid", count)
	return section
}

// aliasCommandPreview renders the stored tokens as a start command for display.
func aliasCommandPreview(tokens []string) string {
	return "start " + strings.Join(tokens, " ")
}
