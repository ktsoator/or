// Package harness is a stateful orchestration layer over the core agent.
//
// Where agent.Agent is a thin wrapper over the run loop, Harness composes the
// surrounding concerns an application usually needs — transcript persistence,
// context compaction, and a per-turn system prompt — without forking the loop.
// It wraps an agent.Agent and drives the existing LoopConfig hooks, so the core
// transcript, steering and follow-up queues, and event subscription are reused
// rather than reimplemented.
//
// Each concern is a field on Options; leaving one nil disables it, so the
// zero-configured Harness behaves like a plain Agent. It implements Session
// (transcript persistence and resume), a per-turn system-prompt builder, and a
// Compactor that shrinks the transcript projected to the model. Compaction is
// projection-only: the Session and transcript keep the full history.
package harness
