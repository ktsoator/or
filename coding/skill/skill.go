// Package skill loads file-backed skills and exposes them to a coding agent.
//
// A skill is a directory named after the skill, containing a SKILL.md file with
// YAML frontmatter (name and description) and a Markdown body. Only the name and
// description enter the model's context up front; the body is injected on demand
// when the model calls the skill tool (see Registry.Tool). This keeps the
// initial context small while letting the model pull full instructions for the
// task at hand.
//
// Skills are discovered from two roots: a user root that applies everywhere and
// a project root scoped to one workspace. A project skill overrides a user skill
// of the same name. Loading is decoupled from the agent: callers resolve the two
// roots and pass them to Load, then hand the resulting skills to the session.
package skill

// Source identifies where a skill was loaded from.
type Source string

const (
	// SourceUser is the user-level root that applies to every workspace.
	SourceUser Source = "user"
	// SourceProject is the workspace-scoped root, which overrides user skills of
	// the same name.
	SourceProject Source = "project"
)

// Skill is one loaded skill: the metadata advertised to the model plus the body
// injected when the skill is invoked.
type Skill struct {
	// Name is the stable identifier, equal to the skill's directory name. It is
	// used for lookup and in the model-visible listing.
	Name string
	// Description is the model-visible note on when to use the skill. Required.
	Description string
	// Content is the SKILL.md body, injected verbatim (after placeholder
	// expansion) when the skill is invoked. It is not part of the initial context.
	Content string
	// Dir is the absolute path to the skill's directory, exposed to Content via
	// the ${OR_SKILL_DIR} placeholder so bundled scripts and references resolve.
	Dir string
	// Path is the absolute path to the SKILL.md file, for diagnostics.
	Path string
	// Source records which root the skill came from.
	Source Source
}

// Diagnostic reports a skill that could not be loaded, so the caller can surface
// the problem without failing the whole load.
type Diagnostic struct {
	// Path is the SKILL.md (or directory) the problem concerns.
	Path string
	// Message explains why the skill was skipped.
	Message string
}
