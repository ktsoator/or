//go:build windows

package tools

import "testing"

func TestWindowsPathToBash(t *testing.T) {
	tests := map[string]string{
		`C:\Users\me\repo`: `/c/Users/me/repo`,
		`D:\`:              `/d/`,
		`C:`:               `/c/`,
		`C:relative`:       `C:relative`,
		`\\server\share`:   `//server/share`,
	}
	for input, want := range tests {
		if got := windowsPathToBash(input); got != want {
			t.Errorf("windowsPathToBash(%q) = %q, want %q", input, got, want)
		}
	}
}
