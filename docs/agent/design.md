# Agent package design

Status: proposal. This document describes a planned `github.com/ktsoator/or/agent`
package and is not yet implemented. It records the design agreed before any code
is written so the implementation has a fixed target.

## Scope

The `llm` package is a stateless translation layer: it decides what to send for
one request and how to read the streamed response, and leaves history storage,
context compaction, and the tool-call loop to the caller. Every application that
calls a tool today rewrites the same loop — stream a turn, collect tool calls,
execute them, append results, ask again — as the `example/llm/tool` command
shows in full.

The `agent` package moves that loop into a reusable engine. Its first phase is
deliberately small:

- An **engine**: a pure function that drives the tool-call loop and emits events.
- A thin **stateful shell**: an `Agent` that owns the transcript and exposes
  prompt, subscribe, steer, follow-up, and abort.

It does **not** include context compaction, session persistence, skills, or an
execution environment (filesystem and shell access). Those are separate concerns
and belong in a future higher-level package, mirroring how the reference
implementation splits `pi-agent-core` from `pi-coding-agent`. The engine leaves a
hook (`TransformContext`) where compaction will later attach, but ships no
compaction of its own.

## Relationship to `llm`

The `agent` package is built on `llm` and sits beside it; `llm` has no knowledge
of `agent`. The agent operates on its own message type throughout a run and
converts to `llm.Message` values only at the request boundary. It calls
`llm.Stream` for each turn, consumes the `llm.Event` channel, and relies on the
existing contract that a stream never fails by panicking: failures arrive as an
`EventError` carrying an `AssistantMessage` whose `StopReason` is
`StopReasonError` or `StopReasonAborted`.

Because `agent` does not modify `llm`, the `llm` message types cannot implement
an interface declared in `agent`. The message model below resolves this with a
small adapter rather than by adding agent-aware methods to `llm`.

## Principles

- **Stateless engine, optional state.** The loop is a function with no retained
  state; it takes a starting context and configuration, emits events, and returns
  the messages it appended. The `Agent` type is a separate, optional shell that
  adds mutable state, subscription, and queues on top.
- **Hooks are function fields.** Extension points live as `func` fields on a
  config struct, matching the `OnRequest` and `OnResponse` hooks already on
  `llm.StreamOptions`. The zero value of each is "no hook"; a zero config is a
  valid, plain tool loop.
- **Errors travel as messages.** The engine never returns a Go error for a model
  or tool failure mid-run. A failed turn ends with an assistant message whose
  stop reason is error or aborted, and the loop stops cleanly.
- **`llm` stays unaware of `agent`.** No type or import in `llm` references the
  agent layer.

## Message model

A run operates on `AgentMessage`, the union of standard LLM messages and any
UI-only messages an application wants to keep in the transcript (notifications,
artifacts, status entries). UI-only messages take part in history and event
emission but are filtered out before the model sees them.

```go
// AgentMessage is any entry that can appear in an agent transcript.
type AgentMessage interface {
	isAgentMessage()
}

// FromLLM adapts a standard llm.Message into an AgentMessage. This is the common
// path for user, assistant, and tool-result messages.
func FromLLM(m llm.Message) AgentMessage

// Custom is embedded by an application's own message types so they satisfy
// AgentMessage without depending on the interface's unexported marker.
type Custom struct{}
```

Standard messages are wrapped with `FromLLM` rather than implementing the
interface directly, because `agent` cannot add methods to types owned by `llm`.
Application message types embed `Custom` to join the union.

Projection to the wire is a configurable batch step, so an application can do
cross-message work (for example, fold several notifications into one system note)
rather than mapping each message in isolation:

```go
// ConvertToLLM projects the transcript into llm.Message values for one request.
// The default unwraps FromLLM messages and drops everything else.
ConvertToLLM func(messages []AgentMessage) []llm.Message
```

## Events

The engine reports progress on a channel, the same shape `llm.Stream` already
uses, so consumers stay in one idiom.

```go
type AgentEventType string

const (
	AgentStart   AgentEventType = "agent_start"
	AgentEnd     AgentEventType = "agent_end"   // carries the appended messages
	TurnStart    AgentEventType = "turn_start"
	TurnEnd      AgentEventType = "turn_end"
	MessageStart AgentEventType = "message_start"
	MessageUpdate AgentEventType = "message_update" // carries the llm.Event
	MessageEnd   AgentEventType = "message_end"
	ToolStart    AgentEventType = "tool_execution_start"
	ToolUpdate   AgentEventType = "tool_execution_update"
	ToolEnd      AgentEventType = "tool_execution_end"
)

type AgentEvent struct {
	Type     AgentEventType
	Message  AgentMessage
	LLMEvent *llm.Event // set on MessageUpdate, passing through the underlying event
	ToolCallID string
	ToolName   string
	Args       any
	Result     any
	IsError    bool
	Messages   []AgentMessage // set on AgentEnd: the messages this run appended
}
```

A turn is one assistant response plus any tool calls and results it triggers.
`AgentEnd` is the final event of a run.

## Tools

```go
// ToolResult is what a tool returns to the model, with optional structured
// details for logging or UI and an optional early-termination hint.
type ToolResult struct {
	Content   []llm.ToolResultContent
	Details   any
	Terminate bool
}

type ExecutionMode string

const (
	ExecutionParallel   ExecutionMode = "parallel"   // default
	ExecutionSequential ExecutionMode = "sequential"
)

type AgentTool struct {
	Definition    llm.ToolDefinition
	Execute       func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(ToolResult)) (ToolResult, error)
	ExecutionMode ExecutionMode // per-tool override of the loop default
}
```

A tool reports failure by returning a Go error; the engine turns that into an
error tool result and continues, so one failing tool does not abort the run.
`onUpdate` streams partial results and is valid only for the duration of one
`Execute` call.

Tool batches run in parallel by default. A batch runs sequentially when the loop
is configured for it or when any tool in the batch declares
`ExecutionSequential`. A batch stops the agent early only when **every** result
in it sets `Terminate`.

## Loop configuration

All extension points are function fields. The zero value of the struct, given a
model and `ConvertToLLM`, is a plain tool loop with no interception.

```go
type LoopConfig struct {
	Model         llm.Model
	StreamOptions llm.StreamOptions
	ConvertToLLM  func([]AgentMessage) []llm.Message
	ToolExecution ExecutionMode

	// BeforeToolCall runs after arguments validate and before execution.
	// Returning block=true skips the tool; reason becomes the error result text.
	BeforeToolCall func(BeforeToolCallCtx) (block bool, reason string)

	// AfterToolCall runs after a tool finishes. A non-nil return overrides the
	// executed result field by field; nil keeps it unchanged.
	AfterToolCall func(AfterToolCallCtx) *ToolResult

	// ShouldStopAfterTurn requests a graceful stop after the current turn,
	// before another model request starts.
	ShouldStopAfterTurn func(TurnCtx) bool

	// PrepareNextTurn may replace the model, thinking level, or context for the
	// next turn. Returning nil keeps the current settings.
	PrepareNextTurn func(TurnCtx) *TurnUpdate

	// TransformContext adjusts the transcript before projection. It is the
	// attachment point for context compaction; phase one ships no default.
	TransformContext func([]AgentMessage) []AgentMessage

	// GetSteeringMessages is polled after each turn's tool calls finish, to
	// inject messages mid-run. GetFollowUpMessages is polled when the agent
	// would otherwise stop, to continue it.
	GetSteeringMessages func() []AgentMessage
	GetFollowUpMessages func() []AgentMessage
}
```

`BeforeToolCall` returns `(block, reason)` rather than a result struct because
that is lighter in Go, and `AfterToolCall` returns `*ToolResult` so nil cleanly
means "no override."

## Engine

```go
// RunLoop drives a complete tool-call loop and returns a channel of events. The
// final AgentEnd event carries the messages the run appended to the transcript.
func RunLoop(ctx context.Context, prompts []AgentMessage, base Context, cfg LoopConfig) <-chan AgentEvent
```

The loop has two nested levels. The inner level processes the current turn:
drain any pending steering messages, stream one assistant response, execute its
tool calls, append results, then consult `PrepareNextTurn` and
`ShouldStopAfterTurn`. The outer level, reached when the inner level has no more
tool calls and no steering messages, polls `GetFollowUpMessages`; if any arrive
it re-enters the inner level, otherwise the run ends.

## Stateful agent

```go
type Agent struct{ /* messages, tools, model, queues, subscribers */ }

func New(opts Options) *Agent

func (a *Agent) Prompt(ctx context.Context, input any) error // text or messages
func (a *Agent) Subscribe(fn func(AgentEvent)) (unsubscribe func())
func (a *Agent) Steer(m AgentMessage)
func (a *Agent) FollowUp(m AgentMessage)
func (a *Agent) Abort()
func (a *Agent) Snapshot() State // read-only view of state
```

`Agent` wraps `RunLoop` with the retained transcript, a subscriber set fed from
the event channel, and the steering and follow-up queues backing
`GetSteeringMessages` and `GetFollowUpMessages`.

## Model switching within a run

`PrepareNextTurn` may return a different model for the next turn. Because the
engine projects the transcript through `llm`'s `TransformMessages` before every
request, switching models mid-run carries the existing history across the change:
unsupported images are downgraded, reasoning signatures are dropped or preserved
per model, and tool-call identifiers are normalized — including across a protocol
change. A run can therefore draft with an inexpensive model and review with a
stronger one, even when the two speak different wire protocols, without the
caller rebuilding any history. This is a capability the underlying `llm` layer
already provides and the agent loop exposes for free.

## Deferred to later phases

- **Compaction**: token estimation, a threshold check, summarization at turn
  boundaries, and cut-point selection. The `TransformContext` hook is its seam.
- **Session persistence**: a session tree with branching and on-disk storage.
- **Skills and system-prompt assembly.**
- **Execution environment**: filesystem and shell abstractions for a
  coding-agent package.

## Open questions

- Whether the engine should ever expose a synchronous `Complete`-style helper
  that drains the event channel and returns the final transcript, mirroring
  `llm.Complete`.
- How `Agent.Prompt` should behave when called during an active run: reject,
  queue as a follow-up, or steer.
