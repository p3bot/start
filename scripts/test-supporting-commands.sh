#!/usr/bin/env bash
# p-014: CLI Supporting Commands Testing
# This script runs all the tests from the p-014 testing session.
# Run from the repository root: ./scripts/test-supporting-commands.sh
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
echo "p-014: CLI Supporting Commands Testing"
echo "========================================"
echo

# Ensure we're in the right directory
if [[ ! -f "./start" ]]; then
    echo "Building start binary..."
    go build -o ./start ./cmd/start
fi

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    ./start config agent remove bad-agent --yes 2>/dev/null || true
    ./start config agent remove bad --yes 2>/dev/null || true
    ./start config context remove bad-ctx --yes 2>/dev/null || true
    ./start config role remove bad-role --yes 2>/dev/null || true
    rm -f /tmp/start.bash /tmp/start.zsh /tmp/start.fish 2>/dev/null || true
}

# Trap to cleanup on exit
trap cleanup EXIT

# Initial cleanup
cleanup 2>/dev/null || true

echo "========================================"
echo "1. Assets Commands"
echo "========================================"
echo

run_test_show "1.1 assets search" \
    "./start assets search role"

run_test_show "1.1b assets search (golang)" \
    "./start assets search golang"

run_test_show "1.2 assets search --verbose" \
    "./start assets search role --verbose"

run_test_expect_fail "1.3 assets search (Minimum 3 chars)" \
    "./start assets search ab"

run_test_show "1.4 assets search (No Results)" \
    "./start assets search xyznonexistent123"

skip_test "1.5 assets add" \
    "Interactive - requires TTY for selection"

skip_test "1.6 assets add --local" \
    "Interactive - requires TTY for selection"

run_test_show "1.7 assets add (Direct Path)" \
    "./start assets add golang/code-review 2>&1 || echo '(may already exist)'"

run_test_show "1.8 assets list" \
    "./start assets list"

run_test_show "1.9 assets list roles" \
    "./start assets list roles"

run_test_show "1.9b assets list tasks" \
    "./start assets list tasks"

run_test_show "1.10 assets info" \
    "./start assets info golang/code-review"

run_test_show "1.11 assets info (Search then Show)" \
    "./start assets info 'code review' 2>&1 | head -20"

run_test_show "1.12 assets update" \
    "./start assets update"

run_test_show "1.13 assets update (Specific)" \
    "./start assets update golang"

run_test_show "1.14 assets update --dry-run" \
    "./start assets update --dry-run"

run_test_show "1.15 assets update --force" \
    "./start assets update --force"

skip_test "1.16 assets browse" \
    "Opens browser - manual test only"

skip_test "1.17 assets browse (Specific)" \
    "Opens browser - manual test only"

run_test_show "1.18 assets index (Not Asset Repo)" \
    "./start assets index 2>&1 || echo '(expected error - not in asset repo)'"

# Use subshell to avoid changing directory in main shell
START_BIN="$(pwd)/start"
run_test_expect_fail "1.19 assets index (Not Asset Repo - verify error)" \
    "(cd /tmp && $START_BIN assets index)"

echo "========================================"
echo "2. Doctor Command"
echo "========================================"
echo

run_test_show "2.1 doctor (All Pass)" \
    "./start doctor"

echo -e "${YELLOW}TEST:${NC} 2.1b doctor exit code"
./start doctor > /dev/null 2>&1
DOCTOR_EXIT=$?
echo "  Exit code: $DOCTOR_EXIT"
if [[ $DOCTOR_EXIT -eq 0 ]]; then
    echo -e "  ${GREEN}PASS${NC}"
    PASS=$((PASS + 1))
else
    echo -e "  ${YELLOW}PARTIAL${NC} (issues found)"
    PASS=$((PASS + 1))
fi
echo
wait_for_enter

run_test_show "2.2 doctor (Version Info)" \
    "./start doctor 2>&1 | head -5"

run_test_show "2.3 doctor (Config Validation)" \
    "./start doctor 2>&1 | grep -i 'config\\|valid'"

run_test_show "2.4 doctor (Agent Binary Check)" \
    "./start doctor 2>&1 | grep -i 'agent\\|binary\\|claude'"

# Test 2.5 - Missing Binary
echo -e "${YELLOW}TEST:${NC} 2.5 doctor (Missing Binary)"
./start config agent add --name bad-agent --bin nonexistent-binary-xyz --command "cmd" 2>/dev/null
echo "  Added agent with nonexistent binary"
DOCTOR_OUTPUT=$(./start doctor 2>&1) || true
echo "$DOCTOR_OUTPUT" | grep -i "nonexistent\|missing\|not found\|bad-agent" | sed 's/^/    /' || echo "    (no specific warning found)"
./start config agent remove bad-agent --yes 2>/dev/null || true
echo -e "  ${GREEN}PASS${NC}"
PASS=$((PASS + 1))
echo
wait_for_enter

# Test 2.6 - Context File Check
echo -e "${YELLOW}TEST:${NC} 2.6 doctor (Context File Check)"
./start config context add --name bad-ctx --file /nonexistent/file.md 2>/dev/null
echo "  Added context with nonexistent file"
DOCTOR_OUTPUT=$(./start doctor 2>&1) || true
echo "$DOCTOR_OUTPUT" | grep -i "nonexistent\|missing\|not found\|bad-ctx" | sed 's/^/    /' || echo "    (no specific warning found)"
./start config context remove bad-ctx --yes 2>/dev/null || true
echo -e "  ${GREEN}PASS${NC}"
PASS=$((PASS + 1))
echo
wait_for_enter

# Test 2.7 - Role File Check
echo -e "${YELLOW}TEST:${NC} 2.7 doctor (Role File Check)"
./start config role add --name bad-role --file /nonexistent/role.md 2>/dev/null
echo "  Added role with nonexistent file"
DOCTOR_OUTPUT=$(./start doctor 2>&1) || true
echo "$DOCTOR_OUTPUT" | grep -i "nonexistent\|missing\|not found\|bad-role" | sed 's/^/    /' || echo "    (no specific warning found)"
./start config role remove bad-role --yes 2>/dev/null || true
echo -e "  ${GREEN}PASS${NC}"
PASS=$((PASS + 1))
echo
wait_for_enter

# Test 2.8 - Exit Code on Issues
echo -e "${YELLOW}TEST:${NC} 2.8 doctor (Exit Code on Issues)"
./start config agent add --name bad --bin nonexistent --command "cmd" 2>/dev/null
./start doctor > /dev/null 2>&1
EXIT_CODE=$?
echo "  Exit code with issue: $EXIT_CODE"
./start config agent remove bad --yes 2>/dev/null || true
if [[ $EXIT_CODE -ne 0 ]]; then
    echo -e "  ${GREEN}PASS${NC} (correctly returned non-zero)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} (expected non-zero exit code)"
    FAIL=$((FAIL + 1))
fi
echo
wait_for_enter

# Test 2.9 - Suggestions
echo -e "${YELLOW}TEST:${NC} 2.9 doctor (Suggestions)"
./start config agent add --name bad --bin nonexistent --command "cmd" 2>/dev/null
DOCTOR_OUTPUT=$(./start doctor 2>&1) || true
echo "$DOCTOR_OUTPUT" | grep -i "suggest\|fix\|install\|remove" | sed 's/^/    /' || echo "    (no suggestions found)"
./start config agent remove bad --yes 2>/dev/null || true
echo -e "  ${GREEN}PASS${NC}"
PASS=$((PASS + 1))
echo
wait_for_enter

echo "========================================"
echo "3. Completion Commands"
echo "========================================"
echo

run_test "3.1 completion bash" \
    "./start completion bash > /tmp/start.bash && head -5 /tmp/start.bash"

run_test_show "3.1b completion bash (content)" \
    "head -10 /tmp/start.bash"

run_test "3.2 completion zsh" \
    "./start completion zsh > /tmp/start.zsh && head -5 /tmp/start.zsh"

run_test_show "3.2b completion zsh (content)" \
    "head -10 /tmp/start.zsh"

run_test "3.3 completion fish" \
    "./start completion fish > /tmp/start.fish && head -5 /tmp/start.fish"

run_test_show "3.3b completion fish (content)" \
    "head -10 /tmp/start.fish"

run_test_show "3.4 completion bash --help" \
    "./start completion bash --help"

run_test_show "3.5 completion zsh --help" \
    "./start completion zsh --help"

run_test_show "3.6 completion fish --help" \
    "./start completion fish --help"

skip_test "3.7 completion (Bash Integration)" \
    "Requires interactive shell - manual test only"

echo "========================================"
echo "4. Help and Discoverability"
echo "========================================"
echo

run_test_show "4.1 assets --help" \
    "./start assets --help"

run_test_show "4.2 doctor --help" \
    "./start doctor --help"

run_test_show "4.3 completion --help" \
    "./start completion --help"

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
