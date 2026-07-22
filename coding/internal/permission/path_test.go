package permission

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathResolverClassifiesWorkspaceBoundary(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "repo-other")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}

	resolver, err := NewPathResolver(workspace)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want Location
	}{
		{name: "workspace root", path: "", want: Workspace},
		{name: "missing workspace file", path: "new/child.txt", want: Workspace},
		{name: "parent traversal", path: "../repo-other/file.txt", want: OutsideWorkspace},
		{name: "absolute sibling with shared prefix", path: filepath.Join(outside, "file.txt"), want: OutsideWorkspace},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolver.Resolve(Access{Action: Read, Path: test.path})
			if got.Location != test.want {
				t.Fatalf("Resolve(%q).Location = %q, want %q (resolved %q, error %q)", test.path, got.Location, test.want, got.ResolvedPath, got.ResolutionError)
			}
		})
	}
}

func TestPathResolverFollowsSymlinksOutsideWorkspace(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}

	existingLink := filepath.Join(workspace, "existing-link")
	if err := os.Symlink(outside, existingLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	danglingLink := filepath.Join(workspace, "dangling-link")
	if err := os.Symlink(filepath.Join(outside, "missing"), danglingLink); err != nil {
		t.Fatal(err)
	}

	resolver, err := NewPathResolver(workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(existingLink, "file.txt"),
		filepath.Join(danglingLink, "file.txt"),
	} {
		got := resolver.Resolve(Access{Action: Write, Path: path})
		if got.Location != OutsideWorkspace {
			t.Fatalf("Resolve(%q).Location = %q, want %q (resolved %q, error %q)", path, got.Location, OutsideWorkspace, got.ResolvedPath, got.ResolutionError)
		}
	}
}
