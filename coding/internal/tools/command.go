package tools

import (
	"path"
	"regexp"
	"strings"
)

// readOnlyCommands are shell programs that only inspect state and cannot modify
// the workspace. The list is deliberately conservative: a program is included
// only when its common invocations have no writing mode. Tools that can write
// with a flag (sed -i, awk with system(), find -delete) are excluded, as are
// command runners that execute an arbitrary argument program (env, xargs, sudo,
// timeout), so they still require confirmation.
var readOnlyCommands = map[string]bool{
	"ls": true, "pwd": true, "echo": true, "cat": true, "head": true,
	"tail": true, "wc": true, "grep": true, "egrep": true, "fgrep": true,
	"rg": true, "which": true, "type": true, "file": true, "stat": true,
	"du": true, "df": true, "tree": true, "printenv": true,
	"date": true, "whoami": true, "hostname": true, "uname": true,
	"basename": true, "dirname": true, "realpath": true, "readlink": true,
	"cmp": true, "sort": true, "uniq": true, "cut": true, "id": true,
	"true": true, "false": true, "test": true, "seq": true, "column": true,
}

// gitReadOnlySubcommands are git subcommands that only read repository state.
var gitReadOnlySubcommands = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true, "branch": true,
	"remote": true, "rev-parse": true, "describe": true, "blame": true,
	"tag": true, "config": true, "ls-files": true, "ls-remote": true,
	"shortlog": true, "reflog": true, "cat-file": true, "whatchanged": true,
}

// goReadOnlySubcommands are go subcommands that do not modify source files.
var goReadOnlySubcommands = map[string]bool{
	"version": true, "env": true, "doc": true, "list": true,
}

// segmentSplitter breaks a command line on the shell operators that separate
// independent commands. A read-only command may chain several read-only
// segments; every segment must qualify.
var segmentSplitter = regexp.MustCompile(`\|\||&&|;|\||&`)

// commandIsReadOnly reports whether a shell command only inspects state. It is
// conservative: any construct it cannot prove safe — redirection, command
// substitution, or an unrecognized program — makes it return false so the call
// falls through to confirmation.
func commandIsReadOnly(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	// Command substitution can hide writes inside an otherwise read-only command.
	if strings.Contains(command, "`") || strings.Contains(command, "$(") {
		return false
	}
	for _, segment := range segmentSplitter.Split(command, -1) {
		if !segmentIsReadOnly(segment) {
			return false
		}
	}
	return true
}

// segmentIsReadOnly classifies one command segment (no shell operators).
func segmentIsReadOnly(segment string) bool {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return true // an empty segment (e.g. a trailing separator) writes nothing
	}
	// Any redirection writes to a file or fd.
	if strings.ContainsAny(segment, "<>") {
		return false
	}

	fields := strings.Fields(segment)
	// Skip leading NAME=value environment assignments.
	i := 0
	for i < len(fields) && isEnvAssignment(fields[i]) {
		i++
	}
	if i >= len(fields) {
		return false
	}
	prog := path.Base(fields[i])
	rest := fields[i+1:]

	switch prog {
	case "git":
		return subcommandIn(rest, gitReadOnlySubcommands)
	case "go":
		return subcommandIn(rest, goReadOnlySubcommands)
	default:
		return readOnlyCommands[prog]
	}
}

// subcommandIn reports whether the first non-flag argument is in the allowed
// set. A bare command with no subcommand (for example "git") is not read-only.
func subcommandIn(args []string, allowed map[string]bool) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		return allowed[a]
	}
	return false
}

func isEnvAssignment(token string) bool {
	eq := strings.IndexByte(token, '=')
	if eq <= 0 {
		return false
	}
	for _, r := range token[:eq] {
		if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
