# Context compaction design

Status: proposal. This document describes a planned `github.com/ktsoator/or/agent/harness`
package and one supporting change to the `agent` core. It is not yet implemented.

## Scope

The `agent` core runs the tool loop but leaves context management to the caller:
when a conversation outgrows the model's context window, the caller decides what
to drop or summarize. This is the first piece of a harness layer that supplies
those policies.

Phase one is **context compaction only**: when the transcript approaches the
model's context window, summarize the older turns and keep the recent ones, so a
long-running agent can continue past the window. Session persistence, skills, and
an execution environment are later, separate work.

## Where it lives

A new subpackage `agent/harness` holds the compaction logic and a thin `Harness`
type. It depends on `agent` and `llm`; neither depends on it. Keeping it in a
subpackage rather than in `agent` leaves the core import lean for callers that
only want the engine.

```
llm  ←  agent  ←  agent/harness
```

## One core change: TransformContext gains a context

Compaction summarizes old turns by calling a model, which needs a
`context.Context` and can fail. The core's existing seam does not allow either:

```go
// today
TransformContext func([]AgentMessage) []AgentMessage
```

It evolves to carry a context and report an error:

```go
// proposed
TransformContext func(context.Context, []AgentMessage) ([]AgentMessage, error)
```

The engine passes its run context and treats an error as "no transformation":
it keeps the pre-transform messages and continues, preserving the rule that a
failing hook never breaks the run.

```go
messages := current.Messages
if e.cfg.TransformContext != nil {
	if transformed, err := e.cfg.TransformContext(e.ctx, messages); err == nil {
		messages = transformed
	}
}
```

This is a breaking change to one hook signature. The project is pre-1.0, and
`TransformContext` has no callers in the repository today, so the change is
contained. It touches `config.go`, `agent.go`, and `loop.go`.

## Compaction algorithm

Settings control when and how much to compact:

```go
type Settings struct {
	// Threshold is the fraction of the context window that triggers compaction
	// (for example 0.8). Zero uses a default.
	Threshold float64
	// KeepRecentTurns is how many trailing turns to leave untouched.
	KeepRecentTurns int
}
```

The compaction step, run before each request through TransformContext:

1. **Estimate** the transcript's token use. Phase one uses a cheap heuristic
   (characters / 4); the last assistant response's reported usage can refine it
   later.
2. **Check the threshold.** If estimated tokens are under
   `Threshold × contextWindow`, return the messages unchanged.
3. **Find a cut point** at a turn boundary, keeping the last `KeepRecentTurns`
   turns. Compaction never splits a turn, so a tool call stays with its result.
4. **Summarize** the messages before the cut with a single `llm.Complete` call
   under a fixed summarization system prompt, producing a structured summary.
5. **Replace** the cut prefix with one summary message, followed by the kept
   recent turns:

   ```
   [ summary message ] + messages[cut:]
   ```

The summary message is a synthesized user message carrying the summary text, so
it projects to the model like any other message. Its exact form is an open
question below.

Failure in step 4 (the model call) returns an error from TransformContext, which
the engine treats as "skip compaction this turn" — the run proceeds on the full
transcript rather than failing.

## The Harness type

`Harness` wraps an `agent.Agent` and wires compaction into its TransformContext.
It embeds `*agent.Agent`, so `Prompt`, `Subscribe`, `Steer`, and the rest are
inherited unchanged; the only added behavior is automatic compaction.

```go
type Harness struct {
	*agent.Agent
	model    llm.Model // model used to summarize; defaults to the agent's model
	settings Settings
}

type Options struct {
	agent.Options
	// Compaction configures when and how much to compact. The zero value uses
	// defaults.
	Compaction Settings
	// SummaryModel summarizes old turns. The zero value uses Options.Model.
	SummaryModel llm.Model
}

func New(opts Options) *Harness {
	h := &Harness{settings: opts.Compaction, model: resolveSummaryModel(opts)}
	inner := opts.Options
	inner.TransformContext = h.compact // inject compaction into the core hook
	h.Agent = agent.New(inner)
	return h
}
```

A caller uses it exactly like an `Agent`, and gets automatic compaction:

```go
assistant := harness.New(harness.Options{
	Options: agent.Options{Model: model, Tools: tools},
})
assistant.Prompt(ctx, "...") // compacts automatically when the window fills
```

The summary model defaults to the agent's model and can be overridden with
`Options.SummaryModel` — for example, a cheaper model for summarization.

## Data flow

```
each request →  engine calls TransformContext  →  Harness.compact
                                                     │
                                  under threshold ───┴─── return unchanged
                                  over threshold  ──────  summarize old turns,
                                                          return summary + recent
```

The empty `TransformContext` seam the core left open is now filled by the
Harness; the engine is otherwise unchanged.

## Open questions

- **Summary message form.** A user message is the simplest carrier, but the
  summary is not user input. Alternatives: a dedicated marker message type, or
  folding the summary into the system prompt. Phase one uses a user message and
  may revisit this.
- **Token estimation.** The characters/4 heuristic is rough. Using the last
  response's reported usage (input + output) is more accurate but only available
  after a turn. Phase one ships the heuristic.
- **Explicit compaction.** Whether to also expose `Harness.Compact(ctx)` for
  callers that want to compact on demand rather than only automatically.
