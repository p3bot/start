package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/p3bot/agentdex"
	"github.com/p3bot/start/internal/fault"
	"github.com/p3bot/start/internal/skills"
)

// JoinAgent fills Bin from agentdex when Agentdex is set. Catalog bin wins over
// a CUE bin. An empty join key leaves the agent unchanged and does not open
// agentdex. Unknown ids are usage; a missing catalog is transient (no CUE-bin
// fallback). Join opens the catalog Cached (network only on a cold miss) so a
// joined recipe can resolve its bin; models.dev stays CacheOnly. --refresh
// overrides both to Latest.
func JoinAgent(ctx context.Context, agent Agent, workingDir string, opts ...agentdex.Option) (Agent, error) {
	if agent.Agentdex == "" {
		return agent, nil
	}
	idx, err := skills.OpenIndex(workingDir, joinFetchOpts(opts)...)
	if err != nil {
		return agent, mapLaunchCatalogErr(err)
	}
	detail, err := idx.Agents.Get(ctx, agent.Agentdex, agentdex.AgentGetQuery{Enrich: agentdex.EnrichNone})
	if err != nil {
		if errors.Is(err, agentdex.ErrAgentUnknown) {
			return agent, fault.Usage(fmt.Errorf("unknown agentdex id %q", agent.Agentdex))
		}
		return agent, mapLaunchCatalogErr(err)
	}
	if detail.Detection.Found && detail.Detection.BinaryPath != "" {
		agent.Bin = detail.Detection.BinaryPath
	} else {
		agent.Bin = detail.Bin
	}
	return agent, nil
}

// LiveModel is one models.dev row. ID is the agent-facing value filled into
// {{.model}}; CanonicalID is an extra exact-match spelling (often provider/id).
type LiveModel struct {
	ID          string
	CanonicalID string
}

func (m LiveModel) fillID() string {
	if m.ID != "" {
		return m.ID
	}
	return m.CanonicalID
}

func (m LiveModel) names() []string {
	var out []string
	if m.ID != "" {
		out = append(out, m.ID)
	}
	if m.CanonicalID != "" && !strings.EqualFold(m.CanonicalID, m.ID) {
		out = append(out, m.CanonicalID)
	}
	return out
}

// LiveModelIDs returns models.dev rows for a catalog id. Display names are
// omitted so a unique name hit cannot become the CLI model flag. Enrichment
// failures (including CacheOnly miss) return a nil slice and a non-nil error
// so the caller can passthrough. Models.dev is CacheOnly: no HTTP on a cold cache.
func LiveModelIDs(ctx context.Context, id, workingDir string, opts ...agentdex.Option) ([]LiveModel, error) {
	if id == "" {
		return nil, nil
	}
	idx, err := skills.OpenIndex(workingDir, cacheOnlyFetchOpts(opts)...)
	if err != nil {
		return nil, mapLaunchCatalogErr(err)
	}
	detail, err := idx.Agents.Get(ctx, id, agentdex.AgentGetQuery{Enrich: agentdex.EnrichFull})
	if err != nil {
		if errors.Is(err, agentdex.ErrAgentUnknown) {
			return nil, fault.Usage(fmt.Errorf("unknown agentdex id %q", id))
		}
		return nil, err
	}
	var models []LiveModel
	for _, m := range detail.Models {
		if m.ID == "" && m.CanonicalID == "" {
			continue
		}
		models = append(models, LiveModel{ID: m.ID, CanonicalID: m.CanonicalID})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].fillID() < models[j].fillID()
	})
	return models, nil
}

// KnownCatalogID reports whether id is an agentdex catalog key. Catalog
// unavailability or an unknown id is false: callers keep substring fallback.
func KnownCatalogID(ctx context.Context, id, workingDir string, opts ...agentdex.Option) bool {
	if id == "" {
		return false
	}
	idx, err := skills.OpenIndex(workingDir, cacheOnlyFetchOpts(opts)...)
	if err != nil {
		return false
	}
	_, err = idx.Agents.Get(ctx, id, agentdex.AgentGetQuery{Enrich: agentdex.EnrichNone})
	return err == nil
}

// joinFetchOpts is the join policy: catalog Cached so a cold miss fetches
// once; models.dev CacheOnly so EnrichNone cannot trigger HTTP. Callers'
// options follow so --refresh Latest and test fixtures last-write-win.
func joinFetchOpts(opts []agentdex.Option) []agentdex.Option {
	return prependFetch(opts, agentdex.FetchCached, agentdex.FetchCacheOnly)
}

// cacheOnlyFetchOpts is the fail-open hot path: last disk cache only for
// catalog and models.dev. Used by live model matching (passthrough on miss)
// and catalog-id prefixing (false on miss, substring fallback).
func cacheOnlyFetchOpts(opts []agentdex.Option) []agentdex.Option {
	return prependFetch(opts, agentdex.FetchCacheOnly, agentdex.FetchCacheOnly)
}

func prependFetch(opts []agentdex.Option, catalog, models agentdex.Fetch) []agentdex.Option {
	out := make([]agentdex.Option, 0, len(opts)+2)
	out = append(out,
		agentdex.WithCatalogFetch(catalog),
		agentdex.WithModelsFetch(models),
	)
	return append(out, opts...)
}

func mapLaunchCatalogErr(err error) error {
	mapped := mapCatalogFault(err, "cannot resolve agentdex join")
	if mapped == nil {
		return nil
	}
	if errors.Is(err, agentdex.ErrCatalogUnavailable) {
		return fmt.Errorf("%w\n\nRetry, or run 'start doctor' to fetch the catalog", mapped)
	}
	return mapped
}

func mapSetupCatalogErr(err error) error {
	return mapCatalogFault(err, "cannot detect catalogued CLI tools")
}

func mapCatalogFault(err error, purpose string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, agentdex.ErrCatalogUnavailable) {
		return fault.Transient(fmt.Errorf("agent catalog unavailable: %s: %w", purpose, err))
	}
	if errors.Is(err, agentdex.ErrCatalogInvalid) {
		return fault.UserConfig(fmt.Errorf("agent catalog unavailable: %s: %w", purpose, err))
	}
	return err
}

// MatchLiveModel applies exact then unique-substring matching over live models.
// Id and canonical id are aliases of one model: a unique hit fills the
// agent-facing id. Zero or many model hits return the original query
// (passthrough).
func MatchLiveModel(query string, models []LiveModel) string {
	if query == "" || len(models) == 0 {
		return query
	}
	for _, m := range models {
		for _, n := range m.names() {
			if strings.EqualFold(n, query) {
				return m.fillID()
			}
		}
	}
	q := strings.ToLower(query)
	var hits []LiveModel
	for _, m := range models {
		for _, n := range m.names() {
			if strings.Contains(strings.ToLower(n), q) {
				hits = append(hits, m)
				break
			}
		}
	}
	if len(hits) == 1 {
		return hits[0].fillID()
	}
	return query
}
