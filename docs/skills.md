# Skills

Whale supports local Agent Skills: reusable instruction folders that teach the
agent a specific workflow, domain, or tool pattern.

A skill is a directory containing a `SKILL.md` file. Whale keeps only skill names
and descriptions in the model-visible skill index. The full `SKILL.md` body is
loaded only when a skill is invoked or clearly matches the task.

## Skill Locations

Whale discovers skills from these directories:

- `.whale/skills`
- `.agents/skills`
- `~/.whale/skills`
- `~/.agents/skills`

Workspace skills are discovered before user-global skills, so a project can
override a global skill with the same name.

## Creating a Skill

Each skill lives in a directory named after the skill:

```text
~/.whale/skills/my-skill/
└── SKILL.md
```

`SKILL.md` must start with frontmatter containing `name` and `description`:

```markdown
---
name: my-skill
description: Use this when Whale should follow my custom workflow.
---

# My Skill

Instructions for Whale go here.
```

The skill name must use letters, digits, and hyphens, and the directory name
must match the `name` field.

## Invoking Skills

Run `/skills` in the TUI to list discovered skills.

Invoke a skill with a leading `$skill-name` mention:

```text
$my-skill apply this workflow to the current task
```

Whale stores the original `$my-skill ...` message as the visible user turn and
injects the loaded skill instructions as hidden context for that turn.

The model can also use the read-only `load_skill` tool when the task clearly
matches a discovered skill. This lets Whale load global skills without relaxing
the workspace boundary on `read_file`.

## Current Limitations

Whale's first skill implementation is instruction-only.

It does not currently provide:

- skill install, update, or uninstall commands
- custom `skills_paths` configuration
- disabled-skill configuration
- script execution or trust management
- dependency handling for MCP servers, environment variables, or bundled tools

Put skills directly in one of the discovery directories above.
