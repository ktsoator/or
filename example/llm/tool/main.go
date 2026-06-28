// Command tool demonstrates a complete typed-tool execution loop.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/ktsoator/or/llm"

	_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol
)

const maxToolRounds = 8

type weatherArgs struct {
	City  string `json:"city" jsonschema:"description=City to retrieve weather for,minLength=1"`
	Units string `json:"units,omitempty" jsonschema:"description=Temperature units,enum=celsius,enum=fahrenheit"`
}

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	weatherTool := llm.MustTool[weatherArgs](
		"get_weather",
		"Get the current weather for a city",
	)

	messages := []llm.Message{
		llm.UserText("What is the weather in Shanghai?"),
	}

	answer, err := runToolLoop(
		context.Background(),
		model,
		messages,
		weatherTool,
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nFinal answer:\n%s\n", answer.Text())
}

func runToolLoop(
	ctx context.Context,
	model llm.Model,
	messages []llm.Message,
	weatherTool llm.ToolDefinition,
) (llm.AssistantMessage, error) {
	tools := []llm.ToolDefinition{weatherTool}

	for round := 1; round <= maxToolRounds; round++ {
		fmt.Printf("\n--- Model call %d ---\n", round)
		response, err := llm.Complete(
			ctx,
			model,
			llm.Context{
				SystemPrompt: "Use get_weather before answering any weather question.",
				Messages:     messages,
				Tools:        tools,
			},
			llm.StreamOptions{},
		)
		if err != nil {
			// Complete may return partial content with an error. Tool calls from a
			// failed response must never be executed.
			return response, fmt.Errorf("model request failed: %w", err)
		}

		switch response.StopReason {
		case llm.StopReasonLength:
			return response, errors.New("model output reached its length limit")
		case llm.StopReasonError, llm.StopReasonAborted:
			return response, fmt.Errorf("model stopped with %s", response.StopReason)
		}

		calls := toolCalls(response)
		if len(calls) == 0 {
			if response.StopReason == llm.StopReasonToolUse {
				return response, errors.New("model stopped for tool use without a tool call")
			}
			return response, nil
		}
		if response.StopReason != llm.StopReasonToolUse {
			return response, fmt.Errorf(
				"model returned tool calls with stop reason %q",
				response.StopReason,
			)
		}

		// The successful assistant message must precede its tool results in the
		// conversation history.
		messages = append(messages, &response)
		modes := toolArgumentModes(response)

		for _, call := range calls {
			mode := modes[call.ID]
			if mode == "" {
				mode = llm.ArgumentsStrict
			}
			fmt.Printf(
				"Tool call: %s id=%s arguments=%v mode=%s\n",
				call.Name,
				call.ID,
				call.Arguments,
				mode,
			)

			result, isError := executeWeatherTool(weatherTool, call, mode)
			toolResult := llm.ToolResult(call.ID, call.Name, result)
			toolResult.IsError = isError
			messages = append(messages, toolResult)
			fmt.Printf("Tool result: %s (error=%t)\n", result, isError)
		}
	}

	return llm.AssistantMessage{}, fmt.Errorf(
		"tool loop exceeded %d model calls",
		maxToolRounds,
	)
}

func executeWeatherTool(
	tool llm.ToolDefinition,
	call *llm.ToolCall,
	mode llm.ArgumentsMode,
) (string, bool) {
	if call.Name != tool.Name {
		return fmt.Sprintf("unknown tool %q", call.Name), true
	}
	if mode == llm.ArgumentsPartial || mode == llm.ArgumentsInvalid {
		return fmt.Sprintf(
			"tool arguments are incomplete (parse mode %s); generate them again",
			mode,
		), true
	}

	arguments, err := llm.DecodeToolCall[weatherArgs](tool, *call)
	if err != nil {
		return fmt.Sprintf("invalid tool arguments: %v", err), true
	}

	result, err := getWeather(arguments)
	if err != nil {
		return err.Error(), true
	}
	return result, false
}

// getWeather stands in for an application-owned service. The fixed result
// keeps this example runnable without a second network API.
func getWeather(arguments weatherArgs) (string, error) {
	city := strings.TrimSpace(arguments.City)
	if city == "" {
		return "", errors.New("city must not be empty")
	}

	if arguments.Units == "fahrenheit" {
		return fmt.Sprintf("It is sunny and 75°F in %s.", city), nil
	}
	return fmt.Sprintf("It is sunny and 24°C in %s.", city), nil
}

func toolCalls(message llm.AssistantMessage) []*llm.ToolCall {
	var calls []*llm.ToolCall
	for _, content := range message.Content {
		if call, ok := content.(*llm.ToolCall); ok && call != nil {
			calls = append(calls, call)
		}
	}
	return calls
}

// toolArgumentModes associates argument-recovery diagnostics with tool calls.
// A call without a recovery diagnostic was parsed as strict JSON.
func toolArgumentModes(message llm.AssistantMessage) map[string]llm.ArgumentsMode {
	modes := make(map[string]llm.ArgumentsMode)
	for _, diagnostic := range message.Diagnostics {
		if diagnostic.Type != llm.DiagnosticToolArgumentsRecovered {
			continue
		}
		toolCallID, _ := diagnostic.Details["toolCallId"].(string)
		mode, _ := diagnostic.Details["mode"].(string)
		if toolCallID != "" {
			modes[toolCallID] = llm.ArgumentsMode(mode)
		}
	}
	return modes
}
