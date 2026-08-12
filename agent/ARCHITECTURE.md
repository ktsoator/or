# Agent backend architecture

The backend has three orchestration layers with different stability and policy
boundaries:

```text
coding/internal/engine                 harness
Or product policy and                  optional reusable orchestration
durable session behavior               for SDK consumers
                  \                     /
                   +------> agent <-----+
                    run loop and ephemeral state
                               |
                               v
                              llm
                    one provider-neutral request
```

`coding/internal/engine` and `harness` are sibling consumers of `agent`. The
coding product must not depend on `harness`, and reusable packages must not
import product packages.

## Layer ownership

### `llm`

Owns one model request and the stable provider-neutral types around it:

- models and provider protocols;
- messages and content;
- tool definitions and calls;
- reasoning and request options;
- streaming events and usage.

It does not execute tools or retain conversation state.

### `agent`

Owns one task's execution mechanics:

- `RunLoop`, the stateless model/tool loop;
- tool argument validation, execution, progress, and results;
- ordered run, turn, message, and tool events;
- `Agent`, the optional in-memory transcript and live-state wrapper;
- steering, follow-up, cancellation, and per-turn hooks.

It must remain free of concrete tools, persistence formats, system prompts,
permissions, retry policy, and product-specific context.

The package source is organized by runtime responsibility:

```text
agent.go          public state, options, Agent, and construction
agent_run.go      Prompt, Continue, run lifecycle, and loop configuration
agent_state.go    snapshots, reconfiguration, reset, and event reduction
queue.go          steering and follow-up queues
subscription.go   listener registration and event fan-out
loop.go           stateless turn loop and model streaming
tools_exec.go     tool preflight, execution, and result finalization
message.go        transcript message adaptation
config.go         stateless loop context and hooks
event.go          run event contract
tool.go           tool contract and outcome types
```

### `harness`

Owns reusable orchestration that SDK consumers may opt into:

- transcript persistence through caller-provided `Session` interfaces;
- generic context compaction;
- per-turn system-prompt construction;
- a generic tool registry and Skills abstraction.

It composes `agent.Agent`; it does not fork or replace the run loop. Product
rules that only Or needs do not belong here.

### `coding/internal/engine`

Owns Or's stateful coding-session policy:

- built-in coding tools and permission authorization;
- product transcript entries and checkpoint persistence;
- instruction-file, environment, attachment, and Skill context;
- proactive compaction and context-overflow recovery;
- provider retry policy and usage accounting inputs;
- background task state and product-facing event projection.

It composes `agent.Agent` directly because its durable transcript and context
protocol are product-specific. Similarity with `harness` alone is not a reason
to share code: a concern should move into a reusable layer only when its
contract is product-neutral and both consumers can use it without policy flags.

## Run invariants

Refactors must preserve these behavioral contracts:

1. One `Agent` runs at most one `Prompt` or `Continue` at a time.
2. A run captures its model, tools, prompt, and hooks before entering `RunLoop`;
   setters affect the next run unless a turn hook explicitly changes it.
3. Every model tool call produces exactly one tool-result message, including
   validation failures, blocked calls, panics, cancellation, and timeouts.
4. Tool batches may execute concurrently, but preflight, terminal events, and
   result messages remain in source order.
5. `reduce` updates live state before subscribers receive the corresponding
   event, and subscribers are invoked without holding the Agent mutex.
6. Queue handles remain observable through events but are removed before a
   message is retained in the transcript or projected to the model.
7. Cancellation preserves `context.Canceled` or `context.DeadlineExceeded` so
   callers can distinguish an abort from a provider failure.
8. Product persistence checkpoints model-visible prefixes before the next
   provider request; a process interruption must not leave a provider transcript
   that cannot be resumed safely.

## Dependency rules

- `llm` imports no higher orchestration layer.
- `agent` imports only `llm` and the standard library.
- `harness` may import `agent` and `llm`, never `coding`.
- `coding/internal/engine` may import `agent`, `llm`, and other coding product
  packages, never `harness`.
- Transport and UI packages consume projected product events; they do not own
  agent execution policy.

## Refactoring guidance

- Prefer moving cohesive behavior between files before introducing new types.
- Keep `RunLoop` stateless; retained state belongs in a wrapper.
- Add a shared abstraction only after two real consumers have the same contract.
- Keep permissions and concrete tool effects at the product boundary.
- Treat event order, transcript shape, and error identity as public behavior.
- Verify changes at all affected layers: `agent`, `harness`, and
  `coding/internal/engine`, followed by the full repository test suite.
