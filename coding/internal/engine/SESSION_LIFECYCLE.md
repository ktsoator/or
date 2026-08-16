# Session lifecycle, event log, and durability checkpoints

Status: lifecycle and side-effect checkpoint foundation implemented. The
broader event vocabulary and diagnostic schema described here remain an
incremental target.

## Purpose

The coding agent already has a model/tool loop, streaming, queues, transcript
persistence, provider checkpoints, and diagnostic tracing. The next evolution
is to give those pieces one explicit lifecycle and one durable source of truth.

This design has three goals:

1. Make run, turn, step, request, and retry boundaries unambiguous.
2. Make the session event log sufficient for context projection, replay, and
   crash recovery.
3. Prevent a provider request or tool side effect from starting before the
   facts required to recover it are durable.

The design borrows the useful lifecycle and checkpoint ideas from DeepSeek
Harness without adopting its plugin system or rewriting Or's runtime at once.

## Lifecycle vocabulary

```text
Session
+-- Run
    +-- Turn
        +-- Step
            +-- ProviderRequest
                +-- Attempt
```

### Session

A `Session` is the durable conversation container. It survives process restarts
and owns the append-only event sequence from which model context, product
history, recovery state, and replay views are projected.

### Run

A `Run` is one active `Prompt` or `Continue` invocation, from admission until
the agent becomes idle, completes, fails, or is cancelled. A run is the unit of
concurrency control and top-level product status.

A run can contain more than one turn when follow-up input is claimed before the
agent becomes idle. Input submitted after the previous run is idle starts a new
run.

### Turn

A `Turn` is one claimed unit of user or follow-up intent. It begins when the
runtime accepts that input and can contain one or more steps.

- The initial prompt starts the first turn of a run.
- A claimed follow-up ends the preceding turn and starts another turn in the
  same run.
- Steering does not start a turn. It joins the active turn and becomes visible
  to the next step.

### Step

A `Step` is one logical assistant cycle: construct model input, obtain one
terminal assistant message, and execute the tool calls requested by that
message. A step ends only after every requested tool call has a terminal result
or the step is closed with a terminal failure.

Tool results or steering may require another step in the same turn. A terminal
assistant message with no more work ends the turn.

### ProviderRequest

A `ProviderRequest` is one immutable, model-visible input assembled for a step.
Normally a step has one provider request. A recovery policy that changes the
input, model, or effective request configuration creates another provider
request in the same step.

The provider request ID identifies the logical request, not an individual HTTP
exchange.

### Attempt

An `Attempt` is one physical dispatch of a provider request. A transient
transport retry keeps the provider request ID and creates a new attempt ID.

```text
same immutable input + transport retry  => same request, new attempt
changed input, model, tools, or config  => new request
```

## Mapping from the agent runtime

The `agent.RunLoop` vocabulary predates this distinction. Its
`TurnStart` and `TurnEnd` events surround one model request and its tool batch,
so engine adapts them to durable `Step` boundaries, not durable `Turn`
boundaries.

| Current concept | Target concept | Migration note |
| --- | --- | --- |
| `agent.AgentStart` / `AgentEnd` | `Run` | Engine persists a stable Run identity. |
| `agent.TurnStart` / `TurnEnd` | `Step` | Engine adapts these legacy public events internally. |
| Initial prompt | First `Turn` | Engine persists an explicit boundary. |
| Follow-up queue item | New `Turn` | It remains inside the active `Run`. |
| Steering queue item | Current `Turn`, next `Step` | It does not increment the Turn identity. |
| Provider SDK transport retry | New `Attempt` | Reuse the immutable provider request identity. |
| Product app-level retry | New `Step` and `ProviderRequest` | Keep the active Turn identity. |
| Overflow recovery with rebuilt context | New `Step` and `ProviderRequest` | Keep the active Turn identity. |

Compatibility adapters continue exposing old product event names where needed,
while durable storage and diagnostics use the precise lifecycle. Diagnostic
events carry separate `TurnID` and `StepID` fields.

## Durable events and diagnostic events

Or needs two event streams with different guarantees.

### Session events

`SessionEvent` records product facts required to reconstruct the conversation.
It is append-only, versioned, and the session's durable format contract.
It must not be rotated or dropped. A required append or checkpoint failure is a
product failure and blocks the external operation protected by that checkpoint.

Session events support:

- model-context projection;
- conversation and tool-result history;
- restart and crash repair;
- fork and replay;
- durable UI state derived from the conversation.

### Observability events

`ObservabilityEvent` records diagnostics such as first-token latency, attempt
duration, token usage, estimated cost, approval wait time, and checkpoint
duration. It may be sampled, rotated, or written fail-open. It must never be
required to resume a session or decide whether a side effect may be retried.

The trace UI may join both streams by stable IDs, but the streams remain
different sources with different reliability requirements.

## Session event vocabulary

Names below describe the target wire semantics. Schema work may add fields, but
must preserve their meanings.

| Event | Durable fact |
| --- | --- |
| `run/start` | A run was admitted with a stable run ID. |
| `run/end` | The run reached a terminal status. |
| `turn/start` | One unit of initial or follow-up intent was claimed. |
| `turn/end` | The turn completed, failed, was cancelled, or was interrupted. |
| `step/start` | An assistant cycle began inside a turn. |
| `step/end` | Its assistant and tool work reached a terminal boundary. |
| `user/message` | Model-visible user, follow-up, steering, or injected input. |
| `request/header` | Effective model, provider, system prompt, options, and tool schemas when initially established or changed. |
| `context/attachment` | Product-generated model context and its placement. |
| `assistant/message` | The complete terminal assistant message and reported usage. |
| `tool/call` | A validated and authorized tool invocation is about to be dispatched. |
| `tool/result` | The terminal model-facing and product-facing result for one assistant-requested invocation. |
| `compaction` | A summary boundary and the source range it replaces in active model context. |

Raw assistant chunks, request snapshots, provider attempts, timings, and costs
may remain diagnostic data. They become session events only if replay or product
behavior is defined to require them.

The version 6 transcript persists and validates `run/start`,
`run/end`, `turn/start`, `turn/end`, `step/start`, and `step/end`. Existing
message, context, compaction, tool-call, and tool-outcome entries supply the
other implemented durable facts. Product history and run timing are projected
from explicit lifecycle boundaries; no derived `run` entry exists. Every entry
carries a contiguous durable `seq`, and append/replace reject gaps or restated
positions. Versions 5 and earlier are rejected explicitly and are not migrated
on load or append.

Every session event has at least:

```text
session_id
sequence
event_id
timestamp
type
payload
```

Lifecycle events additionally carry their owning `run_id`, `turn_id`, and
`step_id` as applicable. IDs are stable facts; ordinal numbers are presentation
metadata and must not be used as identities.

## Context projection

The model input is a deterministic projection of a committed session-event
prefix, not a second independently authoritative transcript.

Conceptually:

```text
BuildContext(committed events, active compaction, request header)
    -> immutable provider input
```

For every provider request, Or must be able to identify:

- the committed event-sequence boundary used as input;
- the effective request header;
- the active compaction boundary;
- the stable request and step identities.

A full diagnostic request snapshot is useful for inspection, but it validates
the projection rather than replacing the session event log as the source of
truth.

## Semantic checkpoints

A checkpoint is a durability barrier tied to the meaning of an external
operation. It is not merely a periodic flush.

### 1. Before provider dispatch

Before the first attempt of a provider request, persist and flush the complete
event prefix from which its input was projected, including newly claimed input,
request-header changes, context attachments, compaction state, and the step
boundary.

```text
append request facts
-> flush
-> dispatch provider attempt
```

If the flush fails, the provider adapter must not be called.

### 2. Before a tool side effect

After validation, permission, and approval succeed, append `tool/call` and make
it durable before invoking the tool body.

```text
validate and authorize
-> append tool/call
-> flush
-> recheck cancellation
-> invoke tool body
-> append tool/result
```

The event means "dispatch may have occurred", not merely "the model requested
this tool". The assistant message already records the model's request. This
distinction is what makes recovery safe.

A validation failure, policy denial, or rejected approval produces a terminal
`tool/result` without a `tool/call`, because no body became eligible for
dispatch. That result links directly to the request in `assistant/message`.

For a parallel batch, each tool body is protected by its own durable intent.
The implementation may coalesce storage work, but it must preserve the barrier
for every call and commit terminal results in model order.

If the flush fails, the tool body must not execute.

### 3. Before the next step or clean idle

Before another provider request, flush the preceding assistant message, tool
results, and completed step boundary. The pre-provider checkpoint naturally
provides this barrier.

When a run becomes cleanly idle, flush its terminal turn and run events so a
successful return never leaves an apparently interrupted durable tail.

## Crash repair

On load, validate the event sequence and repair only an interrupted tail. Never
rewrite or discard the committed prefix.

For each assistant-requested tool call without a durable result:

```text
assistant requested call, no tool/call
    => TOOL_NOT_STARTED

tool/call exists, no tool/result
    => TOOL_OUTCOME_UNKNOWN
```

`TOOL_NOT_STARTED` means the dispatch barrier was never committed, so the tool
body did not start under this protocol. The repaired model-facing result may
state that the call can be requested again if still needed.

`TOOL_OUTCOME_UNKNOWN` means dispatch may have occurred. Recovery must append a
synthetic error result and must not automatically retry the operation. The
model or user must first use tool semantics and external state to decide whether
a retry is safe. Read-only or explicitly idempotent tools may later opt into a
more permissive policy, but that policy is not inferred from the tool name.

After synthesizing missing tool results in model order, repair closes any open
step, turn, and run with an `interrupted` reason. The repaired tail is appended
durably and becomes ordinary input to context projection.

## Event invariants

The session validator and tests must enforce these rules:

1. Sequence numbers are contiguous and events are immutable after append.
2. A session has at most one open run, a run at most one open turn, and a turn
   at most one open step.
3. Parent lifecycle IDs exist and match the currently open boundaries.
4. Every step has at most one terminal assistant message.
5. Every assistant-requested tool call has exactly one terminal tool result in
   model order, including synthetic cancellation and recovery results.
6. Every `tool/call` refers to a request in that step's assistant message. Every
   `tool/result` refers to the assistant request and, when a `tool/call` exists,
   cites that dispatch-intent event. Preflight and authorization failures have
   a result without a `tool/call`.
7. A step cannot end with unresolved tool calls; a turn cannot end with an open
   step; a run cannot end with an open turn.
8. A failed provider checkpoint performs no provider dispatch.
9. A failed tool checkpoint performs no tool-body dispatch.
10. Observability data is never required to validate, repair, or project a
    session.

## Implementation status

Implemented:

1. Version 4 lifecycle and `tool/call` transcript entry types; earlier
   development logs are not migrated.
2. Authorized tool intent is durable before execution, and a failed checkpoint
   prevents the tool body from running.
3. Interrupted-tail validation and `TOOL_NOT_STARTED` /
   `TOOL_OUTCOME_UNKNOWN` repair, followed by interrupted Step, Turn, and Run
   closure.
4. Explicit Run, Turn, and Step boundaries, with follow-up and steering
   semantics enforced by tests.
5. Fork behavior preserves a valid completed lifecycle tail.
6. The trace UI presents session-scoped Turn and Step groups instead of a flat
   provider-request list.
7. A deterministic, read-only `ProjectSession` fold reconstructs lifecycle,
   message ownership, tool dispatch/result/outcome relationships, context
   attachments, and compaction boundaries from a committed transcript prefix.
8. Version 5 removes the legacy `run` history entry. Engine History and
   RunCompleted message correlation consume the lifecycle projection instead.
9. Context attachments are committed after their owning `step/start`, so their
   Run, Turn, and Step ownership is deterministic.
10. Projection and validation drive one deterministic `sessionReducer`; a
    single `RecoverSession` replay validates the committed prefix, repairs
    interrupted tools, and then closes its lifecycle boundaries.
11. Version 6 assigns a contiguous durable `seq` to every entry. Journal writes
    prepare a batch-local reducer delta, append the sequenced entries to the
    Store, and commit that delta only after persistence succeeds. Failed Store
    appends leave the canonical reducer and its next sequence unchanged.
12. A session-owned projection registry eagerly drives the lifecycle, message,
    tool, context, and compaction read model at the same commit boundary. Its
    snapshots carry the shared `AsOfSeq`; History and RunCompleted message
    correlation consume this incremental view instead of replaying the log.

Remaining:

1. Switch model-context and trace consumers from their current scans
   to registered session projections.
2. Persist request-header facts so each provider input can be reconstructed
   from a precise committed event boundary.
3. Add versioned persisted projection checkpoints only after the live
   incremental semantics and invalidation rules are stable.

Observability now emits real Turn and Step lifecycle events. Provider requests
carry both parent IDs, and each physical provider dispatch has a stable
`AttemptID`; the attempt number remains presentation metadata. Request snapshots
and the version 2 trace bundle preserve the same correlation chain. Older local
diagnostic logs remain readable through a projection fallback, but new events
do not overload `TurnID` with Step identity.

Each phase should be independently releasable. A storage-version change must
be rejected explicitly rather than partially decoded as the current format.

## Acceptance tests

The migration is complete when tests demonstrate that:

- a provider is not called when its checkpoint fails;
- a tool body is not called when its intent checkpoint fails;
- a call without `tool/call` repairs to `TOOL_NOT_STARTED`;
- a `tool/call` without `tool/result` repairs to `TOOL_OUTCOME_UNKNOWN`;
- parallel tool calls retain model order and accurate started/result state;
- every provider input can be reconstructed from its committed event prefix;
- every tool request has exactly one terminal result after normal execution,
  cancellation, panic, or crash repair;
- trace projection no longer guesses lifecycle boundaries from event names.

## Non-goals

This design does not require Or to adopt DeepSeek Harness's dependency injection
or plugin architecture, persist the rotating observability log as conversation
history, store every streamed token durably, or replace the entire agent loop in
one change.
