# Coding Agent backend architecture

`engine` owns one stateful Or coding session. It adapts the reusable `agent`
runtime to product-specific context, tools, permissions, persistence, retries,
compaction, and events.

```text
app -> conversation -> engine -> agent -> llm
          |              |
          |              +-> tools, permission, prompt, skills
          |              +-> contextprojection, transcript, compaction
          +-> session lifecycle, queue ownership, product notifications
```

`conversation` owns the collection of product sessions and their lifecycle.
`engine` owns the execution policy inside one session. The reusable `agent`
package owns the provider-neutral model/tool loop and ephemeral runtime state.

The precise run/turn/step semantics, durable session-event model, and
side-effect checkpoint contract are documented in [SESSION_LIFECYCLE.md](SESSION_LIFECYCLE.md).

## Package boundaries

### `conversation`

Owns product-wide session coordination:

- create, restore, edit, fork, and delete conversations;
- serialize user commands around an engine session;
- manage queued-message ownership and product cancellation behavior;
- forward engine events to transports and usage accounting;
- construct `engine.Options` from application services.

It must not implement model turns, tool execution, context projection, or
transcript checkpoint rules.

### `engine`

Owns one coding session's execution policy:

- assemble `agent.Agent` with Or tools and permission hooks;
- refresh project context and Skills at top-level run boundaries;
- project hidden product context into each provider request;
- checkpoint resumable model-visible prefixes before provider I/O;
- persist explicit run, turn, and step lifecycle boundaries;
- persist terminal messages, tool outcomes, and run metadata;
- apply retry, compaction, and context-overflow recovery policy;
- expose product events, history, usage, background-task state, and diagnostic
  correlations across Run, Turn, Step, provider request, attempt, and tool call.

It must not own HTTP/SSE wire formats, the global session registry, provider
settings storage, or reusable SDK abstractions.

### Reusable and supporting packages

- `agent` owns the provider-neutral run loop, live state, queues, and hooks.
- `llm` owns one provider request and provider-neutral message types.
- `tools` owns concrete Or tools and their access descriptions.
- `permission` owns authorization policy for concrete tool effects.
- `contextprojection` owns staged hidden context and request projection.
- `transcript` owns durable entry types, projection, and storage contracts.
- `compaction` owns summary generation and transcript compaction preparation.
- `prompt` and `skills` own source material used to build model context.

The Coding backend composes `agent` directly. It does not depend on `harness`:
Or's transcript checkpoints, context protocol, permissions, and recovery rules
are product contracts rather than generic SDK behavior.

## Engine source ownership

```text
session.go          public configuration, Session state, queues, and setters
assembly.go         construction and dependency wiring
prompt.go           top-level prompt input and explicit Skill invocation
run.go              Prompt/Continue run lifecycle and completion metadata
run_state.go        concurrency-safe state for the active run
lifecycle.go        durable Run, Turn, and Step boundary coordination
checkpoint.go       pre-provider context projection and durable checkpoint
journal.go          durable entries, outcomes, persistence, and snapshots
context_refresh.go  project context and Skill refresh lifecycle
context.go          context-usage measurement and estimation
compact.go          explicit compaction
auto_compact.go     proactive compaction and overflow recovery
retry.go            transient provider retry policy
event.go            product event contract and agent-event projection
history.go          durable and live history projection
attachment.go       attached-file message context
background_tasks.go background-task query and control
tool_outcome.go     durable tool-outcome encoding
```

## Run sequence

One top-level `Prompt` or `Continue` follows this order:

1. Acquire the session run lock and flush any previously unpersisted messages.
2. Refresh Skills and project context staged since the previous request.
3. Record run state, queue durable Run and initial Turn boundaries, and publish
   `RunStarted`.
4. Apply proactive compaction when the measured context crosses its threshold.
5. Enter `agent.Agent`; before every provider request, project hidden context
   and persist the complete canonical request prefix plus its Step boundary.
6. Retry transient failures or compact and recover from context overflow when
   policy permits.
7. A claimed follow-up closes the current Turn and starts another in the same
   Run. Steering and tool loops create Steps inside the current Turn.
8. Persist terminal Step, Turn, and Run boundaries and terminal messages, then
   publish `RunCompleted`. Product history is projected from those events.

## Behavioral invariants

Refactors must preserve these contracts:

1. A session runs at most one top-level `Prompt`, `Continue`, or `Compact` at a
   time; steering, follow-up, abort, subscription, and snapshots remain safe.
2. Skill and project-context loaders run at top-level request boundaries, not
   for provider retries, tool-loop turns, or overflow recovery.
3. The canonical model-visible prefix and newly projected hidden attachments
   are durable before the provider request begins.
4. A checkpoint failure prevents provider I/O and is not retried as a model or
   transport failure.
5. Every completed run appends its terminal messages and lifecycle boundaries;
   no derived history record is written back into the event log.
6. Compaction retains complete product history while replacing only the active
   model context.
7. Tool outcomes remain associated with their tool-call IDs across persistence,
   history projection, and compaction.
8. Engine events are product-domain events. HTTP/SSE serialization belongs in
   `httpapi` and must not leak into the engine.
9. Cancellation and provider errors retain their identity for conversation and
   transport adapters.
10. An authorized tool invocation is durable before its body can execute; a
    failed tool checkpoint stops the remaining run before another side effect or
    provider request.
11. Session restore validates and durably repairs interrupted tool-call tails
    before model-context and product-history projection. A dispatched call with
    no durable result has an unknown outcome and is never retried implicitly.
12. Durable Run, Turn, and Step IDs are nested and never reused. Follow-up input
    starts a Turn; steering, tool loops, and provider retries stay in the active
    Turn and create Steps.
13. Diagnostic events use the same Turn and Step identities as lifecycle
    checkpoints. A Step whose checkpoint fails remains diagnostic-only.
    Provider requests and physical HTTP attempts add stable request and attempt
    IDs; an attempt number is presentation metadata, not identity.
14. Session projection and interrupted-tail repair share one transcript reducer;
    ordering and ownership rules must not be reimplemented by consumers.

## Refactoring rules

- Move cohesive behavior between files before adding types or subpackages.
- Keep the `Session` public surface stable unless a product contract change is
  intentional and coordinated with `conversation` and `httpapi`.
- Treat transcript shape, event ordering, checkpoint timing, queue semantics,
  and error identity as observable behavior.
- Share code with `harness` only when both consumers have the same
  product-neutral contract without policy flags.
- Verify changes through `engine`, `conversation`, and `httpapi`, then run the
  full repository test suite for changes that cross package boundaries.
