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
./scripts/test-config-commands.sh      # p-013: config, show, merging, --local
./scripts/test-supporting-commands.sh  # p-014: assets, doctor, completion
```

Options:
- `-y, --yes` - Run without pausing between tests
- `-h, --help` - Show usage

### Edge Case Tests

```bash
./scripts/test-no-agents.sh     # Test behaviour with no agents configured
./scripts/test-single-agent.sh  # Test behaviour with single agent
```

## Development

```bash
./scripts/upsert-context        # Update context index files
./scripts/validate-assets.sh    # Validate CUE asset modules
```
