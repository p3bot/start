package cue

import "cuelang.org/go/cue"

// AgentModels resolves an agent's `models` field into a map from alias to model
// id. It accepts the simple form (alias: "id") and, defensively, the object
// form (alias: {id: "id"}), skipping any entry that resolves to neither.
//
// It returns nil when the agent has no `models` field, and a non-nil (possibly
// empty) map when the field is present. Callers rely on this nil-versus-empty
// distinction to mirror whether the source defined `models` at all.
func AgentModels(agentVal cue.Value) map[string]string {
	modelsVal := agentVal.LookupPath(cue.ParsePath(KeyModels))
	if !modelsVal.Exists() {
		return nil
	}

	models := make(map[string]string)
	iter, err := modelsVal.Fields()
	if err != nil {
		return models
	}
	for iter.Next() {
		if id, ok := resolveModelID(iter.Value()); ok {
			models[iter.Selector().Unquoted()] = id
		}
	}
	return models
}

// resolveModelID extracts a model id from a single `models` entry, accepting the
// simple string form first and falling back to the object `id` form. The second
// return is false when the entry is neither.
func resolveModelID(entry cue.Value) (string, bool) {
	if s, err := entry.String(); err == nil {
		return s, true
	}
	if idVal := entry.LookupPath(cue.ParsePath(KeyID)); idVal.Exists() {
		if s, err := idVal.String(); err == nil {
			return s, true
		}
	}
	return "", false
}
