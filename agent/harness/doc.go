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
//
// A run can be reconfigured between turns via the Set* methods — model, thinking
// level, system prompt, the tool registry, and which registered tools are active
// (advertised to the model). Changes apply from the next run.
//
// Skills and prompt templates are named, reusable prompts: Skill injects a
// skill's instructions as a turn, PromptFromTemplate expands a template's
// arguments, and FormatSkillsForSystemPrompt advertises the model-invocable
// skills inside a system prompt.
package harness
