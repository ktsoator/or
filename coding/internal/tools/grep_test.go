package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepSkipsFilesThatMayHoldCredentials(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("main.go", "const token = \"SEARCHTERM-in-source\"\n")
	write(".env", "API_TOKEN=SEARCHTERM-secret\n")
	write(".env.example", "API_TOKEN=SEARCHTERM-placeholder\n")
	write("certs/server.key", "SEARCHTERM-private-key\n")

	// grep reads whole files that no approval preflight ever named, so the
	// credentials must not come back in either mode.
	for _, mode := range []string{"files", "content"} {
		t.Run(mode, func(t *testing.T) {
			result, err := runGrep(context.Background(), root, grepArgs{
				Pattern: "SEARCHTERM",
				Mode:    mode,
			})
			if err != nil {
				t.Fatal(err)
			}
			out := resultText(t, result)

			if !strings.Contains(out, "main.go") {
				t.Fatalf("grep output lost an ordinary match:\n%s", out)
			}
			if !strings.Contains(out, ".env.example") {
				t.Fatalf("grep output lost the placeholder env file:\n%s", out)
			}
			for _, secret := range []string{"SEARCHTERM-secret", "SEARCHTERM-private-key", "server.key"} {
				if strings.Contains(out, secret) {
					t.Fatalf("grep output leaked %q:\n%s", secret, out)
				}
			}
			// The model is told why, so a skipped file does not read as an
			// empty search worth retrying through bash.
			if !strings.Contains(out, "may hold credentials") {
				t.Fatalf("grep output did not report the skipped files:\n%s", out)
			}
		})
	}
}

func TestGrepReportsSkippedFilesWhenNothingElseMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=only-here\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := runGrep(context.Background(), root, grepArgs{Pattern: "only-here"})
	if err != nil {
		t.Fatal(err)
	}
	out := resultText(t, result)
	if strings.Contains(out, "only-here") {
		t.Fatalf("grep output leaked the secret:\n%s", out)
	}
	if !strings.Contains(out, "No matches found.") || !strings.Contains(out, "may hold credentials") {
		t.Fatalf("grep output = %q, want no matches plus the skip notice", out)
	}
}
