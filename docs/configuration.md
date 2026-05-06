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
- `preferences.json`
- `sessions/`
- `usage.jsonl`

Do not commit these files.

## Hooks

Whale supports external shell hooks via JSON config files:

- project: `./.whale/settings.json`
- global: `~/.whale/settings.json`

Whale loads project hooks before global hooks.

Supported events:

- `PreToolUse`
- `PostToolUse`
- `UserPromptSubmit`
- `Stop`

Example:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "match": "bash",
        "command": "echo 'blocked by policy' >&2; exit 2",
        "timeout": 5000
      }
    ]
  }
}
```

Treat hook files as untrusted input when reproducing another workspace, because hook commands can execute shell commands.

## Runtime notes

- `whale exec` and the interactive TUI use the same underlying tool loop.
- Normal approval and hook behavior still applies in headless mode.
- Reasoning effort can be overridden per run with:

```bash
whale --config model_reasoning_effort=max exec "Think carefully and propose a plan"
```
