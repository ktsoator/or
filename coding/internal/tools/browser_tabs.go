package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type browserTabsArgs struct{}

type BrowserTabsStatus string

const (
	BrowserTabsCompleted BrowserTabsStatus = "completed"
	BrowserTabsFailed    BrowserTabsStatus = "failed"
	BrowserTabsCancelled BrowserTabsStatus = "cancelled"
	BrowserTabsTimeout   BrowserTabsStatus = "timeout"
)

type BrowserControlCapability string

const (
	BrowserControlRead     BrowserControlCapability = "read"
	BrowserControlNavigate BrowserControlCapability = "navigate"
	BrowserControlInteract BrowserControlCapability = "interact"
)

type BrowserTabStatus string

const (
	BrowserTabIdle       BrowserTabStatus = "idle"
	BrowserTabNavigating BrowserTabStatus = "navigating"
	BrowserTabReady      BrowserTabStatus = "ready"
	BrowserTabFailed     BrowserTabStatus = "failed"
)

// BrowserOpenTab is bounded metadata for one tab in the requesting
// session's browser workspace. TabID is stable until that tab is closed.
type BrowserOpenTab struct {
	TabID  string           `json:"tabID"`
	URL    string           `json:"url,omitempty"`
	Title  string           `json:"title,omitempty"`
	Status BrowserTabStatus `json:"status"`
}

// BrowserControlledTab describes temporary Agent attachment to an open tab.
// It intentionally does not record who originally created the tab.
type BrowserControlledTab struct {
	TabID        string                     `json:"tabID"`
	Capabilities []BrowserControlCapability `json:"capabilities"`
}

type BrowserTabsResult struct {
	ID             string
	Status         BrowserTabsStatus
	OpenTabs       []BrowserOpenTab
	ControlledTabs []BrowserControlledTab
	Selected       string
	Error          string
}

type BrowserTabsProvider interface {
	BrowserTabs(context.Context) (BrowserTabsResult, error)
}

// BrowserTabs returns a product tool that lists only the tabs belonging to the
// current coding session. It exposes metadata, not page content.
func BrowserTabs(providers ...BrowserTabsProvider) Tool {
	var provider BrowserTabsProvider
	if len(providers) > 0 {
		provider = providers[0]
	}
	def := llm.MustTool[browserTabsArgs]("tabs_context", browserTabsText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "List browser tabs",
			Execute: func(ctx context.Context, _ string, _ json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				if provider == nil {
					return failedResult("browser_unavailable", "Could not list browser tabs: browser observation is unavailable", nil), nil
				}
				result, err := provider.BrowserTabs(ctx)
				if err != nil {
					return agent.ToolResult{}, err
				}
				return browserTabsToolResult(result), nil
			},
		},
		AccessFor:  InternalAccess,
		Guidelines: browserTabsText.guidelines,
	}
}

func browserTabsToolResult(result BrowserTabsResult) agent.ToolResult {
	switch result.Status {
	case BrowserTabsCompleted:
		openTabs := result.OpenTabs
		if openTabs == nil {
			openTabs = []BrowserOpenTab{}
		}
		controlledTabs := result.ControlledTabs
		if controlledTabs == nil {
			controlledTabs = []BrowserControlledTab{}
		}
		var selected *string
		if value := strings.TrimSpace(result.Selected); value != "" {
			selected = &value
		}
		encoded, err := json.MarshalIndent(struct {
			ControlledTabs []BrowserControlledTab `json:"controlledTabs"`
			OpenTabs       []BrowserOpenTab       `json:"openTabs"`
			Selected       *string                `json:"selected"`
		}{
			ControlledTabs: controlledTabs,
			OpenTabs:       openTabs,
			Selected:       selected,
		}, "", "  ")
		if err != nil {
			return failedResult("browser_tabs_result_invalid", "Could not list browser tabs: browser returned an invalid result", nil)
		}
		return textResult(string(encoded))
	case BrowserTabsFailed:
		detail := strings.TrimSpace(result.Error)
		if detail == "" {
			detail = "browser tab listing failed"
		}
		return failedResult("browser_tabs_failed", "Could not list browser tabs: "+detail, nil)
	case BrowserTabsTimeout:
		return timeoutResult("browser_tabs_timeout", "The browser did not return its open tabs", nil)
	case BrowserTabsCancelled:
		return cancelledResult("browser_tabs_cancelled", "The browser tab listing was cancelled", nil)
	default:
		return failedResult("browser_tabs_result_invalid", "Could not list browser tabs: browser returned an invalid result", nil)
	}
}
