# Steering and follow-ups

`Prompt` blocks until a run completes, and an agent runs one prompt at a time —
calling `Prompt` again while a run is in progress returns an error. To influence a
run as it happens, call `Steer`, `FollowUp`, or `Abort` from another goroutine.

```go
go func() {
	_ = assistant.Prompt(ctx, "Summarize the repository")
}()

assistant.Steer(agent.FromLLM(llm.UserText("Focus on the agent package.")))
```

All of an agent's methods are safe for concurrent use.

## Steering: inject mid-run

`Steer` queues a message to inject before the run's next turn. The loop drains the
steering queue after each turn's tool calls finish, so a steering message is seen
by the model on the following turn — useful for redirecting a long task without
restarting it.

```go
assistant.Steer(agent.FromLLM(llm.UserText("Stop and show me what you have so far.")))
```

## Follow-ups: continue past a stop

`FollowUp` queues a message to process when the agent would otherwise stop. When a
run reaches the point of ending, the loop drains the follow-up queue and, if it
finds anything, runs another turn instead of finishing.

```go
assistant.FollowUp(agent.FromLLM(llm.UserText("Now write the tests.")))
```

The difference is timing: steering interrupts an in-progress run between turns;
a follow-up extends a run that was about to end.

## How many drain at once

`SteeringMode` and `FollowUpMode` control how many queued messages a single drain
returns:

- `QueueOneAtATime` (the default) injects only the oldest queued message, leaving
  the rest for later drain points.
- `QueueAll` injects every queued message at once.

```go
assistant := agent.New(agent.Options{
	SteeringMode: agent.QueueAll,        // inject all pending steering at the next turn
	FollowUpMode: agent.QueueOneAtATime, // process follow-ups one run at a time
	/* ... */
})
```

## Continuing a run

`Continue` resumes from the current transcript without adding a new message — for
a retry, or after appending messages out of band.

A provider needs a user or tool result as the latest turn. When the transcript
ends with an assistant message, `Continue` falls back to the queues: it drains the
steering queue first, then the follow-up queue, and runs whatever it finds. It
returns an error only when the last message is an assistant message and both
queues are empty.

```go
if err := assistant.Continue(ctx); err != nil {
	log.Fatal(err) // e.g. "cannot continue from an assistant message"
}
```

## Aborting

`Abort` cancels the current run, if any. The in-flight turn ends with an aborted
stop reason, and any tools still pending receive an aborted result so every tool
call is still answered and the transcript stays valid for a later request.

```go
assistant.Abort()
```

## Inspecting and clearing the queues

```go
assistant.HasQueuedMessages()   // any steering or follow-up message queued?
assistant.ClearSteeringQueue()  // drop queued steering messages
assistant.ClearFollowUpQueue()  // drop queued follow-up messages
assistant.ClearQueues()         // drop both
```

`Reset` clears the transcript, the last error, and both queues while keeping the
configuration — call it when the agent is idle to start a fresh conversation.
