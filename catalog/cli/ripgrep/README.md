# ripgrep

ripgrep (`rg`) recursively searches text and source trees. It respects
`.gitignore`, `.ignore`, and `.rgignore` rules by default and skips hidden and
binary files unless the caller opts into searching them.

- Upstream: <https://github.com/BurntSushi/ripgrep>
- Documentation: <https://github.com/BurntSushi/ripgrep/blob/master/GUIDE.md>
- Installation: <https://github.com/BurntSushi/ripgrep#installation>
- Latest reviewed release: [`15.2.0`](https://github.com/BurntSushi/ripgrep/releases/tag/15.2.0)
- License: MIT or Unlicense
- Executable: `rg`
- Platforms: macOS, Linux, Windows

## Why it belongs in the catalog

Source search is a common coding-agent operation. ripgrep is fast, works on
macOS, Linux, and Windows, produces predictable line-oriented output, and is
available through established package managers on each platform.

## Examples

```sh
# Search the current directory
rg "MCPServers"

# Search only TypeScript files
rg "MCPServer" coding/client/src -g '*.ts' -g '*.tsx'

# List files that would be searched
rg --files

# Include hidden files while still respecting ignore rules
rg --hidden "TODO"
```

## Useful behavior to know

- Normal searches read files under the requested paths and write results only
  to standard output.
- Ignore files affect which paths are searched. Flags such as `--hidden`,
  `--no-ignore`, and `-uuu` can deliberately expand the readable scope.
- ripgrep does not use the network during normal operation.
- The optional `--pre` feature can start an arbitrary external preprocessor.
  Review that command before using it.
- A user-defined ripgrep configuration can add arguments implicitly through
  `RIPGREP_CONFIG_PATH`; inspect that file when observed behavior differs from
  the displayed command.
