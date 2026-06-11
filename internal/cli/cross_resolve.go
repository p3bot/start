package cli

// resolveCross resolves a cross-category identifier (start get, start describe)
// through the unified engine, spanning all four categories. The exact tier
// consults both installed config and the registry across every scoped category,
// so a same-name twin in another category is detected wherever it lives; a lone
// exact resolves directly (installing a registry-only one), and more than one
// falls to selection. A filesystem path bypasses the search and is returned for
// the caller to read directly.
//
// Callers needing clean stdout (e.g. start get piping content) put stderr in the
// stdout slot: newResolver(cfg, flags, stderr, stderr, stdin).
//
// Post-call: when r.cfgStale is true, the module is on disk but r.cfg is not
// refreshed; a caller that then reads r.cfg.Value must call r.reloadConfig
// first, which clears cfgStale (see runGet, runDescribeSearch).
func (r *resolver) resolveCross(query string) (resolveOutcome, error) {
	return r.resolve(query, crossCategoryScope())
}
