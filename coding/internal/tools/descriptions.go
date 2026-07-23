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
- ALWAYS use grep for searching code. NEVER run grep, rg, or find through the bash tool — the dedicated tools are faster and preserve workspace-aware access checks.
- Returns matching file paths by default (mode "files"). Set mode to "content" for matching lines with line numbers.
- Narrow the search with path (a subdirectory) and glob (a filename filter such as "*.go").
- Results are capped; refine the pattern if output is truncated. Common vendored directories (.git, node_modules, and similar) are skipped.`,
	snippet: "grep — search file contents by regular expression",
	guidelines: []string{
		"Find code with `grep` and `glob`, never with `grep`/`rg`/`find` through `bash` — the dedicated tools are faster and preserve workspace-aware access checks.",
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
		"Find code with `grep` and `glob`, never with `grep`/`rg`/`find` through `bash` — the dedicated tools are faster and preserve workspace-aware access checks.",
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
- If edit reports that the file was not read or has changed, use read and then retry edit; do not switch to bash.
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
- Read an existing file before overwriting it; creating a new file does not require a prior read.
- If write reports that the file was not read or has changed, use read and then retry write; do not switch to bash.
- Prefer edit over write when changing part of an existing file; write replaces the whole file.
- Writes replace the destination atomically and preserve existing file permissions and symlinks.
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
- Do not use bash as a substitute for read, grep, glob, ls, edit, or write.
- Create or replace files with write, not echo or printf redirection, tee, or heredocs. Modify existing files with edit, not sed -i, awk, or perl.
- If edit or write requires a prior read, call read and retry the same tool.
- A non-zero exit code is reported as output, not as a failure, so you can react to it.
- Always set description to a short active-voice summary of the command (about 5-10 words); it is shown in the UI in place of the raw command.
- For a long-lived process that does not exit on its own — a dev server, a watcher, a database — set run_in_background instead of waiting for it. bash returns a shell id immediately; read its output with bash_output and stop it with kill_bash. Never wait on such a command in the foreground.`,
	snippet: "bash — run a shell command in the workspace",
	guidelines: []string{
		"Never bypass a `read`, `edit`, or `write` error with `bash`; satisfy the requested precondition and retry the same tool.",
		"Set `bash`'s `description` to a short active-voice summary of each command; it is what the UI shows instead of the raw command.",
		"Start long-lived processes (servers, watchers) with `bash` `run_in_background`, then inspect them with `bash_output` and stop them with `kill_bash`; never run them in the foreground.",
	},
}

var bashOutputText = toolText{
	description: `Read new output from a background shell started by bash with run_in_background.

Usage:
- shell_id is the id bash returned when the command was started.
- Each call returns only the output produced since the previous call, plus whether the shell is still running and, once finished, its exit code.
- Poll after starting a background server to confirm it came up, or to collect logs while other work proceeds.`,
	snippet: "bash_output — read new output from a background shell",
}

var openPreviewText = toolText{
	description: `Open a workspace HTML file or local web application in Coding's built-in Browser view.

Usage:
- For a static HTML page, pass its absolute workspace path directly. Workspace-relative paths and file:// URLs inside the workspace are also accepted. Do not start a server for static HTML.
- For an application that requires a runtime or dev server, url must be a complete http or https URL on localhost, 127.0.0.1, ::1, or a wildcard loopback listener such as 0.0.0.0.
- Start required long-lived development servers with bash run_in_background, then use bash_output to confirm the server is running before opening its URL.
- Use this when a web interface is ready for the user to inspect. Do not call it for API servers, test runners, or links on the public internet.
- title is optional and should be a short name for the page.`,
	snippet: "open_preview — open a workspace HTML file or running local web app in Coding's Browser view",
	guidelines: []string{
		"Preview static HTML by passing its absolute workspace path to `open_preview`; do not start a server unless the app requires a runtime.",
		"After starting a required local app server and confirming its URL, call `open_preview` so the user can inspect it instead of only printing the URL.",
	},
}

var killBashText = toolText{
	description: `Stop a background shell started by bash with run_in_background, terminating its whole process group.

Usage:
- shell_id is the id bash returned when the command was started.
- Stopping an already-finished shell is a no-op.
- Kill a background server or watcher once you are done with it so it does not keep holding its port.`,
	snippet: "kill_bash — stop a background shell",
}
