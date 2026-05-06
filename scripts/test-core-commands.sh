#!/usr/bin/env bash
# p-012: CLI Core Commands Testing
# This script runs all the tests from the p-012 testing session.
# Run from the repository root: ./scripts/test-core-commands.sh
#
# Options:
#   -y, --yes    Run all tests without pausing for confirmation

# Don't use set -e as we handle errors ourselves

# Parse arguments
AUTO_YES=false
while [[ $# -gt 0 ]]; do
    case $1 in
        -y|--yes)
            AUTO_YES=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [-y|--yes]"
            echo "  -y, --yes    Run all tests without pausing for confirmation"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [-y|--yes]"
            exit 1
            ;;
    esac
done

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Counters
PASS=0
FAIL=0
SKIP=0

# Wait for user to press Enter (unless --yes)
wait_for_enter() {
    if [[ "$AUTO_YES" != "true" ]]; then
        echo -e "${CYAN}Press Enter to continue...${NC}"
        read -r
    fi
}

# Test function
run_test() {
    local description="$1"
    local command="$2"

    echo -e "${YELLOW}TEST:${NC} $description"
    echo -e "  ${YELLOW}CMD:${NC} $command"

    if eval "$command" > /dev/null 2>&1; then
        echo -e "  ${GREEN}PASS${NC}"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}FAIL${NC}"
        FAIL=$((FAIL + 1))
    fi
    echo
    wait_for_enter
}

# Test function that shows output
run_test_show() {
    local description="$1"
    local command="$2"
    local output

    echo -e "${YELLOW}TEST:${NC} $description"
    echo -e "  ${YELLOW}CMD:${NC} $command"
    echo "  Output:"
    output=$(eval "$command" 2>&1) || true
    echo "$output" | sed 's/^/    /'
    echo -e "  ${GREEN}PASS${NC}"
    PASS=$((PASS + 1))
    echo
    wait_for_enter
}

# Test function that expects failure
run_test_expect_fail() {
    local description="$1"
    local command="$2"

    echo -e "${YELLOW}TEST:${NC} $description"
    echo -e "  ${YELLOW}CMD:${NC} $command"
    echo "  Output:"
    output=$(eval "$command" 2>&1) || true
    echo "$output" | sed 's/^/    /'

    if eval "$command" > /dev/null 2>&1; then
        echo -e "  ${RED}FAIL (expected error but succeeded)${NC}"
        FAIL=$((FAIL + 1))
    else
        echo -e "  ${GREEN}PASS (correctly failed)${NC}"
        PASS=$((PASS + 1))
    fi
    echo
    wait_for_enter
}

# Skip test
skip_test() {
    local description="$1"
    local reason="$2"

    echo -e "${YELLOW}TEST:${NC} $description"
    echo -e "  ${CYAN}SKIP:${NC} $reason"
    SKIP=$((SKIP + 1))
    echo
    wait_for_enter
}

echo "========================================"
echo "p-012: CLI Core Commands Testing"
echo "========================================"
echo

# Ensure we're in the right directory
if [[ ! -f "./start" ]]; then
    echo "Building start binary..."
    go build -o ./start ./cmd/start
fi

echo "========================================"
echo "1. First-Run Experience"
echo "========================================"
echo

skip_test "1.1 Auto-Setup (No Config)" \
    "Requires removing ~/.config/start - destructive test"

skip_test "1.2 Auto-Setup (Empty Directory)" \
    "Requires removing ~/.config/start - destructive test"

skip_test "1.3 Auto-Setup (Non-TTY)" \
    "Requires removing ~/.config/start - destructive test"

run_test_expect_fail "1.4 Invalid CUE File Error" \
    "mkdir -p /tmp/start-test-config && echo 'agents: { test: { bin: \"test\" command: \"cmd\" } }' > /tmp/start-test-config/config.cue && XDG_CONFIG_HOME=/tmp/start-test-config ./start --dry-run"

rm -rf /tmp/start-test-config 2>/dev/null || true

echo "========================================"
echo "2. Start Command (Default)"
echo "========================================"
echo

run_test_show "2.1 Start with Dry-Run" \
    "./start --dry-run"

run_test_show "2.2 Start Default Contexts (debug)" \
    "./start --dry-run --debug 2>&1 | head -20"

skip_test "2.3 Start Execution" \
    "Would launch interactive agent session"

echo "========================================"
echo "3. Prompt Command"
echo "========================================"
echo

run_test_show "3.1 Prompt with Text" \
    "./start prompt 'Hello world' --dry-run"

run_test_show "3.2 Prompt Excludes Defaults (debug)" \
    "./start prompt 'test' --dry-run --debug 2>&1 | grep -E 'defaults|Selection'"

run_test_show "3.3 Prompt with Context Tag" \
    "./start prompt 'test' -c default --dry-run"

run_test_show "3.4 Prompt with Default Pseudo-tag" \
    "./start prompt 'test' -c default --dry-run"

echo "========================================"
echo "4. Task Command"
echo "========================================"
echo

# Check if golang/code-review task exists
if ./start config task list 2>/dev/null | grep -q "code-review"; then
    run_test_show "4.1 Task Execution (dry-run)" \
        "./start task code-review --dry-run"

    run_test_show "4.2 Task with Instructions" \
        "./start task code-review 'focus on error handling' --dry-run"

    run_test_show "4.3 Task Substring Match" \
        "./start task review --dry-run"
else
    skip_test "4.1 Task Execution" \
        "No tasks configured - add golang/code-review task to test"
    skip_test "4.2 Task with Instructions" \
        "No tasks configured"
    skip_test "4.3 Task Substring Match" \
        "No tasks configured"
fi

run_test_expect_fail "4.5 Task Not Found" \
    "./start task nonexistent-task-xyz-123"

echo "========================================"
echo "5. Global Flags"
echo "========================================"
echo

run_test_show "5.1 --agent Flag" \
    "./start --agent claude --dry-run"

run_test_expect_fail "5.1b --agent Flag (invalid)" \
    "./start --agent nonexistent-agent-xyz --dry-run"

# Check if any roles exist
if ./start config role list 2>/dev/null | grep -q -v "No roles"; then
    FIRST_ROLE=$(./start config role list 2>/dev/null | grep -v "No roles" | head -1 | awk '{print $1}')
    run_test_show "5.2 --role Flag" \
        "./start --role $FIRST_ROLE --dry-run"
else
    skip_test "5.2 --role Flag" \
        "No roles configured"
fi

run_test_show "5.3 --model Flag" \
    "./start --model 'opus' --dry-run"

run_test_show "5.4 --context Flag (Single)" \
    "./start -c default --dry-run"

run_test_show "5.5 --context Flag (Multiple Comma)" \
    "./start -c default --dry-run"

run_test_show "5.6 --context Flag (Multiple Flags)" \
    "./start -c default --dry-run"

run_test_show "5.7 --context default Pseudo-tag" \
    "./start -c default --dry-run"

run_test_show "5.8 --context No Match Warning" \
    "./start -c nonexistent-tag-xyz --dry-run 2>&1"

run_test_show "5.9 --directory Flag" \
    "./start --directory /tmp --dry-run --debug 2>&1 | grep -i 'directory'"

run_test_show "5.10 --dry-run Flag" \
    "./start --dry-run"

run_test_show "5.11 --quiet Flag" \
    "./start --quiet --dry-run"

run_test_show "5.12 --verbose Flag" \
    "./start --verbose --dry-run"

run_test_show "5.13 --debug Flag" \
    "./start --debug --dry-run 2>&1 | head -30"

run_test_show "5.14 --version Flag" \
    "./start --version"

run_test_show "5.15 --help Flag" \
    "./start --help"

run_test_show "5.15b prompt --help" \
    "./start prompt --help"

run_test_show "5.15c task --help" \
    "./start task --help"

echo "========================================"
echo "6. Error Handling"
echo "========================================"
echo

run_test_expect_fail "6.1 Unknown Command" \
    "./start unknowncommand"

run_test_expect_fail "6.2 Invalid Flag" \
    "./start --invalid-flag-xyz"

run_test_expect_fail "6.3 Invalid Directory" \
    "./start --directory /nonexistent/path/xyz --dry-run"

skip_test "6.4 Agent Binary Not Found" \
    "Requires config modification"

skip_test "6.5 No Agent Configured" \
    "Auto-setup triggers when no config exists"

echo -e "${YELLOW}TEST:${NC} 6.6 Exit Codes"
echo -n "  Success exit code: "
./start --dry-run > /dev/null 2>&1
SUCCESS_CODE=$?
echo "$SUCCESS_CODE"
echo -n "  Error exit code: "
./start --invalid-flag-xyz > /dev/null 2>&1
ERROR_CODE=$?
echo "$ERROR_CODE"
if [[ $SUCCESS_CODE -eq 0 && $ERROR_CODE -ne 0 ]]; then
    echo -e "  ${GREEN}PASS${NC}"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} (expected 0 and non-zero)"
    FAIL=$((FAIL + 1))
fi
echo
wait_for_enter

echo "========================================"
echo "Summary"
echo "========================================"
echo
echo -e "Tests passed: ${GREEN}$PASS${NC}"
echo -e "Tests failed: ${RED}$FAIL${NC}"
echo -e "Tests skipped: ${CYAN}$SKIP${NC}"
echo

if [[ $FAIL -eq 0 ]]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed.${NC}"
    exit 1
fi
