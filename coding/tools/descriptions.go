package tools

// This file holds the model-facing text for every built-in tool — the schema
// description, the one-line system-prompt snippet, and the guideline bullets —
// separate from the execution code. Keeping the wording in one place keeps the
// cross-references that steer the model between tools (for example "search with
// grep, never grep via bash") consistent and easy to maintain.

// toolText groups a tool's model-facing text. description is sent in the tool
// schema; snippet is the one-liner for the system prompt's Available tools list;
// guidelines are bullets appended to the Guidelines section while the tool is
// active. Guidelines are de-duplicated across tools when the prompt is built.
type toolText struct {
	description string
	snippet     string
	guidelines  []string
}

var readText = toolText{
	description: `Read a text file from the workspace and return its contents with 1-based line numbers (cat -n format).

Usage:
- path may be absolute or relative to the workspace root.
- offset is 1-based and defaults to 1; limit defaults to 1000 lines and cannot exceed 2000.
- When more content is available, continue from the offset reported at the end of the result.
- Read a file before you edit it, so your edits match its current contents.
- Output is capped at complete line boundaries; an unusually long single line returns an error instead of partial content.`,
	snippet: "read — read a file's contents with line numbers",
	guidelines: []string{
		"Read a file with `read` before you `edit` it, so edits match its current contents.",
	},
}

var grepText = toolText{
	description: `Search file contents across the workspace with a regular expression (Go regexp syntax). Built for code search.

Usage:
- ALWAYS use grep for searching code. NEVER run grep, rg, or find through the bash tool — grep is faster and needs no confirmation.
- Returns matching file paths by default (mode "files"). Set mode to "content" for matching lines with line numbers.
- Narrow the search with path (a subdirectory) and glob (a filename filter such as "*.go").
- Results are capped; refine the pattern if output is truncated. Common vendored directories (.git, node_modules, and similar) are skipped.`,
	snippet: "grep — search file contents by regular expression",
	guidelines: []string{
		"Find code with `grep` and `glob`, never with `grep`/`rg`/`find` through `bash` — the dedicated tools are faster and need no confirmation.",
	},
}

var globText = toolText{
	description: `Find files by name using a glob pattern, for any size of codebase.

Usage:
- Patterns support * (within a path segment), ? (one character), and ** (any number of segments), e.g. "**/*.go" or "cmd/**/main.go".
- Returns paths sorted by most-recently-modified first, so the files you are likely working on appear near the top.
- Use glob to find files by name; use grep to search their contents. Do not use find through the bash tool.`,
	snippet: "glob — find files by name pattern",
	guidelines: []string{
		"Find code with `grep` and `glob`, never with `grep`/`rg`/`find` through `bash` — the dedicated tools are faster and need no confirmation.",
	},
}

var lsText = toolText{
	description: `List the entries of a directory in the workspace, directories first.

Usage:
- path defaults to the workspace root. Common vendored directories are still listed but their contents are not traversed.
- Use glob to find files by pattern or grep to search contents; ls is for a quick look at one directory.`,
	snippet: "ls — list a directory's entries",
}

var editText = toolText{
	description: `Perform an exact string replacement in a file.

Usage:
- Read the file with the read tool first, so old_string matches its current contents exactly.
- old_string must match exactly one place in the file unless replace_all is set; include enough surrounding context (usually a few adjacent lines) to make it unique.
- Preserve exact indentation — match the text as it appears after the line-number prefix in read output, never including that prefix.
- Prefer edit over write when changing part of an existing file.`,
	snippet: "edit — replace an exact string in a file",
	guidelines: []string{
		"Include enough context in old_string to match exactly one location, or set replace_all.",
	},
}

var writeText = toolText{
	description: `Write a file in full, creating it or overwriting its contents; parent directories are created as needed.

Usage:
- Prefer edit over write when changing part of an existing file; write replaces the whole file.
- Provide the complete intended contents, not a fragment.`,
	snippet: "write — create or overwrite a file in full",
	guidelines: []string{
		"Prefer `edit` over `write` when modifying an existing file.",
	},
}

var bashText = toolText{
	description: `Run a bash command in the workspace directory and return its combined output and exit code.

Usage:
- Use bash for building, testing, running programs, and version control.
- Prefer the dedicated tools — read, grep, glob, ls, edit, write — over shell equivalents like cat, grep, find, or sed. They are faster and need no confirmation.
- A non-zero exit code is reported as output, not as a failure, so you can react to it.`,
	snippet: "bash — run a shell command in the workspace",
	guidelines: []string{
		"Use `bash` for building, testing, and running code — not for reading or searching, which the dedicated tools do without confirmation.",
	},
}
