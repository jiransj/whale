# Configuration

## Credentials

Whale uses the DeepSeek API.

- `whale setup` saves a key to `~/.whale/credentials.json`
- `DEEPSEEK_API_KEY` takes precedence over the saved credential

Example:

```bash
whale setup
DEEPSEEK_API_KEY=... whale
```

## Local state

Whale stores local state under `~/.whale/`, including:

- `credentials.json`
- `config.toml`
- `mcp.json`
- `sessions/`
- `usage.jsonl`

Do not commit these files.

## Config files

Whale reads user-editable configuration from:

- global: `~/.whale/config.toml`
- project: `./.whale/config.toml`

Project config overrides global config. The `--model` CLI flag can override
the configured model for one run.

Example:

```toml
model = "deepseek-v4-flash"
reasoning_effort = "high"
thinking_enabled = true

[permissions]
mode = "on-request"
allow_shell_prefixes = ["git status", "go test"]
deny_shell_prefixes = ["rm -rf"]

[budget]
session_limit_usd = 1.0

[mcp]
config_path = "~/.whale/mcp.json"

[context]
auto_compact = true
compact_threshold = 0.85
model_context_window = 128000

[project_doc]
enabled = true
max_bytes = 8000
fallback_filenames = ["AGENTS.md", ".claude/instructions.md", "CLAUDE.md"]
```

## Migrating old config

Whale v0.1.8 and earlier used `preferences.json` and `settings.json`. New
builds no longer read those files.

Run this once only if you used Whale v0.1.8 or earlier and still have those
legacy files:

```bash
whale migrate-config
```

If you started with Whale v0.1.9 or newer, you do not need this command.

## Runtime notes

- `whale exec` and the interactive TUI use the same underlying tool loop.
- Normal approval behavior still applies in headless mode.
- Reasoning effort and thinking are configured in `config.toml`.
