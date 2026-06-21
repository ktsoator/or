// Package agent is a provider-neutral orchestration layer built on the llm
// package.
//
// It turns a single model request into a complete tool-call loop: stream a
// turn, execute the tool calls the model requests, append the results, and
// continue until the model stops. RunLoop is the stateless engine; Agent is a
// thin stateful wrapper that adds a retained transcript, event subscription,
// and steering and follow-up queues.
//
// A run operates on AgentMessage values — standard llm messages adapted with
// FromLLM, plus any UI-only messages an application keeps in the transcript —
// and projects them to llm.Message only at the request boundary via
// ConvertToLLM. Extension points are function fields on LoopConfig.
//
// This package is an early skeleton. The exported types and signatures are the
// agreed targets from docs/agent/design.md; their bodies are not yet
// implemented.
package agent
