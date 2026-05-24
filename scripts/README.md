# Scripts

Development and testing scripts for the `start` CLI.

## Testing

### Unit Tests

```bash
./scripts/invoke-tests          # Run Go unit tests
```

### CLI End-to-End Tests

Manual CLI testing scripts from the p-012, p-013, p-014 testing projects.

```bash
./scripts/test-core-commands.sh        # p-012: start, prompt, task, global flags
./scripts/test-config-commands.sh      # p-013: config, describe, merging, --local
./scripts/test-supporting-commands.sh  # p-014: modules, doctor, completion
```

Options:
- `-y, --yes` - Run without pausing between tests
- `-h, --help` - Show usage

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
