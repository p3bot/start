package cli

import "strings"

// noneTokens are the reserved flag values that mean "skip the implicit". For
// --role, any none-token skips role assignment entirely. For --context, a
// none-token suppresses the contexts that load automatically (required and
// default), leaving only the selectors the user names explicitly — so
// "none" alone yields zero contexts while "none,foo" yields just foo.
// "none" is canonical; "nil", "off", and "0" are aliases for it, each matching a
// different mental model (programmer, toggle, falsy-shorthand). The set is
// intentionally fixed at these four.
var noneTokens = map[string]bool{
	"none": true,
	"nil":  true,
	"off":  true,
	"0":    true,
}

// isNoneToken reports whether s, trimmed and case-folded, is a none-token. Empty
// string is never a none-token: it carries the established "use the default"
// meaning for both --role and --context.
func isNoneToken(s string) bool {
	return noneTokens[strings.ToLower(strings.TrimSpace(s))]
}

// resolveContextSkip splits a --context selection into the none-sentinel
// decision and the real selectors. skip is true when any element is a
// none-token; rest is the remaining selectors with every none-token removed, so
// "none,foo" suppresses the implicit contexts yet still selects foo.
func resolveContextSkip(contexts []string) (skip bool, rest []string) {
	for _, c := range contexts {
		if isNoneToken(c) {
			skip = true
			continue
		}
		rest = append(rest, c)
	}
	return skip, rest
}
