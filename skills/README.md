# Or Skills

This directory contains Agent Skills curated and distributed by the Or project.
Every child directory is a self-contained Skill package that follows the open
[Agent Skills specification](https://agentskills.io/specification).

This is a source collection, not a runtime discovery directory. Or only loads
installed Skills from the standard locations:

- User Skills: `~/.agents/skills/<name>/SKILL.md`
- Workspace Skills: `<workspace>/.agents/skills/<name>/SKILL.md`

Until Or provides an installer, copy a selected directory from this collection
to one of those locations.

## Layout

```text
skills/
`-- <name>/
    |-- SKILL.md
    |-- scripts/       # optional
    |-- references/    # optional
    `-- assets/        # optional
```

## Contribution requirements

- Keep each Skill self-contained in one immediate child directory.
- Make the directory name exactly match the `name` in `SKILL.md` frontmatter.
- Use only fields defined by the Agent Skills specification. Put version data
  under `metadata.version`; do not add a top-level `version` field.
- Do not add Or-specific compatibility fields, prompt-template substitution, or
  alternate runtime paths.
- Preserve upstream attribution and licensing for imported Skills. Only include
  content that the project may redistribute.
- Review bundled scripts before inclusion. Reading a Skill never grants its
  scripts permission to execute.
- Do not commit credentials, generated output, dependency caches, or local
  configuration.

Project-specific Skills that are intended only to guide work on this repository
belong in `.agents/skills/`, not in this distribution collection.
