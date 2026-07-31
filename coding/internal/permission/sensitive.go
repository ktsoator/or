package permission

import (
	"path/filepath"
	"strings"
)

// SensitiveKind classifies a filesystem target that needs approval on its own
// merits, independent of where it sits. A secret inside the workspace is still
// a secret, so Location alone cannot decide these.
type SensitiveKind string

const (
	// NotSensitive is an ordinary file, judged only by Location.
	NotSensitive SensitiveKind = ""
	// SecretFile customarily holds credentials. Reading one puts its contents
	// into the model's context and the durable transcript, so it is never an
	// automatic workspace read.
	SecretFile SensitiveKind = "secret_file"
	// RepositoryInternals is Git's own state. Writing there changes what later
	// git commands do — hooks and config select programs to execute — so it is
	// not an ordinary workspace edit even when edits are enabled.
	RepositoryInternals SensitiveKind = "repository_internals"
)

// secretDirs are directories whose entire contents are credentials.
var secretDirs = map[string]bool{
	".ssh":    true,
	".aws":    true,
	".gnupg":  true,
	".gpg":    true,
	".kube":   true,
	".docker": true,
}

// secretNames are exact file names that hold credentials.
var secretNames = map[string]bool{
	".netrc":           true,
	"_netrc":           true,
	".npmrc":           true,
	".pypirc":          true,
	".git-credentials": true,
	"credentials":      true,
	"id_rsa":           true,
	"id_dsa":           true,
	"id_ecdsa":         true,
	"id_ed25519":       true,
}

// secretExts are file extensions that carry keys and certificates.
var secretExts = map[string]bool{
	".pem":      true,
	".key":      true,
	".p12":      true,
	".pfx":      true,
	".jks":      true,
	".keystore": true,
	".ppk":      true,
}

// placeholderEnvSuffixes mark the committed example files that accompany a real
// .env. They hold no secrets, and treating them as secrets would put an
// approval in front of an everyday read.
var placeholderEnvSuffixes = []string{".example", ".sample", ".template", ".dist"}

// ClassifySensitive reports whether path needs approval on its own merits. It
// matches on names alone: it never opens the file, so it is cheap enough to run
// on every resolved access and cannot be defeated by the file's contents.
//
// It is deliberately generous. The outcome of a match is one approval prompt,
// so a false positive costs a click while a false negative leaks a credential.
func ClassifySensitive(path string) SensitiveKind {
	if path == "" {
		return NotSensitive
	}
	cleaned := filepath.Clean(path)

	gitIndex := -1
	for index, component := range strings.Split(filepath.ToSlash(cleaned), "/") {
		if component == "" {
			continue
		}
		if secretDirs[strings.ToLower(component)] {
			return SecretFile
		}
		if component == ".git" && gitIndex < 0 {
			gitIndex = index
		}
	}

	base := strings.ToLower(filepath.Base(cleaned))
	if secretNames[base] || secretExts[strings.ToLower(filepath.Ext(base))] || isEnvFile(base) {
		return SecretFile
	}
	if gitIndex >= 0 {
		// A remote URL in .git/config may embed a token, so config reads are
		// treated as credential reads rather than as ordinary repository state.
		if base == "config" {
			return SecretFile
		}
		return RepositoryInternals
	}
	return NotSensitive
}

// isEnvFile reports whether base is a .env file holding real values, excluding
// the committed placeholder variants.
func isEnvFile(base string) bool {
	if base != ".env" && !strings.HasPrefix(base, ".env.") {
		return false
	}
	for _, suffix := range placeholderEnvSuffixes {
		if strings.HasSuffix(base, suffix) {
			return false
		}
	}
	return true
}
