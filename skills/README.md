# Or Skills

This directory contains Agent Skills curated and distributed by the Or project.
Every child directory is a self-contained Skill package that follows the open
[Agent Skills specification](https://agentskills.io/specification).

## Included Skills

| Skill | Upstream revision | License |
|---|---|---|
| [`find-skills`](find-skills/SKILL.md) | [`vercel-labs/skills@a4d243c`](https://github.com/vercel-labs/skills/tree/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/skills/find-skills) | [MIT](find-skills/LICENSE) |
| [`officecli`](officecli/SKILL.md) | [`iOfficeAI/OfficeCLI@459b1a4`](https://github.com/iOfficeAI/OfficeCLI/blob/459b1a473faf33f2f52e697ac6d265a3f67b176a/SKILL.md) | [Apache-2.0](officecli/LICENSE) |

The Skill files match the recorded upstream revisions without modification.

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
- Do not add Or-specific compatibility fields, argument substitution, or
  alternate runtime paths.
- Preserve upstream attribution and licensing for imported Skills. Only include
  content that the project may redistribute.
- Review bundled scripts before inclusion. Reading a Skill never grants its
  scripts permission to execute.
- Do not commit credentials, generated output, dependency caches, or local
  configuration.

Project-specific Skills that are intended only to guide work on this repository
belong in `.agents/skills/`, not in this distribution collection.
