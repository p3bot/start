package orchestration

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/fault"
	"github.com/start-cli/start/internal/temp"
	"golang.org/x/mod/semver"
)

// ContextSelection specifies which contexts to include.
type ContextSelection struct {
	IncludeRequired bool
	IncludeDefaults bool
	Tags            []string
}

// Context represents a resolved context.
type Context struct {
	Name        string
	Description string
	Content     string
	Required    bool
	Default     bool
	Tags        []string
	File        string // Source file path (if file-based)
	Status      string // "loaded", "skipped", "error"
	Error       string // Error message if resolution failed
}

// RoleResolution tracks the resolution status of a role during fallback.
type RoleResolution struct {
	Name     string // Role name (map key or file path)
	Status   string // "loaded", "skipped", "error"
	File     string // Source file path (if file-based)
	Optional bool   // Whether this role is optional
	Error    string // Error message if resolution failed
}

// Composer handles prompt composition from CUE configuration.
type Composer struct {
	processor   *TemplateProcessor
	tempManager *temp.Manager
	workingDir  string
}

// NewComposer creates a new prompt composer.
func NewComposer(processor *TemplateProcessor, workingDir string) *Composer {
	return &Composer{
		processor:   processor,
		tempManager: temp.NewUTDManager(workingDir),
		workingDir:  workingDir,
	}
}

// resolveFileToTemp copies a source file into .start/temp/, returning the temp path
// (empty if filePath is empty). entityType is "task", "role", or "context".
func (c *Composer) resolveFileToTemp(entityType, name, filePath string) (string, error) {
	if filePath == "" {
		return "", nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading %s file %s: %w", entityType, filePath, err)
	}

	tempPath, err := c.tempManager.WriteUTDFile(entityType, name, string(content))
	if err != nil {
		return "", fmt.Errorf("writing %s temp file: %w", entityType, err)
	}

	return tempPath, nil
}

// isCwdPath reports whether filePath is within the working directory (relative paths,
// or absolute paths under workingDir). Such files are already accessible to agents and
// need no temp copy.
func (c *Composer) isCwdPath(filePath string) bool {
	if filePath == "" {
		return false
	}

	if !filepath.IsAbs(filePath) {
		return true
	}

	cleanPath := filepath.Clean(filePath)
	cleanWorkDir := filepath.Clean(c.workingDir)

	return strings.HasPrefix(cleanPath, cleanWorkDir+string(filepath.Separator))
}

// SectionState classifies how a header section resolved so the render layer
// can report it without re-deriving intent from list length or CLI flags.
type SectionState int

const (
	SectionNone    SectionState = iota // nothing available; render "<label>: none"
	SectionListed                      // entries resolved; render the table
	SectionSkipped                     // deliberate opt-out; render "<label>: skipped via <reason>"
)

// SectionOutcome is the producer-set result for one header section. Whichever
// layer makes the decision sets it; the render layer only switches over it.
type SectionOutcome struct {
	State  SectionState
	Reason string // opt-out flag (e.g. "--role none"); set only for SectionSkipped
}

// ComposeResult contains the result of prompt composition.
type ComposeResult struct {
	// Prompt is the fully composed prompt.
	Prompt string
	// Contexts is the list of contexts that were included.
	Contexts []Context
	// Selection is the context selection criteria used.
	Selection ContextSelection
	// Role is the resolved role content.
	Role string
	// RoleFile is the path to the role file (original for cwd files, temp for external/inline).
	RoleFile string
	// RoleName is the name of the role used.
	RoleName string
	// RoleResolutions tracks all roles checked during resolution for UI display.
	RoleResolutions []RoleResolution
	// RoleOutcome classifies how the role section resolved so the render layer
	// reports it with a pure switch. ComposeWithRole stamps SectionListed or
	// SectionNone; the --role none caller stamps SectionSkipped.
	RoleOutcome SectionOutcome
	// Warnings contains any non-fatal issues.
	Warnings []string
}

// Compose builds the final prompt from configuration.
func (c *Composer) Compose(cfg cue.Value, selection ContextSelection, customText string) (ComposeResult, error) {
	var result ComposeResult
	result.Selection = selection
	var promptParts []string
	addedContexts := make(map[string]bool)

	addConfigContext := func(ctx Context) {
		if addedContexts[ctx.Name] {
			return
		}
		addedContexts[ctx.Name] = true

		resolved, err := c.resolveContext(cfg, ctx.Name)
		if err != nil {
			ctx.Status = "error"
			ctx.Error = err.Error()
		} else {
			ctx.Status = "loaded"
			ctx.Content = resolved.Content
			if resolved.Content != "" {
				promptParts = append(promptParts, strings.TrimRight(resolved.Content, "\n"))
			}
		}
		result.Contexts = append(result.Contexts, ctx)
	}

	if selection.IncludeRequired {
		requiredSelection := ContextSelection{IncludeRequired: true}
		contexts, err := c.selectContexts(cfg, requiredSelection)
		if err != nil {
			return result, fmt.Errorf("selecting contexts: %w", err)
		}
		for _, ctx := range contexts {
			addConfigContext(ctx)
		}
	}

	// Explicit tags suppress defaults.
	if selection.IncludeDefaults && len(selection.Tags) == 0 {
		defaultSelection := ContextSelection{IncludeDefaults: true}
		contexts, err := c.selectContexts(cfg, defaultSelection)
		if err != nil {
			return result, fmt.Errorf("selecting contexts: %w", err)
		}
		for _, ctx := range contexts {
			addConfigContext(ctx)
		}
	}

	// User tags are processed in given order.
	for _, tag := range selection.Tags {
		if IsFilePath(tag) {
			ctx := Context{
				Name: tag,
				File: tag,
			}
			content, err := ReadFilePath(tag)
			if err != nil {
				ctx.Status = "error"
				ctx.Error = err.Error()
			} else {
				ctx.Status = "loaded"
				ctx.Content = content
				if content != "" {
					promptParts = append(promptParts, strings.TrimRight(content, "\n"))
				}
			}
			result.Contexts = append(result.Contexts, ctx)
		} else if tag == "default" {
			defaultSelection := ContextSelection{IncludeDefaults: true}
			contexts, err := c.selectContexts(cfg, defaultSelection)
			if err != nil {
				return result, fmt.Errorf("selecting contexts: %w", err)
			}
			for _, ctx := range contexts {
				addConfigContext(ctx)
			}
		} else {
			// Exact context name match takes precedence over tag matching.
			ctxVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyContexts))
			if ctxVal.Exists() && ctxVal.LookupPath(cue.MakePath(cue.Str(tag))).Exists() {
				ctx := Context{Name: tag}
				addConfigContext(ctx)
			} else {
				tagSelection := ContextSelection{Tags: []string{tag}}
				contexts, err := c.selectContexts(cfg, tagSelection)
				if err != nil {
					return result, fmt.Errorf("selecting contexts: %w", err)
				}
				if len(contexts) == 0 {
					result.Warnings = append(result.Warnings, fmt.Sprintf("context %q not found", tag))
				}
				for _, ctx := range contexts {
					addConfigContext(ctx)
				}
			}
		}
	}

	if customText != "" {
		promptParts = append(promptParts, strings.TrimRight(customText, "\n"))
	}

	// Record excluded defaults as "skipped" so they remain visible in the UI.
	defaultSelection := ContextSelection{IncludeDefaults: true}
	allDefaults, err := c.selectContexts(cfg, defaultSelection)
	if err != nil {
		return result, fmt.Errorf("selecting contexts: %w", err)
	}
	for _, ctx := range allDefaults {
		if !addedContexts[ctx.Name] {
			ctx.Status = "skipped"
			result.Contexts = append(result.Contexts, ctx)
		}
	}

	result.Prompt = strings.Join(promptParts, "\n\n")
	return result, nil
}

// ComposeWithRole composes prompt and resolves role.
// When roleName is provided (explicit --role), errors are fatal.
// When using default selection, optional roles are skipped gracefully.
func (c *Composer) ComposeWithRole(cfg cue.Value, selection ContextSelection, roleName, customText string) (result ComposeResult, err error) {
	// Stamp the role outcome from one choke point so every exit — the normal
	// return and both error early-returns — agrees with the final resolutions
	// slice: a row is appended iff roles are configured, so SectionListed and
	// SectionNone are an exact partition. The render layer discards
	// RoleResolutions unless the state is SectionListed, so a missed stamp on an
	// error return would render "Role: none" while error rows exist.
	defer func() {
		if len(result.RoleResolutions) > 0 {
			result.RoleOutcome = SectionOutcome{State: SectionListed}
		} else {
			result.RoleOutcome = SectionOutcome{State: SectionNone}
		}
	}()

	result, err = c.Compose(cfg, selection, customText)
	if err != nil {
		return result, err
	}

	explicitRole := roleName != ""

	if roleName == "" {
		var resolutions []RoleResolution
		var selectErr error
		roleName, resolutions, selectErr = c.selectDefaultRole(cfg)
		result.RoleResolutions = resolutions

		if selectErr != nil {
			return result, selectErr
		}
	}
	result.RoleName = roleName

	if roleName != "" {
		var roleContent string
		var roleFilePath string
		var roleErr error

		if IsFilePath(roleName) {
			roleContent, roleErr = ReadFilePath(roleName)
			if roleErr == nil {
				roleFilePath, _ = ExpandFilePath(roleName)
			}
			res := RoleResolution{
				Name: roleName,
				File: roleName,
			}
			if roleErr != nil {
				res.Status = "error"
				res.Error = roleErr.Error()
			} else {
				res.Status = "loaded"
			}
			result.RoleResolutions = append(result.RoleResolutions, res)
		} else {
			roleContent, roleFilePath, roleErr = c.resolveRole(cfg, roleName)

			if len(result.RoleResolutions) == 0 || result.RoleResolutions[len(result.RoleResolutions)-1].Name != roleName {
				res := RoleResolution{
					Name: roleName,
					File: roleFilePath,
				}
				if roleErr != nil {
					res.Status = "error"
					res.Error = roleErr.Error()
				} else {
					res.Status = "loaded"
				}
				result.RoleResolutions = append(result.RoleResolutions, res)
			}
		}

		if roleErr != nil {
			if explicitRole {
				return result, fmt.Errorf("role %q: %w", roleName, roleErr)
			}
			// Non-explicit role failure is shown in the role table via ○ status
		} else {
			result.Role = roleContent
			result.RoleFile = roleFilePath
		}
	}

	return result, nil
}

// selectContexts returns contexts matching the selection criteria in definition order.
func (c *Composer) selectContexts(cfg cue.Value, selection ContextSelection) ([]Context, error) {
	contextsVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyContexts))
	if !contextsVal.Exists() {
		return nil, nil
	}

	var contexts []Context
	iter, err := contextsVal.Fields()
	if err != nil {
		return nil, fmt.Errorf("iterating contexts: %w", err)
	}

	tagSet := make(map[string]bool)
	for _, tag := range selection.Tags {
		tagSet[tag] = true
	}

	for iter.Next() {
		name := iter.Selector().Unquoted()
		ctxVal := iter.Value()

		ctx := Context{Name: name}

		if desc := ctxVal.LookupPath(cue.ParsePath("description")); desc.Exists() {
			ctx.Description, _ = desc.String()
		}
		if req := ctxVal.LookupPath(cue.ParsePath("required")); req.Exists() {
			ctx.Required, _ = req.Bool()
		}
		if def := ctxVal.LookupPath(cue.ParsePath("default")); def.Exists() {
			ctx.Default, _ = def.Bool()
		}
		if tags := ctxVal.LookupPath(cue.ParsePath("tags")); tags.Exists() {
			tagIter, err := tags.List()
			if err == nil {
				for tagIter.Next() {
					if s, err := tagIter.Value().String(); err == nil {
						ctx.Tags = append(ctx.Tags, s)
					}
				}
			}
		}
		if file := ctxVal.LookupPath(cue.ParsePath("file")); file.Exists() {
			ctx.File, _ = file.String()
		}

		include := false

		if selection.IncludeRequired && ctx.Required {
			include = true
		}

		if selection.IncludeDefaults && ctx.Default {
			include = true
		}

		if len(selection.Tags) > 0 {
			// "default" pseudo-tag matches default contexts.
			if tagSet["default"] && ctx.Default {
				include = true
			}

			for _, tag := range ctx.Tags {
				if tagSet[tag] {
					include = true
					break
				}
			}
		}

		if include {
			contexts = append(contexts, ctx)
		}
	}

	return contexts, nil
}

// resolveContext resolves a context through UTD processing.
func (c *Composer) resolveContext(cfg cue.Value, name string) (ProcessResult, error) {
	ctxVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyContexts)).LookupPath(cue.MakePath(cue.Str(name)))
	if !ctxVal.Exists() {
		return ProcessResult{}, fault.NotFound(fmt.Errorf("context not found"))
	}

	fields := ExtractUTDFields(ctxVal)
	if !IsUTDValid(fields) {
		return ProcessResult{}, fmt.Errorf("invalid UTD: no file, command, or prompt")
	}

	resolved, err := resolveModuleFile(fields.File, ctxVal)
	if err != nil {
		return ProcessResult{}, err
	}
	fields.File = resolved

	// Only external files are copied to temp; cwd files are already accessible.
	var tempPath string
	if fields.File != "" {
		if c.isCwdPath(fields.File) {
			expandedPath, err := ExpandFilePath(fields.File)
			if err != nil {
				return ProcessResult{}, fmt.Errorf("expanding context file path %s: %w", fields.File, err)
			}
			if _, err := os.Stat(expandedPath); err != nil {
				return ProcessResult{}, fmt.Errorf("reading context file %s: %w", fields.File, err)
			}
			fields.File = expandedPath
		} else {
			var err error
			tempPath, err = c.resolveFileToTemp("context", name, fields.File)
			if err != nil {
				return ProcessResult{}, err
			}
			fields.File = tempPath
		}
	}

	result, err := c.processor.Process(fields, "")
	if err != nil {
		return result, err
	}

	result.TempFile = tempPath
	return result, nil
}

// resolveRole resolves a role through UTD processing.
// Returns the resolved content and the file path where the content can be read.
// For file-based roles: returns original path (cwd) or temp path (external).
// For inline roles (prompt/command): writes content to temp and returns temp path.
func (c *Composer) resolveRole(cfg cue.Value, name string) (content, filePath string, err error) {
	roleVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyRoles)).LookupPath(cue.MakePath(cue.Str(name)))
	if !roleVal.Exists() {
		return "", "", fault.NotFound(fmt.Errorf("role not found"))
	}

	fields := ExtractUTDFields(roleVal)
	if !IsUTDValid(fields) {
		return "", "", fmt.Errorf("invalid UTD: no file, command, or prompt")
	}

	resolved, err := resolveModuleFile(fields.File, roleVal)
	if err != nil {
		return "", "", err
	}
	fields.File = resolved

	// roleFilePath backs the {{.role_file}} placeholder; for inline roles it is set
	// to the temp file written after processing.
	var roleFilePath string

	if fields.File != "" {
		if c.isCwdPath(fields.File) {
			expandedPath, err := ExpandFilePath(fields.File)
			if err != nil {
				return "", "", fmt.Errorf("expanding role file path %s: %w", fields.File, err)
			}
			if _, err := os.Stat(expandedPath); err != nil {
				return "", "", fmt.Errorf("reading role file %s: %w", fields.File, err)
			}
			fields.File = expandedPath
			roleFilePath = expandedPath
		} else {
			tempPath, err := c.resolveFileToTemp("role", name, fields.File)
			if err != nil {
				return "", "", err
			}
			fields.File = tempPath
			roleFilePath = tempPath
		}
	}

	result, err := c.processor.Process(fields, "")
	if err != nil {
		return "", "", err
	}

	// Inline roles still need a temp file so {{.role_file}} always has a valid path.
	if roleFilePath == "" && result.Content != "" {
		tempPath, err := c.tempManager.WriteUTDFile("role", name, result.Content)
		if err != nil {
			return "", "", fmt.Errorf("writing role temp file: %w", err)
		}
		roleFilePath = tempPath
	}

	return result.Content, roleFilePath, nil
}

// selectDefaultRole returns the default role name and resolution tracking.
// Roles are checked in definition order:
// - Optional roles with missing files are skipped
// - Required roles with missing files cause an error
// - First available role is selected
// Returns empty roleName with nil error if no roles are defined.
// Returns error if all roles fail or a required role fails.
func (c *Composer) selectDefaultRole(cfg cue.Value) (roleName string, resolutions []RoleResolution, err error) {
	roles := cfg.LookupPath(cue.ParsePath(internalcue.KeyRoles))
	if !roles.Exists() {
		return "", nil, nil
	}

	iter, err := roles.Fields()
	if err != nil {
		return "", nil, fmt.Errorf("iterating roles: %w", err)
	}

	for iter.Next() {
		name := iter.Selector().Unquoted()
		roleVal := iter.Value()

		optional := false
		if opt := roleVal.LookupPath(cue.ParsePath("optional")); opt.Exists() {
			optional, _ = opt.Bool()
		}

		var filePath string
		if file := roleVal.LookupPath(cue.ParsePath("file")); file.Exists() {
			filePath, _ = file.String()
		}

		// Non-file roles (command/prompt only) are always available at selection time.
		available := true
		var checkErr string

		if filePath != "" {
			if ok, msg := roleFileAvailable(filePath, roleVal); !ok {
				available = false
				checkErr = msg
			}
		}

		res := RoleResolution{
			Name:     name,
			File:     filePath,
			Optional: optional,
		}

		if available {
			res.Status = "loaded"
			resolutions = append(resolutions, res)
			return name, resolutions, nil
		}

		if optional {
			res.Status = "skipped"
			res.Error = checkErr
			resolutions = append(resolutions, res)
			continue
		}

		res.Status = "error"
		res.Error = checkErr
		resolutions = append(resolutions, res)
		return "", resolutions, fmt.Errorf("role %q: %s", name, checkErr)
	}

	if len(resolutions) > 0 {
		return "", resolutions, fmt.Errorf("no roles available — all configured roles reference missing files\n  Run 'start config roles' to check your role configuration\n  Run 'start install <role-name>' to install a role from the registry")
	}

	return "", nil, nil
}

// roleFileAvailable reports whether a role's file path is resolvable and present.
// @module/ paths resolve via the role's origin field against the CUE cache;
// non-module paths are expanded and stat'd directly.
func roleFileAvailable(filePath string, roleVal cue.Value) (ok bool, errMsg string) {
	checkPath := filePath
	if strings.HasPrefix(checkPath, "@module/") {
		origin := ExtractOrigin(roleVal)
		if origin == "" {
			return false, "missing origin for @module/ path"
		}
		resolved, err := ResolveModulePath(checkPath, origin)
		if err != nil {
			return false, fmt.Sprintf("resolving module path: %v", err)
		}
		checkPath = resolved
	}
	expandedPath, err := ExpandFilePath(checkPath)
	if err != nil {
		return false, fmt.Sprintf("expanding path: %v", err)
	}
	if _, err := os.Stat(expandedPath); err != nil {
		return false, "file not found"
	}
	return true, ""
}

// ExtractUTDFields extracts UTD fields from a CUE value.
func ExtractUTDFields(v cue.Value) UTDFields {
	var fields UTDFields

	if file := v.LookupPath(cue.ParsePath("file")); file.Exists() {
		fields.File, _ = file.String()
	}
	if cmd := v.LookupPath(cue.ParsePath("command")); cmd.Exists() {
		fields.Command, _ = cmd.String()
	}
	if prompt := v.LookupPath(cue.ParsePath("prompt")); prompt.Exists() {
		fields.Prompt, _ = prompt.String()
	}
	if shell := v.LookupPath(cue.ParsePath("shell")); shell.Exists() {
		fields.Shell, _ = shell.String()
	}
	if timeout := v.LookupPath(cue.ParsePath("timeout")); timeout.Exists() {
		if i, err := timeout.Int64(); err == nil {
			fields.Timeout = int(i)
		}
	}

	return fields
}

// ResolveTask resolves a task by name and processes its UTD.
func (c *Composer) ResolveTask(cfg cue.Value, name, instructions string) (ProcessResult, error) {
	taskVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyTasks)).LookupPath(cue.MakePath(cue.Str(name)))
	if !taskVal.Exists() {
		return ProcessResult{}, fault.NotFound(fmt.Errorf("task %q not found", name))
	}

	fields := ExtractUTDFields(taskVal)
	if !IsUTDValid(fields) {
		return ProcessResult{}, fmt.Errorf("invalid UTD: no file, command, or prompt")
	}

	resolved, err := resolveModuleFile(fields.File, taskVal)
	if err != nil {
		return ProcessResult{}, err
	}
	fields.File = resolved

	// Only external files are copied to temp; cwd files are already accessible.
	var tempPath string
	if fields.File != "" {
		if c.isCwdPath(fields.File) {
			expandedPath, err := ExpandFilePath(fields.File)
			if err != nil {
				return ProcessResult{}, fmt.Errorf("expanding task file path %s: %w", fields.File, err)
			}
			if _, err := os.Stat(expandedPath); err != nil {
				return ProcessResult{}, fmt.Errorf("reading task file %s: %w", fields.File, err)
			}
			fields.File = expandedPath
		} else {
			var err error
			tempPath, err = c.resolveFileToTemp("task", name, fields.File)
			if err != nil {
				return ProcessResult{}, err
			}
			fields.File = tempPath
		}
	}

	result, err := c.processor.Process(fields, instructions)
	if err != nil {
		return result, err
	}

	result.TempFile = tempPath
	return result, nil
}

// ProcessContent processes raw content through template substitution.
// This is used for file-based tasks where the content is read directly
// but still needs template processing for placeholders like {{.instructions}}.
func (c *Composer) ProcessContent(content, instructions string) (ProcessResult, error) {
	fields := UTDFields{
		Prompt: content, // prompt field makes content go through template processing
	}
	return c.processor.Process(fields, instructions)
}

// resolveModuleFile resolves an @module/ file path against the origin field of
// a CUE value. Non-@module/ paths are returned unchanged with a nil error.
// Missing origin or resolver failure produce errors with the install hint.
func resolveModuleFile(file string, v cue.Value) (string, error) {
	if !strings.HasPrefix(file, "@module/") {
		return file, nil
	}
	origin := ExtractOrigin(v)
	if origin == "" {
		return "", fmt.Errorf("missing origin for @module/ path %s\nRun 'start install' to reinstall", file)
	}
	resolved, err := ResolveModulePath(file, origin)
	if err != nil {
		return "", fmt.Errorf("resolving module path %s: %w\nRun 'start install' to reinstall", file, err)
	}
	return resolved, nil
}

// ExtractOrigin extracts the origin field from a CUE value.
func ExtractOrigin(v cue.Value) string {
	if origin := v.LookupPath(cue.ParsePath("origin")); origin.Exists() {
		if s, err := origin.String(); err == nil {
			return s
		}
	}
	return ""
}

// ResolveModulePath resolves an @module/ path to the CUE cache location.
// @module/ paths resolve relative to the cached module directory.
// The origin field contains the exact versioned module path (e.g.,
// "github.com/.../task@v0.1.2") which maps directly to a cache directory.
func ResolveModulePath(path, origin string) (string, error) {
	if !strings.HasPrefix(path, "@module/") {
		return path, nil
	}

	relativePath := strings.TrimPrefix(path, "@module/")

	cacheDir, err := GetCUECacheDir()
	if err != nil {
		return "", fmt.Errorf("getting CUE cache dir: %w", err)
	}

	// The CUE cache stores the version as part of the leaf directory name, so the
	// origin maps directly to cacheDir/mod/extract/<dir>/<base>@<version>/.
	if idx := strings.LastIndex(origin, "@"); idx != -1 {
		modulePath := origin[:idx]
		version := origin[idx:]
		versionedDir := filepath.Join(cacheDir, "mod", "extract",
			filepath.Dir(modulePath),
			filepath.Base(modulePath)+version)
		if _, statErr := os.Stat(versionedDir); statErr == nil {
			return filepath.Join(versionedDir, relativePath), nil
		}
	}

	// Fallback for unversioned origins or a missing exact directory: scan for a version.
	originWithoutVersion := origin
	if idx := strings.LastIndex(origin, "@"); idx != -1 {
		originWithoutVersion = origin[:idx]
	}
	parentDir := filepath.Join(cacheDir, "mod", "extract", filepath.Dir(originWithoutVersion))
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return "", fmt.Errorf("reading cache directory: %w", err)
	}

	baseName := filepath.Base(originWithoutVersion)
	prefix := baseName + "@" // used in sort lambda to extract version substring
	var candidates []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), baseName+"@v") {
			candidates = append(candidates, entry.Name())
		}
	}

	var moduleDir string
	if len(candidates) > 0 {
		slices.SortFunc(candidates, func(a, b string) int {
			return semver.Compare(a[len(prefix):], b[len(prefix):])
		})
		moduleDir = filepath.Join(parentDir, candidates[len(candidates)-1])
	}

	if moduleDir == "" {
		return "", fmt.Errorf("module %s not found in cache", origin)
	}

	return filepath.Join(moduleDir, relativePath), nil
}

// GetCUECacheDir returns the CUE cache directory.
// Respects CUE_CACHE_DIR environment variable.
func GetCUECacheDir() (string, error) {
	if dir := os.Getenv("CUE_CACHE_DIR"); dir != "" {
		return dir, nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "cue"), nil
}

// GetTaskRole returns the role specified for a task.
func GetTaskRole(cfg cue.Value, taskName string) string {
	taskVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyTasks)).LookupPath(cue.MakePath(cue.Str(taskName)))
	if !taskVal.Exists() {
		return ""
	}

	if role := taskVal.LookupPath(cue.ParsePath("role")); role.Exists() {
		if s, err := role.String(); err == nil {
			return s
		}
	}

	return ""
}
