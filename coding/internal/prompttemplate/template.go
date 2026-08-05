// Package prompttemplate loads reusable, file-backed prompt macros.
package prompttemplate

// Source identifies where a prompt template was loaded from.
type Source string

const (
	SourceUser    Source = "user"
	SourceProject Source = "project"
)

// Template is one Markdown prompt template and its discovery metadata.
type Template struct {
	Name          string
	Description   string
	Descriptions  map[string]string
	ArgumentHint  string
	ArgumentHints map[string]string
	Content       string
	Path          string
	Source        Source
}

// Diagnostic reports a template file that could not be loaded.
type Diagnostic struct {
	Path    string
	Message string
}
