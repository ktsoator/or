# Or CLI Collection

This directory is a lightweight collection of command-line tools that are
useful for software development and coding agents. It helps people discover
good tools and understand why they are worth trying.

This is not a package manager or installer. Each entry is a short guide with an
official source, supported platforms, common examples, and links to upstream
installation documentation.

## Included tools

| Tool | Command | Recommended for |
|---|---|---|---|
| [ripgrep](tools/ripgrep/README.md) | `rg` | Fast recursive text and code search |

## Layout

```text
cli/
|-- README.md
|-- README.zh.md
`-- tools/
    `-- <id>/
        `-- README.md
```

## Contribution requirements

- Keep one tool in each immediate child directory.
- Explain what the tool does, why it is recommended, and when it is useful.
- Include the command name, supported platforms, common examples, official
  repository, documentation, installation guide, and license.
- Prefer actively maintained tools with a clear advantage over common
  alternatives.
- Keep examples short, safe, and representative. Call out surprising behavior
  or flags that substantially expand access or execute other programs.
- Link to upstream installation instructions instead of maintaining a complete
  package-manager matrix in this repository.
- Do not include credentials, private endpoints, or machine-specific setup.
