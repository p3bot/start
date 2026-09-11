package orchestration

import (
	"errors"
	"strings"
	"testing"

	"github.com/p3bot/agentdex"
	"github.com/p3bot/start/internal/fault"
)

func TestMatchLiveModel_IDsOnly(t *testing.T) {
	t.Parallel()
	models := []LiveModel{
		{ID: "claude-sonnet-4-5", CanonicalID: "anthropic/claude-sonnet-4-5"},
		{ID: "claude-haiku-4-5", CanonicalID: "anthropic/claude-haiku-4-5"},
	}

	if got := MatchLiveModel("claude-haiku-4-5", models); got != "claude-haiku-4-5" {
		t.Errorf("exact id = %q", got)
	}
	if got := MatchLiveModel("anthropic/claude-haiku-4-5", models); got != "claude-haiku-4-5" {
		t.Errorf("exact canonical = %q, want agent-facing id", got)
	}
	if got := MatchLiveModel("haiku", models); got != "claude-haiku-4-5" {
		t.Errorf("unique id substring = %q, want claude-haiku-4-5", got)
	}
	if got := MatchLiveModel("sonnet", models); got != "claude-sonnet-4-5" {
		t.Errorf("id+canonical of one model must unique-match, got %q", got)
	}
	if got := MatchLiveModel("Claude Sonnet", models); got != "Claude Sonnet" {
		t.Errorf("display name must passthrough, got %q", got)
	}
	if got := MatchLiveModel("claude", models); got != "claude" {
		t.Errorf("ambiguous id substring must passthrough, got %q", got)
	}
}

func TestMatchLiveModel_TwoGenerationsPassthrough(t *testing.T) {
	t.Parallel()
	models := []LiveModel{
		{ID: "claude-sonnet-4-5", CanonicalID: "anthropic/claude-sonnet-4-5"},
		{ID: "claude-sonnet-4-6", CanonicalID: "anthropic/claude-sonnet-4-6"},
	}
	if got := MatchLiveModel("sonnet", models); got != "sonnet" {
		t.Errorf("two sonnet generations must passthrough, got %q", got)
	}
}

func TestMapLaunchCatalogErr(t *testing.T) {
	t.Parallel()

	unavailable := mapLaunchCatalogErr(agentdex.ErrCatalogUnavailable)
	if unavailable == nil {
		t.Fatal("unavailable: nil")
	}
	if !errors.Is(unavailable, fault.ErrTransient) {
		t.Errorf("unavailable domain = %v, want ErrTransient", unavailable)
	}
	if !errors.Is(unavailable, agentdex.ErrCatalogUnavailable) {
		t.Error("unavailable should keep the agentdex cause")
	}
	if !strings.Contains(unavailable.Error(), "agent catalog unavailable") {
		t.Errorf("unavailable message = %v", unavailable)
	}
	if !strings.Contains(unavailable.Error(), "cannot resolve agentdex join") {
		t.Errorf("launch message = %v, want join wording", unavailable)
	}

	setup := mapSetupCatalogErr(agentdex.ErrCatalogUnavailable)
	if !strings.Contains(setup.Error(), "cannot detect catalogued CLI tools") {
		t.Errorf("setup message = %v, want detection wording", setup)
	}
	if strings.Contains(setup.Error(), "join") {
		t.Errorf("setup message must not say join: %v", setup)
	}

	invalid := mapLaunchCatalogErr(agentdex.ErrCatalogInvalid)
	if invalid == nil {
		t.Fatal("invalid: nil")
	}
	if !errors.Is(invalid, fault.ErrUserConfig) {
		t.Errorf("invalid domain = %v, want ErrUserConfig", invalid)
	}
	if !errors.Is(invalid, agentdex.ErrCatalogInvalid) {
		t.Error("invalid should keep the agentdex cause")
	}

	if got := mapLaunchCatalogErr(nil); got != nil {
		t.Errorf("nil = %v, want nil", got)
	}
	plain := errors.New("other")
	if got := mapLaunchCatalogErr(plain); got != plain {
		t.Errorf("passthrough = %v, want same error", got)
	}
}
