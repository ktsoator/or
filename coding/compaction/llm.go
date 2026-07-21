package compaction

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const maxSummaryTokens int64 = 16_384

// LLM performs a single model request with no tools and thinking disabled.
type LLM struct {
	StreamFn      agent.StreamFn
	StreamOptions llm.StreamOptions
	GetAPIKey     func(provider string) string
}

func (c LLM) Compact(ctx context.Context, request Request) (Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	streamFn := c.StreamFn
	if streamFn == nil {
		streamFn = llm.Stream
	}
	options := c.StreamOptions
	options.Reasoning = llm.ModelThinkingOff
	options.MaxTokens = maxSummaryTokens
	if request.Model.MaxTokens > 0 && request.Model.MaxTokens < options.MaxTokens {
		options.MaxTokens = request.Model.MaxTokens
	}
	if c.GetAPIKey != nil {
		if key := c.GetAPIKey(request.Model.Provider); key != "" {
			options.APIKey = key
		}
	}
	input := llm.Context{
		SystemPrompt: systemPrompt,
		Messages: []llm.Message{&llm.UserMessage{Content: []llm.UserContent{
			&llm.TextContent{Text: buildPrompt(request)},
		}}},
	}
	stream, err := streamFn(ctx, request.Model, input, options)
	if err != nil {
		return Response{}, fmt.Errorf("compaction: start summary: %w", err)
	}
	var final *llm.AssistantMessage
	for event := range stream {
		if event.Type == llm.EventDone || event.Type == llm.EventError {
			final = event.Message
		}
	}
	if final == nil {
		return Response{}, errors.New("compaction: summary stream closed without a final message")
	}
	if final.StopReason == llm.StopReasonError || final.StopReason == llm.StopReasonAborted {
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		message := final.ErrorMessage
		if message == "" {
			message = string(final.StopReason)
		}
		return Response{}, fmt.Errorf("compaction: summary failed: %s", message)
	}
	summary := strings.TrimSpace(final.Text())
	if summary == "" {
		return Response{}, errors.New("compaction: model returned an empty summary")
	}
	return Response{
		Summary:       summary,
		Usage:         final.Usage,
		Provider:      final.Provider,
		Model:         final.Model,
		ResponseModel: final.ResponseModel,
		ResponseID:    final.ResponseID,
		Timestamp:     time.UnixMilli(final.Timestamp).UTC(),
	}, nil
}
