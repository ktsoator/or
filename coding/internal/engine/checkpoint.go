package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/contextprojection"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

// modelStreamFn is the product's model-request boundary. It projects hidden
// product context into a detached provider input, checkpoints the canonical
// messages and any newly emitted context attachments, then reaches the
// provider. A persistence failure prevents both context commit and provider I/O.
func (s *Session) modelStreamFn(delegate agent.StreamFn) agent.StreamFn {
	if delegate == nil {
		delegate = llm.Stream
	}
	return func(
		ctx context.Context,
		model llm.Model,
		input llm.Context,
		options llm.StreamOptions,
	) (<-chan llm.Event, error) {
		checkpoint := s.lifecycle.beginStepCheckpoint(
			len(input.Messages),
			time.Now().UTC(),
		)
		step := checkpoint.step
		correlation := s.lifecycle.attachProviderRequest()
		s.recorder.Record(observability.Event{
			Name: observability.StepStarted, SessionID: s.sessionID,
			RunID: step.runID, TurnID: step.lifecycleTurnID, StepID: step.stepID,
			Status: "running", StartedAt: step.startedAt,
		})
		prepared := s.context.prepareStep(input)
		checkpointStarted := time.Now().UTC()
		providerInput := prepared.Input
		requestHeader, checkpointErr := s.buildProviderRequestHeader(
			model,
			prepared,
			options,
			correlation,
		)
		if checkpointErr == nil {
			checkpointErr = s.persistModelInput(
				ctx,
				input.Messages,
				prepared.Pending,
				checkpoint.entries,
				requestHeader,
			)
		}
		if checkpointErr == nil {
			providerInput, checkpointErr = s.validateCommittedProviderRequest(
				model,
				prepared.Input,
				options,
				correlation,
			)
		}
		if checkpointErr != nil {
			s.recorder.Record(observability.Event{
				Name: observability.CheckpointFailed, Level: slog.LevelError,
				SessionID: s.sessionID, RunID: correlation.runID,
				TurnID: correlation.turnID, RequestID: correlation.requestID,
				StepID: correlation.stepID,
				Status: "failed", ErrorCode: "checkpoint_persist_failed",
				Reason:    "provider_request",
				StartedAt: checkpointStarted, Duration: time.Since(checkpointStarted),
				MessageCount: len(input.Messages), AttachmentCount: len(prepared.Pending),
			})
			checkpointErr = fmt.Errorf("coding: persist model request checkpoint: %w", checkpointErr)
			s.execution.recordPersistenceError(checkpointErr)
			return nil, checkpointErr
		}
		s.lifecycle.commitStepCheckpoint(step.stepID, checkpoint.pendingCount)
		s.recorder.Record(observability.Event{
			Name:      observability.CheckpointCompleted,
			SessionID: s.sessionID, RunID: correlation.runID,
			TurnID: correlation.turnID, RequestID: correlation.requestID,
			StepID: correlation.stepID,
			Status: "completed", Reason: "provider_request", StartedAt: checkpointStarted,
			Duration: time.Since(checkpointStarted), MessageCount: len(input.Messages),
			AttachmentCount: len(prepared.Pending),
		})
		s.context.commit(prepared)
		return s.observeProviderStream(ctx, delegate, model, providerInput, options, correlation)
	}
}

func (s *Session) observeProviderStream(
	ctx context.Context,
	delegate agent.StreamFn,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
	correlation requestCorrelation,
) (<-chan llm.Event, error) {
	startedAt := time.Now().UTC()
	s.recorder.Record(observability.Event{
		Name: observability.ProviderStarted, SessionID: s.sessionID,
		RunID: correlation.runID, TurnID: correlation.turnID,
		StepID:    correlation.stepID,
		RequestID: correlation.requestID, Status: "running",
		StartedAt: startedAt, Provider: model.Provider, Model: model.ID,
	})
	attempts := newProviderAttemptObserver(s, correlation, model, options)
	stream, err := delegate(ctx, model, input, attempts.options)
	if err != nil {
		attempts.finishPending("no_response")
		s.recordProviderFailure(
			correlation, model, startedAt, 0, "provider_setup_failed", nil, ctx,
		)
		return nil, err
	}

	observed := make(chan llm.Event)
	go func() {
		defer close(observed)
		var firstOutputAt time.Time
		terminalSeen := false
		for event := range stream {
			now := time.Now().UTC()
			if firstOutputAt.IsZero() && isFirstOutputEvent(event.Type) {
				firstOutputAt = now
			}
			if !terminalSeen && (event.Type == llm.EventDone || event.Type == llm.EventError) {
				terminalSeen = true
				attempts.finishPending("no_response")
				if event.Message != nil {
					event.Message.ProviderRequestID = correlation.requestID
				}
				s.recordProviderTerminal(
					correlation, model, startedAt, firstOutputAt, event, ctx,
				)
			}
			observed <- event
		}
		if !terminalSeen {
			attempts.finishPending("no_response")
			s.recordProviderFailure(
				correlation, model, startedAt, elapsed(startedAt, firstOutputAt),
				"stream_closed_without_terminal", nil, ctx,
			)
		}
	}()
	return observed, nil
}

func isFirstOutputEvent(eventType llm.EventType) bool {
	switch eventType {
	case llm.EventTextDelta, llm.EventThinkingDelta, llm.EventToolCallStart:
		return true
	default:
		return false
	}
}

func elapsed(startedAt, completedAt time.Time) time.Duration {
	if startedAt.IsZero() || completedAt.IsZero() || completedAt.Before(startedAt) {
		return 0
	}
	return completedAt.Sub(startedAt)
}

func (s *Session) recordProviderTerminal(
	correlation requestCorrelation,
	model llm.Model,
	startedAt, firstOutputAt time.Time,
	event llm.Event,
	ctx context.Context,
) {
	message := event.Message
	if event.Type == llm.EventError || message == nil {
		code := "provider_stream_failed"
		if message == nil {
			code = "terminal_message_missing"
		}
		s.recordProviderFailure(
			correlation, model, startedAt, elapsed(startedAt, firstOutputAt), code, message, ctx,
		)
		return
	}
	record := observability.Event{
		Name: observability.ProviderCompleted, SessionID: s.sessionID,
		RunID: correlation.runID, TurnID: correlation.turnID,
		StepID:    correlation.stepID,
		RequestID: correlation.requestID, Status: "completed",
		StartedAt: startedAt, Duration: time.Since(startedAt),
		TimeToFirstOutput: elapsed(startedAt, firstOutputAt),
		Provider:          model.Provider, Model: model.ID,
		ResponseModel:      message.ResponseModel,
		ProviderResponseID: message.ResponseID, StopReason: string(message.StopReason),
	}
	usageEventFields(&record, message.Usage)
	s.recorder.Record(record)
}

func (s *Session) recordProviderFailure(
	correlation requestCorrelation,
	model llm.Model,
	startedAt time.Time,
	timeToFirstOutput time.Duration,
	errorCode string,
	message *llm.AssistantMessage,
	ctx context.Context,
) {
	status := "failed"
	if ctx.Err() != nil {
		status = "cancelled"
		errorCode = "context_cancelled"
	}
	event := observability.Event{
		Name: observability.ProviderFailed, Level: slog.LevelError,
		SessionID: s.sessionID, RunID: correlation.runID,
		TurnID: correlation.turnID, RequestID: correlation.requestID,
		StepID: correlation.stepID,
		Status: status, ErrorCode: errorCode, StartedAt: startedAt,
		Duration: time.Since(startedAt), TimeToFirstOutput: timeToFirstOutput,
		Provider: model.Provider, Model: model.ID,
	}
	if message != nil {
		event.ResponseModel = message.ResponseModel
		event.ProviderResponseID = message.ResponseID
		event.StopReason = string(message.StopReason)
		usageEventFields(&event, message.Usage)
	}
	s.recorder.Record(event)
}

type providerAttemptObserver struct {
	session     *Session
	correlation requestCorrelation
	model       llm.Model
	options     llm.StreamOptions

	mu        sync.Mutex
	attempt   int
	attemptID string
	startedAt time.Time
}

func newProviderAttemptObserver(
	session *Session,
	correlation requestCorrelation,
	model llm.Model,
	options llm.StreamOptions,
) *providerAttemptObserver {
	observer := &providerAttemptObserver{
		session: session, correlation: correlation, model: model, options: options,
	}
	originalRequest := options.OnRequest
	originalResponse := options.OnResponse
	observer.options.OnRequest = func(method, url string, body []byte) {
		observer.startAttempt()
		if originalRequest != nil {
			originalRequest(method, url, body)
		}
	}
	observer.options.OnResponse = func(status int, headers http.Header) {
		observer.finishResponse(status)
		if originalResponse != nil {
			originalResponse(status, headers)
		}
	}
	return observer
}

func (o *providerAttemptObserver) startAttempt() {
	o.mu.Lock()
	pending, hasPending := o.pendingEventLocked("no_response")
	o.attempt++
	o.attemptID = observability.NewID("attempt")
	o.startedAt = time.Now().UTC()
	event := o.event(observability.HTTPAttemptStarted, "running", "", o.attempt, 0)
	event.StartedAt = o.startedAt
	o.mu.Unlock()
	if hasPending {
		o.session.recorder.Record(pending)
	}
	o.session.recorder.Record(event)
}

func (o *providerAttemptObserver) finishResponse(status int) {
	o.mu.Lock()
	startedAt := o.startedAt
	if startedAt.IsZero() {
		o.mu.Unlock()
		return
	}
	event := o.event(observability.HTTPAttemptResponse, "completed", "", o.attempt, status)
	o.startedAt = time.Time{}
	o.attemptID = ""
	o.mu.Unlock()
	event.StartedAt = startedAt
	event.Duration = time.Since(startedAt)
	o.session.recorder.Record(event)
}

func (o *providerAttemptObserver) finishPending(errorCode string) {
	o.mu.Lock()
	event, ok := o.pendingEventLocked(errorCode)
	o.mu.Unlock()
	if ok {
		o.session.recorder.Record(event)
	}
}

func (o *providerAttemptObserver) pendingEventLocked(errorCode string) (observability.Event, bool) {
	if o.startedAt.IsZero() {
		return observability.Event{}, false
	}
	event := o.event(observability.HTTPAttemptResponse, "failed", errorCode, o.attempt, 0)
	event.Level = slog.LevelError
	event.StartedAt = o.startedAt
	event.Duration = time.Since(o.startedAt)
	o.startedAt = time.Time{}
	o.attemptID = ""
	return event, true
}

func (o *providerAttemptObserver) event(
	name, status, errorCode string,
	attempt, httpStatus int,
) observability.Event {
	return observability.Event{
		Name: name, SessionID: o.session.sessionID,
		RunID: o.correlation.runID, TurnID: o.correlation.turnID,
		StepID:    o.correlation.stepID,
		RequestID: o.correlation.requestID, Status: status, ErrorCode: errorCode,
		Provider: o.model.Provider, Model: o.model.ID,
		AttemptID: o.attemptID, Attempt: attempt, HTTPStatus: httpStatus,
	}
}

// persistModelInput appends the canonical request prefix and any new hidden
// attachments before the provider is called. input is authoritative here:
// RunLoop may emit MessageEnd before Agent has reduced that event into its live
// snapshot, while input already contains the complete canonical prefix.
func (s *Session) persistModelInput(
	ctx context.Context,
	input []llm.Message,
	attachments []contextprojection.Attachment,
	positioned []positionedJournalEntry,
	requestHeader transcript.Entry,
) error {
	messages := make([]agent.AgentMessage, len(input))
	for index, message := range input {
		messages[index] = agent.FromLLM(message)
	}
	contextEntries := make([]transcript.Entry, len(attachments))
	for index, attachment := range attachments {
		contextEntries[index] = transcript.NewContext(transcript.ContextAttachment{
			AttachmentID: attachment.ID,
			Epoch:        attachment.Epoch,
			Kind:         string(attachment.Kind),
			Placement:    string(attachment.Placement),
			Path:         attachment.Path,
			Revision:     attachment.Revision,
			Rendered:     attachment.Rendered,
		})
	}
	if err := s.journal.persistMessages(
		ctx,
		messages,
		contextEntries,
		positioned,
		requestHeader,
	); err != nil {
		return err
	}
	return s.journal.validateModelContext(messages)
}

func (s *Session) buildProviderRequestHeader(
	model llm.Model,
	prepared contextprojection.PreparedStep,
	options llm.StreamOptions,
	correlation requestCorrelation,
) (transcript.Entry, error) {
	modelContext, err := s.journal.modelContextSnapshot()
	if err != nil {
		return transcript.Entry{}, fmt.Errorf("coding: read model-context boundary: %w", err)
	}
	capturedOptions, err := transcript.CaptureRequestOptions(
		model.Protocol,
		prepared.Input.Tools,
		options,
	)
	if err != nil {
		return transcript.Entry{}, err
	}
	attachments := make([]transcript.RequestAttachment, len(prepared.Attachments))
	for index, attachment := range prepared.Attachments {
		attachments[index] = transcript.RequestAttachment{
			AttachmentID: attachment.ID,
			MessageIndex: attachment.MessageIndex,
		}
	}
	return transcript.NewRequestHeader(transcript.RequestHeader{
		ProviderRequestID: correlation.requestID,
		RunID:             correlation.runID, TurnID: correlation.turnID, StepID: correlation.stepID,
		Provider: model.Provider, Model: model.ID, Protocol: model.Protocol,
		ThinkingLevel:           options.Reasoning,
		SystemPrompt:            prepared.Input.SystemPrompt,
		Tools:                   prepared.Input.Tools,
		Options:                 capturedOptions,
		ActiveCompactionEntryID: modelContext.ActiveCompactionEntryID,
		Attachments:             attachments,
	}), nil
}

func (s *Session) validateCommittedProviderRequest(
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
	correlation requestCorrelation,
) (llm.Context, error) {
	reconstructed, err := s.journal.reconstructCommittedProviderRequest(
		correlation.requestID,
	)
	if err != nil {
		return llm.Context{}, fmt.Errorf("coding: reconstruct committed provider request: %w", err)
	}
	header := reconstructed.Header
	if header.RunID != correlation.runID || header.TurnID != correlation.turnID ||
		header.StepID != correlation.stepID ||
		header.ProviderRequestID != correlation.requestID {
		return llm.Context{}, fmt.Errorf("coding: reconstructed provider request has different lifecycle ids")
	}
	if header.Provider != model.Provider || header.Model != model.ID ||
		header.Protocol != model.Protocol || header.ThinkingLevel != options.Reasoning {
		return llm.Context{}, fmt.Errorf("coding: reconstructed provider request has different model settings")
	}
	if err := validateProviderInputParity(input, reconstructed.Input); err != nil {
		return llm.Context{}, fmt.Errorf("coding: reconstructed provider input: %w", err)
	}
	captured, err := transcript.CaptureRequestOptions(model.Protocol, input.Tools, options)
	if err != nil {
		return llm.Context{}, err
	}
	expectedOptions, err := json.Marshal(captured)
	if err != nil {
		return llm.Context{}, fmt.Errorf("coding: encode provider request options: %w", err)
	}
	actualOptions, err := json.Marshal(header.Options)
	if err != nil {
		return llm.Context{}, fmt.Errorf("coding: encode reconstructed provider request options: %w", err)
	}
	if !bytes.Equal(actualOptions, expectedOptions) {
		return llm.Context{}, fmt.Errorf("coding: reconstructed provider request has different semantic options")
	}
	return reconstructed.Input, nil
}

func validateProviderInputParity(expected, reconstructed llm.Context) error {
	if expected.SystemPrompt != reconstructed.SystemPrompt {
		return fmt.Errorf("system prompt differs")
	}
	expectedTools, err := json.Marshal(expected.Tools)
	if err != nil {
		return fmt.Errorf("encode expected tools: %w", err)
	}
	reconstructedTools, err := json.Marshal(reconstructed.Tools)
	if err != nil {
		return fmt.Errorf("encode reconstructed tools: %w", err)
	}
	if !bytes.Equal(reconstructedTools, expectedTools) {
		return fmt.Errorf("tools differ")
	}
	if len(expected.Messages) != len(reconstructed.Messages) {
		return fmt.Errorf(
			"message count %d, want %d",
			len(reconstructed.Messages),
			len(expected.Messages),
		)
	}
	for index := range expected.Messages {
		expectedMessage, err := llm.MarshalMessage(expected.Messages[index])
		if err != nil {
			return fmt.Errorf("encode expected message %d: %w", index, err)
		}
		reconstructedMessage, err := llm.MarshalMessage(reconstructed.Messages[index])
		if err != nil {
			return fmt.Errorf("encode reconstructed message %d: %w", index, err)
		}
		if !bytes.Equal(reconstructedMessage, expectedMessage) {
			return fmt.Errorf("message %d differs", index)
		}
	}
	return nil
}
