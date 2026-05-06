package cli

import (
	"fmt"
	"sort"
)

// allConfigCategories is the ordered set of interactive config categories.
var allConfigCategories = []string{"agents", "roles", "contexts", "tasks"}

// loadNamesForCategory loads the ordered list of names for a config category.
// Agents and tasks are returned sorted alphabetically (their config order is not
// meaningful). Roles and contexts preserve config order, which is managed by
// "start config order".
func loadNamesForCategory(category string, local bool) ([]string, error) {
	switch category {
	case "agents":
		_, order, err := loadAgentsForScope(local)
		sort.Strings(order)
		return order, err
	case "roles":
		_, order, err := loadRolesForScope(local)
		return order, err
	case "contexts":
		_, order, err := loadContextsForScope(local)
		return order, err
	case "tasks":
		_, order, err := loadTasksForScope(local)
		sort.Strings(order)
		return order, err
	default:
		return nil, fmt.Errorf("unknown category %q", category)
	}
}
