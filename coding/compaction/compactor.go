// Package compaction prepares and summarizes old coding-session context. It is
// deliberately independent of the coding Session so it can be tested with a
// fake summarizer and reused by future product adapters.
package compaction

import (
	"context"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type Request struct {
	Model           llm.Model
	Messages        []agent.AgentMessage
	PreviousSummary string
	Instructions    string
}

type Response struct {
	Summary       string
	Usage         llm.Usage
	Provider      string
	Model         string
	ResponseModel string
	ResponseID    string
	Timestamp     time.Time
}

// Compactor performs one tool-free summary request.
type Compactor interface {
	Compact(ctx context.Context, request Request) (Response, error)
}
