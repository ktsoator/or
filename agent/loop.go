package agent

import "context"

// RunLoop drives a complete tool-call loop and returns a channel of events. The
// final AgentEnd event carries the messages the run appended to the transcript.
//
// prompts are the new messages that start the run; base is the existing context
// they extend. The loop streams an assistant turn, executes any tool calls,
// appends results, then consults PrepareNextTurn and ShouldStopAfterTurn before
// continuing. When no tool calls and no steering messages remain, it polls
// GetFollowUpMessages; if none arrive, the run ends.
//
// Not yet implemented.
func RunLoop(ctx context.Context, prompts []AgentMessage, base Context, cfg LoopConfig) <-chan AgentEvent {
	panic("agent: RunLoop not implemented")
}
