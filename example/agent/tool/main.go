// Command tool demonstrates an interactive stateful agent with tool use.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type weatherArgs struct {
	City  string `json:"city" jsonschema:"description=City to retrieve weather for,minLength=1"`
	Units string `json:"units,omitempty" jsonschema:"description=Temperature units,enum=celsius,enum=fahrenheit"`
}

func main() {
	weatherDefinition := llm.MustTool[weatherArgs](
		"get_weather",
		"Get simulated current weather for a city",
	)
	weatherTool := agent.AgentTool{
		Definition: weatherDefinition,
		Execute: func(
			_ context.Context,
			_ string,
			rawArgs json.RawMessage,
			onUpdate func(agent.ToolResult),
		) (agent.ToolResult, error) {
			var args weatherArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return agent.ToolResult{}, fmt.Errorf("decode weather arguments: %w", err)
			}

			onUpdate(agent.ToolResult{Details: "Generating simulated weather data..."})
			text, err := simulatedWeather(args)
			if err != nil {
				return agent.ToolResult{}, err
			}
			return agent.ToolResult{
				Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
			}, nil
		},
	}

	runner := agent.New(agent.Options{
		SystemPrompt:  "You are a concise, friendly assistant. Always call get_weather before answering a weather question. Reply in English.",
		Model:         llm.GetModel("deepseek", "deepseek-v4-flash"),
		ThinkingLevel: llm.ModelThinkingHigh,
		Tools:         []agent.AgentTool{weatherTool},
	})

	style := newTerminalStyle()
	printer := &eventPrinter{showReasoning: true, style: style}
	runner.Subscribe(printer.print)

	fmt.Printf("%s\n", style.paint(ansiBold+ansiCyan, "OR Weather Agent"))
	fmt.Printf("%s\n", style.paint(ansiDim, "Commands: /model <provider> <model-id>  /debug  /thinking  /quit"))
	fmt.Printf("%s\n", style.paint(ansiDim, "Model: deepseek/deepseek-v4-flash"))
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("\n%s ", style.paint(ansiBold+ansiBlue, "You ›"))
		if !scanner.Scan() {
			break
		}

		prompt := strings.TrimSpace(scanner.Text())
		if prompt == "" {
			continue
		}
		if shouldExit(prompt) {
			break
		}
		if strings.EqualFold(prompt, "/debug") {
			printer.debug = !printer.debug
			fmt.Printf("%s Debug events: %s\n", style.paint(ansiYellow, "⚙"), enabledText(printer.debug))
			continue
		}
		if strings.EqualFold(prompt, "/thinking") {
			printer.showReasoning = !printer.showReasoning
			fmt.Printf("%s Reasoning: %s\n", style.paint(ansiYellow, "⚙"), enabledText(printer.showReasoning))
			continue
		}
		if fields := strings.Fields(prompt); len(fields) > 0 && strings.EqualFold(fields[0], "/model") {
			if len(fields) == 1 {
				current := runner.Snapshot().Model
				fmt.Printf(
					"%s Current model: %s/%s\n",
					style.paint(ansiCyan, "◆"),
					current.Provider,
					current.ID,
				)
				continue
			}
			if len(fields) != 3 {
				fmt.Printf("%s Usage: /model <provider> <model-id>\n", style.paint(ansiBold+ansiRed, "✗"))
				continue
			}

			nextModel, ok := llm.LookupModel(fields[1], fields[2])
			if !ok {
				fmt.Printf(
					"%s Unknown model: %s/%s\n",
					style.paint(ansiBold+ansiRed, "✗"),
					fields[1],
					fields[2],
				)
				continue
			}

			state := runner.Snapshot()
			runner = agent.New(agent.Options{
				SystemPrompt:  state.SystemPrompt,
				Model:         nextModel,
				ThinkingLevel: llm.ClampThinkingLevel(nextModel, state.ThinkingLevel),
				Tools:         state.Tools,
				Messages:      state.Messages,
			})
			runner.Subscribe(printer.print)
			fmt.Printf(
				"%s Switched to %s/%s · %d messages preserved\n",
				style.paint(ansiBold+ansiGreen, "↻ MODEL"),
				nextModel.Provider,
				nextModel.ID,
				len(state.Messages),
			)
			continue
		}

		if err := runner.Prompt(context.Background(), prompt); err != nil {
			fmt.Printf("\n%s %v\n", style.paint(ansiBold+ansiRed, "✗ ERROR"), err)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	state := runner.Snapshot()
	fmt.Printf("\n%s %d messages saved\n", style.paint(ansiDim, "Session closed ·"), len(state.Messages))
}

func enabledText(enabled bool) string {
	if enabled {
		return "ON"
	}
	return "OFF"
}

func shouldExit(input string) bool {
	switch strings.ToLower(input) {
	case "退出", "exit", "quit", "/exit", "/quit":
		return true
	default:
		return false
	}
}

type eventPrinter struct {
	reasoningStarted bool
	answerStarted    bool
	showReasoning    bool
	debug            bool
	turns            int
	toolCalls        int
	style            terminalStyle
	trace            []string
	pendingDelta     llm.EventType
	pendingCount     int
}

func (p *eventPrinter) print(event agent.AgentEvent) {
	switch event.Type {
	case agent.AgentStart:
		p.turns = 0
		p.toolCalls = 0
		p.trace = nil
		p.pendingDelta = ""
		p.pendingCount = 0
		p.debugEvent(event)
	case agent.AgentEnd:
		p.debugEvent(event)
		fmt.Printf(
			"\n\n%s  Turns %d  ·  Tools %d  ·  Messages %d\n",
			p.style.paint(ansiBold+ansiGreen, "✓ RUN COMPLETE"),
			p.turns,
			p.toolCalls,
			len(event.Messages),
		)
		p.printTrace()
	case agent.TurnStart:
		p.turns++
		p.reasoningStarted = false
		p.answerStarted = false
		p.debugEvent(event)
	case agent.TurnEnd:
		p.debugEvent(event)
	case agent.MessageStart:
		p.debugEvent(event)
	case agent.MessageUpdate:
		if event.LLMEvent == nil {
			p.debugEvent(event)
			return
		}
		switch event.LLMEvent.Type {
		case llm.EventThinkingDelta:
			p.queueDelta(event.LLMEvent.Type)
			if p.showReasoning && !p.reasoningStarted {
				fmt.Printf(
					"\n%s %s\n",
					p.style.paint(ansiBold+ansiMagenta, "◇ REASONING"),
					p.style.paint(ansiDim, fmt.Sprintf("Turn %d", p.turns)),
				)
				p.reasoningStarted = true
			}
			if p.showReasoning {
				fmt.Print(p.style.paint(ansiDim, event.LLMEvent.Delta))
			}
		case llm.EventTextDelta:
			p.queueDelta(event.LLMEvent.Type)
			if !p.answerStarted {
				fmt.Printf(
					"\n%s %s\n",
					p.style.paint(ansiBold+ansiCyan, "◆ ANSWER"),
					p.style.paint(ansiDim, fmt.Sprintf("Turn %d", p.turns)),
				)
				p.answerStarted = true
			}
			fmt.Print(event.LLMEvent.Delta)
		default:
			if event.LLMEvent.Type == llm.EventToolCallDelta {
				p.queueDelta(event.LLMEvent.Type)
				return
			}
			p.debugEvent(event)
		}
	case agent.MessageEnd:
		p.debugEvent(event)
	case agent.ToolStart:
		p.debugEvent(event)
		p.toolCalls++
		fmt.Printf(
			"\n\n%s %s\n%s  Input   %s\n",
			p.style.paint(ansiBold+ansiYellow, "┌─ TOOL"),
			p.style.paint(ansiBold, event.ToolName),
			p.style.paint(ansiYellow, "│"),
			p.style.paint(ansiDim, formatValue(event.Args)),
		)
	case agent.ToolUpdate:
		p.debugEvent(event)
		if result, ok := event.Result.(agent.ToolResult); ok && result.Details != nil {
			fmt.Printf("%s  Status  %v\n", p.style.paint(ansiYellow, "│"), result.Details)
		} else {
			fmt.Printf("%s  Status  Running...\n", p.style.paint(ansiYellow, "│"))
		}
	case agent.ToolEnd:
		p.debugEvent(event)
		if event.IsError {
			fmt.Printf("%s  %s Failed\n", p.style.paint(ansiRed, "└─"), p.style.paint(ansiBold+ansiRed, "✗"))
		} else {
			fmt.Printf("%s  %s Completed\n", p.style.paint(ansiGreen, "└─"), p.style.paint(ansiBold+ansiGreen, "✓"))
		}
	}
}

func (p *eventPrinter) debugEvent(event agent.AgentEvent) {
	if !p.debug {
		return
	}
	p.flushPendingDelta()
	if event.LLMEvent != nil {
		p.trace = append(p.trace, "LLM       "+llmEventDescription(event.LLMEvent.Type))
		return
	}
	p.trace = append(p.trace, p.agentEventDescription(event))
}

func (p *eventPrinter) queueDelta(eventType llm.EventType) {
	if !p.debug {
		return
	}
	if p.pendingDelta == eventType {
		p.pendingCount++
		return
	}
	p.flushPendingDelta()
	p.pendingDelta = eventType
	p.pendingCount = 1
}

func (p *eventPrinter) flushPendingDelta() {
	if !p.debug || p.pendingCount == 0 {
		return
	}
	p.trace = append(p.trace, fmt.Sprintf("LLM       %s × %d", llmEventDescription(p.pendingDelta), p.pendingCount))
	p.pendingDelta = ""
	p.pendingCount = 0
}

func (p *eventPrinter) printTrace() {
	if !p.debug {
		return
	}
	p.flushPendingDelta()
	fmt.Printf("\n%s\n", p.style.paint(ansiBold+ansiYellow, "EVENT TRACE"))
	for index, entry := range p.trace {
		branch := "├─"
		if index == len(p.trace)-1 {
			branch = "└─"
		}
		fmt.Printf(
			"%s %s  %s\n",
			p.style.paint(ansiDim, branch),
			p.style.paint(ansiDim, fmt.Sprintf("%02d", index+1)),
			p.style.paint(ansiDim, entry),
		)
	}
}

func (p *eventPrinter) agentEventDescription(event agent.AgentEvent) string {
	switch event.Type {
	case agent.AgentStart:
		return "RUN       started (agent_start)"
	case agent.AgentEnd:
		return "RUN       completed (agent_end)"
	case agent.TurnStart:
		return fmt.Sprintf("TURN      #%d started (turn_start)", p.turns)
	case agent.TurnEnd:
		return fmt.Sprintf("TURN      #%d completed (turn_end)", p.turns)
	case agent.MessageStart:
		return "MESSAGE   opened (message_start)"
	case agent.MessageEnd:
		return "MESSAGE   closed (message_end)"
	case agent.MessageUpdate:
		return "MESSAGE   updated without LLM detail (message_update)"
	case agent.ToolStart:
		return fmt.Sprintf("TOOL      %s started (tool_execution_start)", event.ToolName)
	case agent.ToolUpdate:
		return fmt.Sprintf("TOOL      %s reported progress (tool_execution_update)", event.ToolName)
	case agent.ToolEnd:
		return fmt.Sprintf("TOOL      %s completed (tool_execution_end)", event.ToolName)
	default:
		return fmt.Sprintf("AGENT     %s", event.Type)
	}
}

func llmEventDescription(eventType llm.EventType) string {
	switch eventType {
	case llm.EventThinkingStart:
		return "reasoning opened (thinking_start)"
	case llm.EventThinkingDelta:
		return "reasoning chunks (thinking_delta)"
	case llm.EventThinkingEnd:
		return "reasoning closed (thinking_end)"
	case llm.EventTextStart:
		return "answer opened (text_start)"
	case llm.EventTextDelta:
		return "answer chunks (text_delta)"
	case llm.EventTextEnd:
		return "answer closed (text_end)"
	case llm.EventToolCallStart:
		return "tool call opened (toolcall_start)"
	case llm.EventToolCallDelta:
		return "tool argument chunks (toolcall_delta)"
	case llm.EventToolCallEnd:
		return "tool call closed (toolcall_end)"
	default:
		return string(eventType)
	}
}

func formatValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
)

type terminalStyle struct {
	enabled bool
}

func newTerminalStyle() terminalStyle {
	info, err := os.Stdout.Stat()
	return terminalStyle{
		enabled: err == nil &&
			info.Mode()&os.ModeCharDevice != 0 &&
			os.Getenv("NO_COLOR") == "" &&
			os.Getenv("TERM") != "dumb",
	}
}

func (s terminalStyle) paint(code, text string) string {
	if !s.enabled {
		return text
	}
	return code + text + ansiReset
}

// simulatedWeather uses fixed data so the example needs no second service.
func simulatedWeather(args weatherArgs) (string, error) {
	city := strings.TrimSpace(args.City)
	if city == "" {
		return "", errors.New("city must not be empty")
	}
	if args.Units == "fahrenheit" {
		return fmt.Sprintf("It is sunny and 75°F in %s.", city), nil
	}
	return fmt.Sprintf("It is sunny and 24°C in %s.", city), nil
}
