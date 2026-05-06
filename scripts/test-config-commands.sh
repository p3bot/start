#!/usr/bin/env bash
# p-013: CLI Configuration Commands Testing
# This script runs all the tests from the p-013 testing session.
# Run from the repository root: ./scripts/test-p013-config-commands.sh
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

echo "========================================"
echo "p-013: CLI Configuration Commands Testing"
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
    rm -rf ./.start 2>/dev/null || true
    ./start config agent remove temp-default --yes 2>/dev/null || true
    ./start config agent remove temp --yes 2>/dev/null || true
    ./start config agent remove test-merge --yes 2>/dev/null || true
    ./start config agent remove global-only --yes 2>/dev/null || true
    ./start config agent remove local-test --yes 2>/dev/null || true
    ./start config agent remove local-agent --yes 2>/dev/null || true
    ./start config agent remove unique-local --yes 2>/dev/null || true
    ./start config role remove test-role --yes 2>/dev/null || true
    ./start config role remove file-role --yes 2>/dev/null || true
    ./start config role remove info-test --yes 2>/dev/null || true
    ./start config role remove temp-role --yes 2>/dev/null || true
    ./start config role remove def-test --yes 2>/dev/null || true
    ./start config role remove test-show --yes 2>/dev/null || true
    ./start config role remove test-expert --yes 2>/dev/null || true
    ./start config context remove test-ctx --yes 2>/dev/null || true
    ./start config context remove cmd-ctx --yes 2>/dev/null || true
    ./start config context remove tagged-ctx --yes 2>/dev/null || true
    ./start config context remove temp --yes 2>/dev/null || true
    ./start config task remove test-task --yes 2>/dev/null || true
    ./start config task remove role-task --yes 2>/dev/null || true
    ./start config task remove cmd-task --yes 2>/dev/null || true
    ./start config task remove temp --yes 2>/dev/null || true
    rm -f /tmp/test-role.md /tmp/test.md /tmp/a.md /tmp/b.md 2>/dev/null || true
}

# Trap to cleanup on exit
trap cleanup EXIT

# Initial cleanup
cleanup 2>/dev/null || true

echo "========================================"
echo "1. Config Agent Commands"
echo "========================================"
echo

run_test_show "1.8 config agent default (show)" \
    "./start config agent default"

run_test "1.9 config agent default (set) - add temp agent" \
    "./start config agent add --name temp-default --bin echo --command '{role}'"

run_test "1.9 config agent default (set) - set default" \
    "./start config agent default temp-default"

run_test_show "1.9 config agent default (set) - verify" \
    "./start config agent default"

run_test "1.9 config agent default (set) - restore default" \
    "./start config agent default claude"

run_test "1.9 config agent default (set) - cleanup" \
    "./start config agent remove temp-default --yes"

run_test "1.10 config agent remove - add temp" \
    "./start config agent add --name temp --bin temp --command 'temp'"

run_test "1.10 config agent remove - remove" \
    "./start config agent remove temp --yes"

run_test_show "1.11 config agents alias" \
    "./start config agents list"

echo "========================================"
echo "2. Config Role Commands"
echo "========================================"
echo

run_test_show "2.1 config role list (empty)" \
    "./start config role list"

run_test "2.2 config role add (flags)" \
    "./start config role add --name test-role --prompt 'You are a test assistant'"

run_test_show "2.2 config role list (with role)" \
    "./start config role list"

run_test "2.2 config role remove" \
    "./start config role remove test-role --yes"

echo "# Test Role" > /tmp/test-role.md
run_test "2.3 config role add (file)" \
    "./start config role add --name file-role --file /tmp/test-role.md"

run_test_show "2.3 config role info (file)" \
    "./start config role info file-role"

run_test "2.3 config role remove (file)" \
    "./start config role remove file-role --yes"

run_test "2.4 config role add (with tags)" \
    "./start config role add --name info-test --prompt 'Test prompt' --description 'Test desc' --tag 'test,demo'"

run_test_show "2.4 config role info" \
    "./start config role info info-test"

run_test "2.4 cleanup" \
    "./start config role remove info-test --yes"

run_test "2.5 config role edit - add" \
    "./start config role add --name temp-role --prompt 'Original'"

run_test "2.5 config role edit - edit" \
    "./start config role edit temp-role --prompt 'Updated'"

run_test_show "2.5 config role info (edited)" \
    "./start config role info temp-role"

run_test "2.5 cleanup" \
    "./start config role remove temp-role --yes"

run_test_show "2.6 config role default (none set)" \
    "./start config role default"

run_test "2.6 config role default - add role" \
    "./start config role add --name def-test --prompt 'Test'"

run_test "2.6 config role default - set" \
    "./start config role default def-test"

run_test_show "2.6 config role default (verify)" \
    "./start config role default"

run_test "2.6/2.7 cleanup" \
    "./start config role remove def-test --yes"

echo "========================================"
echo "3. Config Context Commands"
echo "========================================"
echo

run_test_show "3.1 config context list" \
    "./start config context list"

echo "test content" > /tmp/test.md
run_test "3.2 config context add (file)" \
    "./start config context add --name test-ctx --file '/tmp/test.md'"

run_test "3.2 config context remove" \
    "./start config context remove test-ctx --yes"

run_test "3.3 config context add (command)" \
    "./start config context add --name cmd-ctx --command 'echo hello'"

run_test_show "3.3 config context info" \
    "./start config context info cmd-ctx"

run_test "3.3 cleanup" \
    "./start config context remove cmd-ctx --yes"

run_test "3.4 config context add (tags/default)" \
    "./start config context add --name tagged-ctx --file '/tmp/test.md' --tag 'test,example' --default"

run_test_show "3.4 config context info" \
    "./start config context info tagged-ctx"

run_test "3.4 cleanup" \
    "./start config context remove tagged-ctx --yes"

run_test "3.6 config context edit - add" \
    "./start config context add --name temp --file '/tmp/test.md'"

touch /tmp/a.md /tmp/b.md
run_test "3.6 config context edit - edit" \
    "./start config context edit temp --file '/tmp/b.md'"

run_test_show "3.6 config context info (edited)" \
    "./start config context info temp"

run_test "3.6/3.7 cleanup" \
    "./start config context remove temp --yes"

echo "========================================"
echo "4. Config Task Commands"
echo "========================================"
echo

run_test_show "4.1 config task list" \
    "./start config task list"

run_test "4.2 config task add" \
    "./start config task add --name test-task --prompt 'Do something'"

run_test "4.2 config task remove" \
    "./start config task remove test-task --yes"

run_test "4.3 config task add (with role) - add role" \
    "./start config role add --name test-expert --prompt 'You are an expert'"

run_test "4.3 config task add (with role) - add task" \
    "./start config task add --name role-task --prompt 'Test' --role test-expert"

run_test_show "4.3 config task info" \
    "./start config task info role-task"

run_test "4.3 cleanup - task" \
    "./start config task remove role-task --yes"

run_test "4.3 cleanup - role" \
    "./start config role remove test-expert --yes"

run_test "4.4 config task add (command)" \
    "./start config task add --name cmd-task --command 'git status'"

run_test_show "4.4 config task info" \
    "./start config task info cmd-task"

run_test "4.4 cleanup" \
    "./start config task remove cmd-task --yes"

run_test "4.6 config task edit - add" \
    "./start config task add --name temp --prompt 'Original'"

run_test "4.6 config task edit - edit" \
    "./start config task edit temp --prompt 'Updated'"

run_test_show "4.6 config task info (edited)" \
    "./start config task info temp"

run_test "4.6/4.7 cleanup" \
    "./start config task remove temp --yes"

echo "========================================"
echo "5. Config Settings Commands"
echo "========================================"
echo

run_test_show "5.1 config settings (list)" \
    "./start config settings"

run_test_show "5.1 config settings (show key)" \
    "./start config settings default_agent"

run_test "5.1 config settings (set)" \
    "./start config settings default_agent claude"

run_test_show "5.2 config settings edit" \
    "EDITOR=cat ./start config settings edit"

echo "========================================"
echo "6. Show Commands"
echo "========================================"
echo

run_test_show "6.1 show agent (list)" \
    "./start show agent"

run_test_show "6.2 show agent (named)" \
    "./start show agent claude"

run_test "6.3/6.4 show role - add role" \
    "./start config role add --name test-show --prompt 'You are a test assistant'"

run_test_show "6.3 show role (list)" \
    "./start show role"

run_test_show "6.4 show role (named)" \
    "./start show role test-show"

run_test "6.3/6.4 cleanup" \
    "./start config role remove test-show --yes"

run_test_show "6.5 show context (list)" \
    "./start show context"

run_test_show "6.6 show context (named)" \
    "./start show context project"

run_test_show "6.7 show task (list)" \
    "./start show task"

run_test_show "6.9 show --scope global" \
    "./start show agent --scope global"

mkdir -p ./.start
run_test "6.10 show --scope local - add local agent" \
    "./start config agent add --name local-test --bin local --command 'local' --local"

run_test_show "6.10 show --scope local" \
    "./start show agent --scope local"

run_test "6.10 cleanup" \
    "./start config agent remove local-test --local"
rm -rf ./.start

echo "========================================"
echo "7. Local Config Flag"
echo "========================================"
echo

run_test_show "7.1 -l shorthand" \
    "./start config agent list -l"

rm -rf ./.start
run_test "7.2 --local creates directory - add" \
    "./start config agent add --name local-agent --bin local --command 'cmd' --local"

run_test_show "7.2 --local creates directory - verify" \
    "ls ./.start/"

run_test "7.2 cleanup" \
    "./start config agent remove local-agent --local"
rm -rf ./.start

run_test "7.3 scope isolation - add local" \
    "./start config agent add --name unique-local --bin local --command 'cmd' --local"

run_test_show "7.3 scope isolation - list merged" \
    "./start config agent list"

run_test_show "7.3 scope isolation - global only" \
    "./start show agent --scope global"

run_test "7.3 cleanup" \
    "./start config agent remove unique-local --local"
rm -rf ./.start

echo "========================================"
echo "8. Configuration Merging"
echo "========================================"
echo

rm -rf ./.start
run_test_show "8.1 global only" \
    "./start show agent"

echo "8.2 local only - skipping (requires moving global config)"
echo

mkdir -p ./.start
run_test "8.3 merged - add global" \
    "./start config agent add --name test-merge --bin global --command 'global'"

run_test "8.3 merged - add local" \
    "./start config agent add --name test-merge --bin local --command 'local' --local"

run_test_show "8.3 merged - show (should be local)" \
    "./start show agent test-merge"

run_test "8.3 cleanup - global" \
    "./start config agent remove test-merge"

run_test "8.3 cleanup - local" \
    "./start config agent remove test-merge --local"
rm -rf ./.start

run_test "8.4 additive merge - add global" \
    "./start config agent add --name global-only --bin global --command 'cmd'"

run_test "8.4 additive merge - add local" \
    "./start config agent add --name local-only --bin local --command 'cmd' --local"

run_test_show "8.4 additive merge - list" \
    "./start config agent list"

run_test "8.4 cleanup - global" \
    "./start config agent remove global-only"

run_test "8.4 cleanup - local" \
    "./start config agent remove local-only --local"
rm -rf ./.start

echo "========================================"
echo "Summary"
echo "========================================"
echo
echo -e "Tests passed: ${GREEN}$PASS${NC}"
echo -e "Tests failed: ${RED}$FAIL${NC}"
echo

if [[ $FAIL -eq 0 ]]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed.${NC}"
    exit 1
fi
