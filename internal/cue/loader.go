// Package cue handles CUE configuration loading and validation.
package cue

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueformat "cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
)

// ErrNoCUEFiles is returned when no CUE files are found in the provided directories.
var ErrNoCUEFiles = errors.New("no CUE files found")

// Loader loads and merges CUE configurations from directories.
type Loader struct {
	ctx *cue.Context
}

// NewLoader creates a new CUE loader.
func NewLoader() *Loader {
	return &Loader{
		ctx: cuecontext.New(),
	}
}

// LoadResult contains the result of loading CUE configuration.
type LoadResult struct {
	// Value is the merged CUE value.
	Value cue.Value
	// GlobalLoaded indicates whether global config was loaded.
	GlobalLoaded bool
	// LocalLoaded indicates whether local config was loaded.
	LocalLoaded bool
}

// Load loads CUE configuration from the specified directories.
// Directories are loaded in order, with later directories taking precedence
// via CUE unification (later values override earlier for matching keys).
// Empty or non-existent directories are skipped.
//
// The caller convention is: dirs[0] = global config, dirs[1] = local config.
// GlobalLoaded/LocalLoaded indicate which of these were successfully loaded.
func (l *Loader) Load(dirs []string) (LoadResult, error) {
	var result LoadResult

	if len(dirs) == 0 {
		return result, fmt.Errorf("no configuration directories provided")
	}

	// Track which directories were loaded by their original index
	loaded := make([]bool, len(dirs))

	var values []cue.Value
	for i, dir := range dirs {
		if dir == "" {
			continue
		}

		// Check if directory exists
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, fmt.Errorf("checking directory %s: %w", dir, err)
		}
		if !info.IsDir() {
			return result, fmt.Errorf("%s is not a directory", dir)
		}

		// Check if directory contains any CUE files
		hasCUE, err := HasCUEFiles(dir)
		if err != nil {
			return result, fmt.Errorf("checking for CUE files in %s: %w", dir, err)
		}
		if !hasCUE {
			continue
		}

		// Load CUE instance from directory
		v, err := l.loadDir(dir)
		if err != nil {
			return result, fmt.Errorf("loading %s: %w", dir, err)
		}

		values = append(values, v)
		loaded[i] = true
	}

	// Set loaded flags based on original directory positions
	if len(dirs) > 0 && loaded[0] {
		result.GlobalLoaded = true
	}
	if len(dirs) > 1 && loaded[1] {
		result.LocalLoaded = true
	}

	if len(values) == 0 {
		return result, fmt.Errorf("%w", ErrNoCUEFiles)
	}

	// Merge values with replacement semantics:
	// - Different keys: additive (union)
	// - Same keys: later value completely replaces earlier
	merged, err := l.mergeWithReplacement(values)
	if err != nil {
		return result, fmt.Errorf("merging configurations: %w", err)
	}

	result.Value = merged
	return result, nil
}

// collectionKeys are the top-level keys that use second-level merge semantics.
// Items within these collections are merged additively by name, with later
// values replacing earlier values for the same item name.
var collectionKeys = map[string]bool{
	KeyAgents:   true,
	KeyRoles:    true,
	KeyContexts: true,
	KeyTasks:    true,
}

// mergeWithReplacement merges multiple CUE values with two-level merge semantics:
//
// For collection keys (agents, roles, contexts, tasks):
//   - Items are merged additively by name (global.claude + local.gemini = both exist)
//   - Same-named items: later completely replaces earlier (no field-level merge)
//
// For all other keys (settings, etc.):
//   - Fields are merged additively
//   - Same field: later value replaces earlier value
//
// This differs from CUE's native unification which requires compatible values.
func (l *Loader) mergeWithReplacement(values []cue.Value) (cue.Value, error) {
	if len(values) == 0 {
		return cue.Value{}, fmt.Errorf("no values to merge")
	}
	if len(values) == 1 {
		return values[0], nil
	}

	// Build merged structs for each top-level key.
	// For collections: map[itemName]cue.Value
	// For others: map[fieldName]cue.Value (field-level merge)
	topLevel := make(map[string]map[string]cue.Value)
	var topLevelOrder []string
	itemOrder := make(map[string][]string) // Track order within each top-level key

	for _, v := range values {
		iter, err := v.Fields(cue.All())
		if err != nil {
			return cue.Value{}, fmt.Errorf("iterating fields: %w", err)
		}

		for iter.Next() {
			key := iter.Selector().String()
			fieldValue := iter.Value()

			// Initialise top-level key if first time seeing it
			if _, exists := topLevel[key]; !exists {
				topLevel[key] = make(map[string]cue.Value)
				topLevelOrder = append(topLevelOrder, key)
				itemOrder[key] = nil
			}

			if collectionKeys[key] {
				// Collection key: merge at item level (second-level)
				itemIter, err := fieldValue.Fields(cue.All())
				if err != nil {
					return cue.Value{}, fmt.Errorf("iterating collection %s: %w", key, err)
				}
				for itemIter.Next() {
					itemName := itemIter.Selector().String()
					if _, exists := topLevel[key][itemName]; !exists {
						itemOrder[key] = append(itemOrder[key], itemName)
					}
					// Later item replaces earlier item entirely
					topLevel[key][itemName] = itemIter.Value()
				}
			} else {
				// Non-collection key: check if it's a struct for field-level merge
				// or a scalar value for direct replacement
				if fieldValue.Kind() == cue.StructKind {
					// Struct value: merge at field level (like settings)
					fieldIter, err := fieldValue.Fields(cue.All())
					if err != nil {
						return cue.Value{}, fmt.Errorf("iterating struct %s: %w", key, err)
					}
					for fieldIter.Next() {
						fieldName := fieldIter.Selector().String()
						if _, exists := topLevel[key][fieldName]; !exists {
							itemOrder[key] = append(itemOrder[key], fieldName)
						}
						// Later field replaces earlier field
						topLevel[key][fieldName] = fieldIter.Value()
					}
				} else {
					// Scalar value: later completely replaces earlier
					topLevel[key][""] = fieldValue
					itemOrder[key] = []string{""}
				}
			}
		}
	}

	// Build CUE source from merged structure
	var sb strings.Builder
	sb.WriteString("{\n")

	for _, key := range topLevelOrder {
		items := topLevel[key]
		order := itemOrder[key]

		// Check if this is a non-struct value (stored under empty key)
		if len(order) == 1 && order[0] == "" {
			formatted, err := formatValue(items[""])
			if err != nil {
				return cue.Value{}, fmt.Errorf("formatting field %s: %w", key, err)
			}
			sb.WriteString("\t")
			sb.WriteString(key)
			sb.WriteString(": ")
			sb.WriteString(formatted)
			sb.WriteString("\n")
			continue
		}

		// Struct value: output nested structure
		sb.WriteString("\t")
		sb.WriteString(key)
		sb.WriteString(": {\n")

		for _, itemName := range order {
			itemValue := items[itemName]
			formatted, err := formatValue(itemValue)
			if err != nil {
				return cue.Value{}, fmt.Errorf("formatting %s.%s: %w", key, itemName, err)
			}
			sb.WriteString("\t\t")
			sb.WriteString(itemName)
			sb.WriteString(": ")
			sb.WriteString(formatted)
			sb.WriteString("\n")
		}

		sb.WriteString("\t}\n")
	}

	sb.WriteString("}")

	// Compile the merged source
	merged := l.ctx.CompileString(sb.String())
	if err := merged.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("compiling merged config: %w", err)
	}

	return merged, nil
}

// formatValue formats a CUE value as CUE syntax string.
func formatValue(v cue.Value) (string, error) {
	// Use CUE's native formatting
	syn := v.Syntax(
		cue.Final(),
		cue.Concrete(false),
		cue.Definitions(true),
		cue.Hidden(true),
		cue.Optional(true),
	)

	b, err := cueformat.Node(syn)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// LoadSingle loads CUE configuration from a single directory.
func (l *Loader) LoadSingle(dir string) (cue.Value, error) {
	if dir == "" {
		return cue.Value{}, fmt.Errorf("directory path is empty")
	}

	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return cue.Value{}, fmt.Errorf("directory does not exist: %s", dir)
	}
	if err != nil {
		return cue.Value{}, fmt.Errorf("checking directory: %w", err)
	}
	if !info.IsDir() {
		return cue.Value{}, fmt.Errorf("%s is not a directory", dir)
	}

	hasCUE, err := HasCUEFiles(dir)
	if err != nil {
		return cue.Value{}, fmt.Errorf("checking for CUE files: %w", err)
	}
	if !hasCUE {
		return cue.Value{}, fmt.Errorf("%w in %s", ErrNoCUEFiles, dir)
	}

	return l.loadDir(dir)
}

// loadDir loads a CUE instance from a directory.
func (l *Loader) loadDir(dir string) (cue.Value, error) {
	cfg := &load.Config{
		Dir: dir,
		// Package "*" loads all packages. Files without packages are loaded
		// in the _ package. This allows loading both packaged modules and
		// simple configuration files.
		Package: "*",
	}

	insts := load.Instances([]string{"."}, cfg)
	if len(insts) == 0 {
		return cue.Value{}, fmt.Errorf("no instances found in %s", dir)
	}

	inst := insts[0]
	if inst.Err != nil {
		return cue.Value{}, fmt.Errorf("loading instance: %w", inst.Err)
	}

	v := l.ctx.BuildInstance(inst)
	if err := v.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("building instance: %w", err)
	}

	return v, nil
}

// HasCUEFiles checks if a directory contains any .cue files.
func HasCUEFiles(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".cue") {
			return true, nil
		}
	}

	return false, nil
}

// Context returns the underlying CUE context.
func (l *Loader) Context() *cue.Context {
	return l.ctx
}

// IdentifyBrokenFiles compiles each CUE file individually and returns a
// summary of which files have errors. This is used to provide actionable
// diagnostics when a directory fails to load.
func IdentifyBrokenFiles(paths []string) string {
	ctx := cuecontext.New()
	var lines []string

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			lines = append(lines, fmt.Sprintf("  %s: %v", path, err))
			continue
		}
		if v := ctx.CompileBytes(data, cue.Filename(path)); v.Err() != nil {
			lines = append(lines, fmt.Sprintf("  %s: %v", path, v.Err()))
		}
	}

	if len(lines) == 0 {
		return "  (files parse individually but fail when combined)"
	}
	return strings.Join(lines, "\n")
}
