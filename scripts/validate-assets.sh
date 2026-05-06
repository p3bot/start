#!/usr/bin/env bash
#
# validate-assets.sh - Validate start-assets CUE modules
#
# Checks:
# - All modules have matching git tags
# - All modules are published to CUE Central Registry
# - All dependency versions are consistent
# - All module.cue files are valid
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ASSETS_DIR="${REPO_ROOT}/context/start-assets"

# start-assets is its own git repo
ASSETS_GIT_DIR="$ASSETS_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Counters
PASS=0
FAIL=0
WARN=0

# Module paths (relative to ASSETS_DIR)
MODULES=(
    "schemas"
    "index"
    "agents/aichat"
    "agents/claude"
    "agents/gemini"
    "contexts/agents"
    "contexts/environment"
    "contexts/project"
    "roles/golang/agent"
    "roles/golang/assistant"
    "roles/golang/teacher"
    "tasks/golang/api-docs"
    "tasks/golang/architecture"
    "tasks/golang/code-review"
    "tasks/golang/debug"
    "tasks/golang/dependency-analysis"
    "tasks/golang/error-handling"
    "tasks/golang/performance"
    "tasks/golang/refactor"
    "tasks/golang/security-audit"
    "tasks/golang/tests"
)

pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((PASS++))
}

fail() {
    echo -e "${RED}✗${NC} $1"
    ((FAIL++))
}

warn() {
    echo -e "${YELLOW}!${NC} $1"
    ((WARN++))
}

info() {
    echo -e "  $1"
}

# Get the latest git tag for a module path
get_latest_tag() {
    local module_path="$1"
    git -C "$ASSETS_GIT_DIR" tag -l "${module_path}/v*" | sort -V | tail -1
}

# Get version from tag (e.g., "schemas/v0.1.0" -> "v0.1.0")
tag_to_version() {
    local tag="$1"
    echo "${tag##*/}"
}

# Extract module path from module.cue
get_module_path() {
    local module_dir="$1"
    local module_cue="${module_dir}/cue.mod/module.cue"
    if [[ -f "$module_cue" ]]; then
        grep '^module:' "$module_cue" | sed 's/module: *"\([^"]*\)"/\1/'
    fi
}

# Extract dependency version from module.cue
get_dep_version() {
    local module_dir="$1"
    local dep_module="$2"
    local module_cue="${module_dir}/cue.mod/module.cue"
    if [[ -f "$module_cue" ]]; then
        # Look for the dependency and extract version
        awk -v dep="$dep_module" '
            $0 ~ dep { found=1 }
            found && /v:/ { gsub(/[^v0-9.]/, ""); print; exit }
        ' "$module_cue"
    fi
}

# Check if module is published to registry
check_registry() {
    local module_path="$1"
    local version="$2"
    if cue mod resolve "${module_path}@${version}" >/dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Validate CUE syntax
validate_cue() {
    local module_dir="$1"
    if (cd "$module_dir" && cue vet *.cue 2>/dev/null); then
        return 0
    else
        return 1
    fi
}

echo "========================================"
echo "Start Assets Validation"
echo "========================================"
echo ""

# Check if start-assets is a git repo
if ! git -C "$ASSETS_GIT_DIR" rev-parse --git-dir >/dev/null 2>&1; then
    fail "start-assets is not a git repository"
    exit 1
fi

echo "Checking ${#MODULES[@]} modules..."
echo ""

# Track schemas version for dependency checking
SCHEMAS_VERSION=""

for module in "${MODULES[@]}"; do
    module_dir="${ASSETS_GIT_DIR}/${module}"

    echo "─────────────────────────────────────"
    echo "Module: ${module}"
    echo "─────────────────────────────────────"

    # Check module directory exists
    if [[ ! -d "$module_dir" ]]; then
        fail "Directory not found: ${module_dir}"
        continue
    fi

    # Check module.cue exists
    if [[ ! -f "${module_dir}/cue.mod/module.cue" ]]; then
        fail "module.cue not found"
        continue
    fi

    # Get module path from module.cue
    full_module_path=$(get_module_path "$module_dir")
    if [[ -z "$full_module_path" ]]; then
        fail "Could not parse module path from module.cue"
        continue
    fi
    # Strip @v0 suffix for display
    base_module_path="${full_module_path%@*}"
    info "Module path: ${base_module_path}"

    # Get latest git tag
    latest_tag=$(get_latest_tag "$module")
    if [[ -z "$latest_tag" ]]; then
        fail "No git tag found for ${module}"
        continue
    fi
    version=$(tag_to_version "$latest_tag")
    info "Git tag: ${latest_tag}"

    # Store schemas version for later comparison
    if [[ "$module" == "schemas" ]]; then
        SCHEMAS_VERSION="$version"
    fi

    # Check if tag is pushed (exists in remote)
    if git -C "$ASSETS_GIT_DIR" ls-remote --tags origin | grep -q "refs/tags/${latest_tag}$"; then
        pass "Tag pushed to origin"
    else
        fail "Tag not pushed to origin: ${latest_tag}"
    fi

    # Check registry publication
    if check_registry "$base_module_path" "$version"; then
        pass "Published to registry: ${version}"
    else
        fail "Not published to registry: ${version}"
    fi

    # Validate CUE syntax
    if validate_cue "$module_dir"; then
        pass "CUE syntax valid"
    else
        fail "CUE syntax error"
    fi

    # Check schemas dependency version (skip for schemas itself)
    if [[ "$module" != "schemas" ]]; then
        schemas_dep=$(get_dep_version "$module_dir" "schemas@v0")
        if [[ -n "$schemas_dep" ]]; then
            if [[ "$schemas_dep" == "$SCHEMAS_VERSION" ]]; then
                pass "Schemas dependency: ${schemas_dep}"
            else
                warn "Schemas dependency mismatch: ${schemas_dep} (expected ${SCHEMAS_VERSION})"
            fi
        fi
    fi

    # Check roles dependency for tasks
    if [[ "$module" == tasks/* ]]; then
        roles_dep=$(get_dep_version "$module_dir" "roles/golang/agent@v0")
        if [[ -n "$roles_dep" ]]; then
            # Get expected roles version
            roles_tag=$(get_latest_tag "roles/golang/agent")
            roles_version=$(tag_to_version "$roles_tag")
            if [[ "$roles_dep" == "$roles_version" ]]; then
                pass "Roles dependency: ${roles_dep}"
            else
                warn "Roles dependency mismatch: ${roles_dep} (expected ${roles_version})"
            fi
        fi
    fi

    echo ""
done

echo "========================================"
echo "Summary"
echo "========================================"
echo -e "${GREEN}Passed:${NC}  ${PASS}"
echo -e "${RED}Failed:${NC}  ${FAIL}"
echo -e "${YELLOW}Warnings:${NC} ${WARN}"
echo ""

if [[ $FAIL -gt 0 ]]; then
    echo -e "${RED}Validation failed!${NC}"
    exit 1
elif [[ $WARN -gt 0 ]]; then
    echo -e "${YELLOW}Validation passed with warnings${NC}"
    exit 0
else
    echo -e "${GREEN}All validations passed!${NC}"
    exit 0
fi
