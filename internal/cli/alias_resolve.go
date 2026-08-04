package cli

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"github.com/p3bot/start/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// runRoot resolves a leading alias token, then executes the root command. The
// alias rewrite happens once, before cobra dispatch, so a spliced first token is
// never re-resolved as another alias (single-pass).
func runRoot(root *cobra.Command, args []string) error {
	rewritten, err := resolveAliasArgs(root, args)
	if err != nil {
		return err
	}
	root.SetArgs(rewritten)
	return root.Execute()
}

// resolveAliasArgs rewrites "start <alias> ..." into the alias's stored tokens,
// spliced in place. It returns args unchanged when the first positional token is
// a known subcommand, matches no alias, or the store is absent or unreadable.
// The only loud failure is a first token that matches a defined-but-malformed
// alias: that surfaces the entry's error rather than falling through.
func resolveAliasArgs(root *cobra.Command, args []string) ([]string, error) {
	idx, ok := firstPositionalIndex(root, args)
	if !ok {
		return args, nil
	}
	candidate := args[idx]

	// A help flag before the alias token means "show help", not "execute": leave
	// args untouched so cobra renders help. A trailing help flag (after the
	// token) is preserved by the rewrite and surfaces the target's help instead.
	if containsHelpFlag(args[:idx]) {
		return args, nil
	}

	// A known subcommand or subcommand alias is never an alias; "start task pc"
	// reaches task, not the alias layer.
	if reservedCommandNames(root)[candidate] {
		return args, nil
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		// Path resolution failure must not block ordinary commands; fall through.
		return args, nil
	}

	ctx := cuecontext.New()
	storePath := config.AliasStorePath(paths)
	v, exists, err := config.CompileAliasStore(ctx, storePath)
	if err != nil || !exists || v.Err() != nil {
		// Absent, unreadable, or unparseable far enough to enumerate names: an
		// ordinary typo must yield cobra's unknown-command error, not store
		// corruption noise.
		return args, nil
	}

	matched, found := matchAliasName(config.AliasNames(v), candidate)
	if !found {
		return args, nil
	}

	tokens, _, err := config.AliasEntryTokens(v, matched)
	if err != nil {
		return nil, fmt.Errorf("alias %q is invalid in %s: %w", matched, storePath, err)
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("alias %q in %s has an empty value; reset it with start alias set %s and a command", matched, storePath, matched)
	}

	rewritten := make([]string, 0, len(args)+len(tokens)-1)
	rewritten = append(rewritten, args[:idx]...)
	rewritten = append(rewritten, tokens...)
	rewritten = append(rewritten, args[idx+1:]...)
	return rewritten, nil
}

// containsHelpFlag reports whether args holds a help flag. Used to detect a help
// request preceding the alias token so the rewrite defers to cobra's help.
func containsHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// matchAliasName finds the stored name matching candidate case-insensitively.
// Stored names are normally lowercased on write, but a hand-edited file may hold
// an uppercase name, which must still resolve.
func matchAliasName(names []string, candidate string) (string, bool) {
	lower := strings.ToLower(candidate)
	for _, name := range names {
		if strings.ToLower(name) == lower {
			return name, true
		}
	}
	return "", false
}

// reservedCommandNames returns every top-level subcommand name and cobra alias.
// Derived live from the command tree so new commands are covered automatically;
// shared by the resolver and by alias-name validation.
func reservedCommandNames(root *cobra.Command) map[string]bool {
	reserved := make(map[string]bool)
	for _, sub := range root.Commands() {
		reserved[sub.Name()] = true
		for _, a := range sub.Aliases {
			reserved[a] = true
		}
	}
	return reserved
}

// firstPositionalIndex returns the index in args of the first positional token,
// skipping persistent flags and their values using the root flag definitions.
// This mirrors pflag's rule (a flag consumes the next arg iff its NoOptDefVal is
// empty), so a flag value equal to an alias name — "start --role pc" — is never
// mistaken for the positional.
func firstPositionalIndex(root *cobra.Command, args []string) (int, bool) {
	lookup := func(name string) *pflag.Flag {
		if f := root.PersistentFlags().Lookup(name); f != nil {
			return f
		}
		return root.Flags().Lookup(name)
	}
	shorthand := func(name string) *pflag.Flag {
		if f := root.PersistentFlags().ShorthandLookup(name); f != nil {
			return f
		}
		return root.Flags().ShorthandLookup(name)
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--":
			// Everything after the terminator is positional.
			if i+1 < len(args) {
				return i + 1, true
			}
			return 0, false
		case strings.HasPrefix(arg, "--"):
			name := arg[2:]
			if found := strings.Contains(name, "="); found {
				i++ // --flag=value carries its value inline.
				continue
			}
			f := lookup(name)
			if f != nil && f.NoOptDefVal == "" {
				i += 2 // --flag value consumes the next token.
			} else {
				i++
			}
		case strings.HasPrefix(arg, "-") && arg != "-":
			if found := strings.Contains(arg, "="); found {
				i++ // -f=value carries its value inline.
				continue
			}
			if shorthandConsumesValue(shorthand, arg[1:]) {
				i += 2
			} else {
				i++
			}
		default:
			return i, true
		}
	}
	return 0, false
}

// shorthandConsumesValue reports whether a shorthand cluster (the chars after
// "-") consumes the following arg as a value. pflag processes shorthands left to
// right; the first value-taking shorthand consumes the cluster remainder as an
// inline value when present, otherwise the next arg.
func shorthandConsumesValue(lookup func(string) *pflag.Flag, cluster string) bool {
	for k := 0; k < len(cluster); k++ {
		f := lookup(string(cluster[k]))
		if f == nil {
			return false // Unknown shorthand; pflag will error, so do not rewrite.
		}
		if f.NoOptDefVal == "" {
			// Value-taking: an inline remainder is the value, else the next arg.
			return k == len(cluster)-1
		}
	}
	return false
}
