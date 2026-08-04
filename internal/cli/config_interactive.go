package cli

import (
	"fmt"
	"sort"

	"github.com/p3bot/start/internal/config"
)

var allConfigCategories = []string{"agents", "roles", "contexts", "tasks"}

// Agents/tasks sort alphabetically; roles/contexts preserve config (injection) order.
func loadNamesForCategory(category string, scope config.Scope) ([]string, error) {
	switch category {
	case "agents":
		_, order, err := loadAgentsForScope(scope)
		sort.Strings(order)
		return order, err
	case "roles":
		_, order, err := loadRolesForScope(scope)
		return order, err
	case "contexts":
		_, order, err := loadContextsForScope(scope)
		return order, err
	case "tasks":
		_, order, err := loadTasksForScope(scope)
		sort.Strings(order)
		return order, err
	default:
		return nil, fmt.Errorf("unknown category %q", category)
	}
}
