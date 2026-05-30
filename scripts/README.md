# Scripts

Development and testing scripts for the `start` CLI.

## Testing

### Unit Tests

```bash
./scripts/invoke-tests          # Run Go unit tests
```

### Manual CLI Walkthrough

```bash
./scripts/manual-test           # Step through every command interactively
./scripts/manual-test --reset   # Clear saved progress and start over
```

Prints each command, copies it to the clipboard, and waits for Enter. Progress
is saved between runs so you can resume where you left off. Covers the full
current command surface: start, prompt, task, search, the config verb tree,
describe, modules, doctor, and completion.

### Auto-Setup Wizard

```bash
./scripts/test-auto-setup single    # One agent: auto-selected, no prompt
./scripts/test-auto-setup multi      # Three agents: walk the tool-selection prompt
./scripts/test-auto-setup no-agent   # No agents: "No AI CLI tools detected" path
```

Drives the first-run auto-setup wizard in a throwaway `HOME`/XDG sandbox on a
sterile PATH of fake agent shims. stdin stays attached to your terminal, so the
wizard is interactive — you answer the prompts by hand. The shims capture the
exact command `start` execs, then the generated config is printed. Any arguments
after the scenario are passed through to `start` (e.g. `test-auto-setup multi
--debug`). Requires network: the CUE module cache is sandboxed, so each run
fetches fresh from the CUE Central Registry.

## Development

```bash
./scripts/upsert-context        # Update context index files
```
