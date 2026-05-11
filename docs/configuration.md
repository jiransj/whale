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

Project config overrides global config. CLI flags override both.

Example:

```toml
model = "deepseek-v4-flash"
reasoning_effort = "high"
thinking_enabled = true

approval_mode = "on-request"
allow_prefixes = ["git status", "go test"]
deny_prefixes = ["rm -rf"]
budget_warning_usd = 1.0

mcp_config = "~/.whale/mcp.json"

[compact]
auto = true
threshold = 0.7

[memory]
enabled = true
max_chars = 12000
file_order = ["AGENTS.md", ".claude/instructions.md", "CLAUDE.md"]
```

## Hooks

Whale supports external shell hooks in `config.toml`:

- project: `./.whale/config.toml`
- global: `~/.whale/config.toml`

Whale loads project hooks before global hooks.

Supported events:

- `PreToolUse`
- `PostToolUse`
- `UserPromptSubmit`
- `Stop`

Example:

```toml
[[hooks.PreToolUse]]
match = "bash"
command = "echo 'blocked by policy' >&2; exit 2"
timeout = 5000
```

`timeout` is in milliseconds. If omitted, `PreToolUse` and `UserPromptSubmit`
default to 5000ms, while `PostToolUse` and `Stop` default to 30000ms.

Treat hook files as untrusted input when reproducing another workspace, because hook commands can execute shell commands.

## Migrating old config

Older Whale builds used `preferences.json` and `settings.json`. New builds no longer read those files.

Run this once to convert them into `config.toml`:

```bash
whale migrate-config
```

## Runtime notes

- `whale exec` and the interactive TUI use the same underlying tool loop.
- Normal approval and hook behavior still applies in headless mode.
- Reasoning effort and thinking can be overridden per run with:

```bash
whale --config model_reasoning_effort=max exec "Think carefully and propose a plan"
whale --config thinking_enabled=false exec "Answer without extended thinking"
```
