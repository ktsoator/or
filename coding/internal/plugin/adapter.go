package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/capability"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

// Load initializes one supervisor and adapts its advertised tools into the
// existing Coding capability pipeline.
func Load(ctx context.Context, supervisor Supervisor, host HostInfo) (capability.Definition, error) {
	if supervisor == nil {
		return capability.Definition{}, errors.New("plugin supervisor is required")
	}

	initialized, err := supervisor.Initialize(ctx, InitializeRequest{
		ProtocolVersion: ProtocolVersion,
		Host:            host,
	})
	if err != nil {
		return capability.Definition{}, fmt.Errorf("initialize plugin: %w", err)
	}
	if initialized.ProtocolVersion != ProtocolVersion {
		return capability.Definition{}, fmt.Errorf(
			"plugin %q selected unsupported protocol version %d (host requires %d)",
			initialized.Plugin.ID,
			initialized.ProtocolVersion,
			ProtocolVersion,
		)
	}
	manifest, err := validateManifest(initialized.Plugin)
	if err != nil {
		return capability.Definition{}, err
	}

	listed, err := supervisor.ListTools(ctx, ListToolsRequest{})
	if err != nil {
		return capability.Definition{}, fmt.Errorf("list tools for plugin %q: %w", manifest.ID, err)
	}
	contributions := make([]capability.ToolContribution, 0, len(listed.Tools))
	seen := make(map[string]struct{}, len(listed.Tools))
	for index, descriptor := range listed.Tools {
		normalized, err := validateToolDescriptor(descriptor)
		if err != nil {
			return capability.Definition{}, fmt.Errorf("plugin %q tool %d: %w", manifest.ID, index, err)
		}
		if _, exists := seen[normalized.Name]; exists {
			return capability.Definition{}, fmt.Errorf("plugin %q advertised tool %q more than once", manifest.ID, normalized.Name)
		}
		seen[normalized.Name] = struct{}{}
		contributions = append(contributions, capability.ToolContribution{
			Tool: adaptTool(supervisor, normalized),
		})
	}

	return capability.Definition{
		Manifest: capability.Manifest{ID: manifest.ID, Version: manifest.Version},
		Tools:    contributions,
	}, nil
}

func validateManifest(manifest Manifest) (Manifest, error) {
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Version = strings.TrimSpace(manifest.Version)
	if manifest.ID == "" {
		return Manifest{}, errors.New("plugin id is required")
	}
	if manifest.Version == "" {
		return Manifest{}, fmt.Errorf("plugin %q version is required", manifest.ID)
	}
	return manifest, nil
}

func validateToolDescriptor(descriptor ToolDescriptor) (ToolDescriptor, error) {
	descriptor.Name = strings.TrimSpace(descriptor.Name)
	descriptor.Description = strings.TrimSpace(descriptor.Description)
	descriptor.Label = strings.TrimSpace(descriptor.Label)
	if descriptor.Name == "" {
		return ToolDescriptor{}, errors.New("tool name is required")
	}
	if len(descriptor.InputSchema) == 0 || !json.Valid(descriptor.InputSchema) {
		return ToolDescriptor{}, fmt.Errorf("tool %q has an invalid input schema", descriptor.Name)
	}
	var schema map[string]any
	if err := json.Unmarshal(descriptor.InputSchema, &schema); err != nil || schema["type"] != "object" {
		return ToolDescriptor{}, fmt.Errorf("tool %q input schema must describe an object", descriptor.Name)
	}
	switch descriptor.ExecutionMode {
	case "", ExecutionParallel, ExecutionSequential:
	default:
		return ToolDescriptor{}, fmt.Errorf("tool %q has invalid execution mode %q", descriptor.Name, descriptor.ExecutionMode)
	}

	guidelines := make([]string, 0, len(descriptor.Guidelines))
	for _, guideline := range descriptor.Guidelines {
		if guideline = strings.TrimSpace(guideline); guideline != "" {
			guidelines = append(guidelines, guideline)
		}
	}
	descriptor.Guidelines = guidelines
	descriptor.InputSchema = append(json.RawMessage(nil), descriptor.InputSchema...)
	return descriptor, nil
}

func adaptTool(supervisor Supervisor, descriptor ToolDescriptor) tools.Tool {
	executionMode := agent.ExecutionMode(descriptor.ExecutionMode)
	return tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.ToolDefinition{
				Name:        descriptor.Name,
				Description: descriptor.Description,
				Parameters:  append(json.RawMessage(nil), descriptor.InputSchema...),
			},
			Label:         descriptor.Label,
			ExecutionMode: executionMode,
			Execute: func(
				ctx context.Context,
				callID string,
				args json.RawMessage,
				onProgress func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				return executeTool(ctx, supervisor, descriptor.Name, callID, args, onProgress)
			},
		},
		Guidelines: append([]string(nil), descriptor.Guidelines...),
		// Protocol v1 has no declarative access contract. Leaving AccessFor nil
		// makes the existing permission service treat every call conservatively.
		AccessFor: nil,
	}
}

func executeTool(
	ctx context.Context,
	supervisor Supervisor,
	toolName string,
	callID string,
	args json.RawMessage,
	onProgress func(agent.ToolProgress),
) (agent.ToolResult, error) {
	var progressMu sync.Mutex
	acceptingProgress := true
	var progressErr error
	closeProgress := func() error {
		progressMu.Lock()
		defer progressMu.Unlock()
		acceptingProgress = false
		return progressErr
	}
	defer closeProgress()
	reportProgress := func(notification ProgressNotification) {
		progressMu.Lock()
		defer progressMu.Unlock()
		if !acceptingProgress || progressErr != nil {
			return
		}
		progress, err := decodeProgress(callID, notification)
		if err != nil {
			progressErr = err
			return
		}
		if onProgress != nil {
			onProgress(progress)
		}
	}

	result, executeErr := supervisor.Execute(ctx, ExecuteRequest{
		CallID:    callID,
		Tool:      toolName,
		Arguments: append(json.RawMessage(nil), args...),
	}, reportProgress)

	invalidProgress := closeProgress()

	if executeErr != nil {
		return agent.ToolResult{}, fmt.Errorf("execute plugin tool %q: %w", toolName, executeErr)
	}
	if invalidProgress != nil {
		return protocolFailure("plugin_progress_invalid", invalidProgress.Error()), nil
	}
	converted, err := decodeResult(callID, result)
	if err != nil {
		return protocolFailure("plugin_result_invalid", err.Error()), nil
	}
	return converted, nil
}

func decodeProgress(callID string, notification ProgressNotification) (agent.ToolProgress, error) {
	if notification.CallID != callID {
		return agent.ToolProgress{}, fmt.Errorf("progress call id %q does not match %q", notification.CallID, callID)
	}
	content, err := decodeContent(notification.Content)
	if err != nil {
		return agent.ToolProgress{}, fmt.Errorf("decode progress content: %w", err)
	}
	data, err := decodeData(notification.Data)
	if err != nil {
		return agent.ToolProgress{}, fmt.Errorf("decode progress data: %w", err)
	}
	return agent.ToolProgress{Content: content, Data: data}, nil
}

func decodeResult(callID string, result Result) (agent.ToolResult, error) {
	if result.CallID != callID {
		return agent.ToolResult{}, fmt.Errorf("result call id %q does not match %q", result.CallID, callID)
	}
	content, err := decodeContent(result.Content)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode result content: %w", err)
	}
	data, err := decodeData(result.Outcome.Data)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode result data: %w", err)
	}
	status, err := decodeOutcomeStatus(result.Outcome)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{
		Content: content,
		Outcome: agent.ToolOutcome{
			Status:    status,
			ErrorCode: result.Outcome.ErrorCode,
			ExitCode:  result.Outcome.ExitCode,
			Data:      data,
		},
		Terminate: result.Terminate,
	}, nil
}

func decodeOutcomeStatus(outcome Outcome) (agent.ToolOutcomeStatus, error) {
	if outcome.Status == OutcomeSuccess {
		if outcome.ErrorCode != "" {
			return "", errors.New("successful plugin result cannot include an error code")
		}
		return agent.ToolOutcomeSuccess, nil
	}
	if outcome.ErrorCode == "" {
		return "", fmt.Errorf("plugin result with status %q requires an error code", outcome.Status)
	}
	switch outcome.Status {
	case OutcomeFailed:
		return agent.ToolOutcomeFailed, nil
	case OutcomeCancelled:
		return agent.ToolOutcomeCancelled, nil
	case OutcomeTimeout:
		return agent.ToolOutcomeTimeout, nil
	default:
		return "", fmt.Errorf("plugin result has invalid status %q", outcome.Status)
	}
}

func decodeContent(blocks []Content) ([]llm.ToolResultContent, error) {
	if blocks == nil {
		return nil, nil
	}
	content := make([]llm.ToolResultContent, 0, len(blocks))
	for index, block := range blocks {
		switch block.Type {
		case ContentText:
			content = append(content, &llm.TextContent{Text: block.Text})
		case ContentImage:
			if block.Data == "" || strings.TrimSpace(block.MIMEType) == "" {
				return nil, fmt.Errorf("image block %d requires data and mimeType", index)
			}
			content = append(content, &llm.ImageContent{Data: block.Data, MIMEType: block.MIMEType})
		default:
			return nil, fmt.Errorf("content block %d has invalid type %q", index, block.Type)
		}
	}
	return content, nil
}

func decodeData(raw json.RawMessage) (any, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("decode trailing data: %w", err)
	}
	return value, nil
}

func protocolFailure(code, text string) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
		Outcome: agent.ToolOutcome{
			Status:    agent.ToolOutcomeFailed,
			ErrorCode: code,
		},
	}
}
