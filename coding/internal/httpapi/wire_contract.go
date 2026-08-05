package httpapi

//go:generate go run ./internal/genwire -source wire_contract.go -output ../../client/src/generated/wire.ts

// wireEventType is the closed set of event names emitted by the history and
// SSE endpoints. Keep event construction on these constants so additions are
// visible to both Go and the generated TypeScript contract.
type wireEventType string

const (
	wireEventUserMessage       wireEventType = "user_message"
	wireEventRunStart          wireEventType = "run_start"
	wireEventDelta             wireEventType = "delta"
	wireEventToolInputStart    wireEventType = "tool_input_start"
	wireEventToolInputDelta    wireEventType = "tool_input_delta"
	wireEventToolInputEnd      wireEventType = "tool_input_end"
	wireEventToolStart         wireEventType = "tool_start"
	wireEventToolEnd           wireEventType = "tool_end"
	wireEventMessageEnd        wireEventType = "message_end"
	wireEventTurnDiscard       wireEventType = "turn_discard"
	wireEventCompactionStart   wireEventType = "compaction_start"
	wireEventCompactionEnd     wireEventType = "compaction_end"
	wireEventTaskStarted       wireEventType = "task_started"
	wireEventTaskNotification  wireEventType = "task_notification"
	wireEventApprovalRequest   wireEventType = "approval_request"
	wireEventApprovalResolved  wireEventType = "approval_resolved"
	wireEventApprovalCancelled wireEventType = "approval_cancelled"
	wireEventBrowserRequest    wireEventType = "browser_request"
	wireEventBrowserTabs       wireEventType = "browser_tabs_request"
	wireEventBrowserInspect    wireEventType = "browser_inspect_request"
	wireEventQueueCancelled    wireEventType = "queue_cancelled"
	wireEventQueueRemoved      wireEventType = "queue_removed"
	wireEventError             wireEventType = "error"
	wireEventDone              wireEventType = "done"
	wireEventSyncRequired      wireEventType = "sync_required"
	wireEventTitleUpdate       wireEventType = "title_update"
	wireEventTitleGeneration   wireEventType = "title_generation_update"
	wireEventQuestionRequest   wireEventType = "question_request"
	wireEventQuestionResolved  wireEventType = "question_resolved"
	wireEventQuestionCancelled wireEventType = "question_cancelled"
)

type wireTaskStatus string

const (
	wireTaskRunning   wireTaskStatus = "running"
	wireTaskSucceeded wireTaskStatus = "succeeded"
	wireTaskFailed    wireTaskStatus = "failed"
	wireTaskStopped   wireTaskStatus = "stopped"
)

type wireTitleGenerationStatus string

const (
	wireTitleGenerationIdle        wireTitleGenerationStatus = "idle"
	wireTitleGenerationGenerating  wireTitleGenerationStatus = "generating"
	wireTitleGenerationSucceeded   wireTitleGenerationStatus = "succeeded"
	wireTitleGenerationFailed      wireTitleGenerationStatus = "failed"
	wireTitleGenerationUnavailable wireTitleGenerationStatus = "unavailable"
)

type wireDeltaKind string

const (
	wireDeltaText     wireDeltaKind = "text"
	wireDeltaThinking wireDeltaKind = "thinking"
)

type wireToolOutcomeStatus string

const (
	wireToolOutcomeSuccess   wireToolOutcomeStatus = "success"
	wireToolOutcomeFailed    wireToolOutcomeStatus = "failed"
	wireToolOutcomeCancelled wireToolOutcomeStatus = "cancelled"
	wireToolOutcomeTimeout   wireToolOutcomeStatus = "timeout"
)

type wireDeliveryMode string

const (
	wireDeliverySteer    wireDeliveryMode = "steer"
	wireDeliveryFollowUp wireDeliveryMode = "followup"
)

type wireInvocationKind string

const (
	wireInvocationPromptTemplate wireInvocationKind = "prompt_template"
)

type wireInvocation struct {
	Kind   wireInvocationKind `json:"kind"`
	Name   string             `json:"name"`
	Source string             `json:"source"`
	Path   string             `json:"path"`
}

type wireBrowserDisposition string

const (
	wireBrowserReuseAgentTab    wireBrowserDisposition = "reuse_agent_tab"
	wireBrowserNewForegroundTab wireBrowserDisposition = "new_foreground_tab"
	wireBrowserNewBackgroundTab wireBrowserDisposition = "new_background_tab"
)

type wireBrowserResultStatus string

const (
	//lint:ignore U1000 This constant is consumed by genwire when it builds the TypeScript union.
	wireBrowserCommitted wireBrowserResultStatus = "committed"
	//lint:ignore U1000 This constant is consumed by genwire when it builds the TypeScript union.
	wireBrowserFailed wireBrowserResultStatus = "failed"
	//lint:ignore U1000 This constant is consumed by genwire when it builds the TypeScript union.
	wireBrowserCancelled wireBrowserResultStatus = "cancelled"
	//lint:ignore U1000 This constant is consumed by genwire when it builds the TypeScript union.
	wireBrowserTimeout wireBrowserResultStatus = "timeout"
)

type wireBrowserInspectionStatus string

const (
	wireBrowserInspectionCompleted wireBrowserInspectionStatus = "completed"
	wireBrowserInspectionFailed    wireBrowserInspectionStatus = "failed"
	wireBrowserInspectionCancelled wireBrowserInspectionStatus = "cancelled"
	wireBrowserInspectionTimeout   wireBrowserInspectionStatus = "timeout"
)

type wireBrowserTabsStatus string

const (
	wireBrowserTabsCompleted wireBrowserTabsStatus = "completed"
	wireBrowserTabsFailed    wireBrowserTabsStatus = "failed"
	wireBrowserTabsCancelled wireBrowserTabsStatus = "cancelled"
	wireBrowserTabsTimeout   wireBrowserTabsStatus = "timeout"
)

type wireBrowserControlCapability string

const (
	wireBrowserControlRead     wireBrowserControlCapability = "read"
	wireBrowserControlNavigate wireBrowserControlCapability = "navigate"
	wireBrowserControlInteract wireBrowserControlCapability = "interact"
)

type wireBrowserTabStatus string

const (
	wireBrowserTabIdle       wireBrowserTabStatus = "idle"
	wireBrowserTabNavigating wireBrowserTabStatus = "navigating"
	wireBrowserTabReady      wireBrowserTabStatus = "ready"
	wireBrowserTabFailed     wireBrowserTabStatus = "failed"
)

type wireBrowserPageStatus string

const (
	wireBrowserPageReady      wireBrowserPageStatus = "ready"
	wireBrowserPageNavigating wireBrowserPageStatus = "navigating"
	wireBrowserPageFailed     wireBrowserPageStatus = "failed"
)

type wireFileChangeType string

const (
	wireChangeFile wireFileChangeType = "file"
)

type wireFailureChangeType string

const (
	wireChangeFailure wireFailureChangeType = "failure"
)

type wireFileOperation string

const (
	wireFileCreate wireFileOperation = "create"
	wireFileUpdate wireFileOperation = "update"
)

// wire:union wireFileChangePayload wireFailureChangePayload
type wireChange interface {
	isWireChange()
}

type wireHunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
}

type wireFileChangePayload struct {
	ChangeType wireFileChangeType `json:"changeType"`
	Path       string             `json:"path"`
	Operation  wireFileOperation  `json:"op"`
	Additions  int                `json:"additions"`
	Deletions  int                `json:"deletions"`
	Bytes      int                `json:"bytes"`
	Hunks      []wireHunk         `json:"hunks"`
}

func (wireFileChangePayload) isWireChange() {}

type wireFailureChangePayload struct {
	ChangeType wireFailureChangeType `json:"changeType"`
	Path       string                `json:"path"`
	Reason     string                `json:"reason"`
	Detail     string                `json:"detail"`
}

func (wireFailureChangePayload) isWireChange() {}

// wireQuestionOption is one selectable answer offered by the agent. The product
// surface adds its own free-text choice, so the agent never sends one.
type wireQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// wireQuestion is one multiple-choice question put to the user.
type wireQuestion struct {
	Question    string               `json:"question"`
	Header      string               `json:"header"`
	Options     []wireQuestionOption `json:"options"`
	MultiSelect bool                 `json:"multiSelect,omitempty"`
}

// wireQuestionAnswer is one reply, carrying the labels the user picked or the
// free text they typed instead.
type wireQuestionAnswer struct {
	Question string   `json:"question"`
	Values   []string `json:"values"`
}

type wireQuestionAnswers struct {
	Questions []wireQuestion       `json:"questions"`
	Answers   []wireQuestionAnswer `json:"answers"`
}

// wireToolOutcome is the single terminal contract for a tool call. Data is
// JSON-shaped and may contain a built-in DTO or capability-defined payload.
type wireToolOutcome struct {
	Status    wireToolOutcomeStatus `json:"status"`
	ErrorCode string                `json:"errorCode,omitempty"`
	ExitCode  *int                  `json:"exitCode,omitempty"`
	Data      any                   `json:"data,omitempty"`
}

// wireEvent is the JSON shape streamed to the browser. Fields are populated
// according to Type; the rest stay zero and are omitted.
type wireEvent struct {
	Type wireEventType `json:"type"`
	// delta events
	Kind  wireDeltaKind `json:"kind,omitempty"`
	Delta string        `json:"delta,omitempty"`
	// tool events (ID correlates tool_start with tool_end)
	Tool    string           `json:"tool,omitempty"`
	Args    any              `json:"args,omitempty"`
	Result  string           `json:"result,omitempty"`
	Outcome *wireToolOutcome `json:"outcome,omitempty"`
	// ToolContentIndex correlates tool argument events before every provider has
	// supplied a stable tool-call ID. Bytes is the size of one argument delta.
	ToolContentIndex *int         `json:"toolContentIndex,omitempty"`
	Bytes            int          `json:"bytes,omitempty"`
	Preview          *wirePreview `json:"preview,omitempty"`
	IsError          bool         `json:"isError,omitempty"`
	// message_end fallback text (used when nothing streamed)
	Text       string          `json:"text,omitempty"`
	Images     []wireImage     `json:"images,omitempty"`
	Files      []wireFile      `json:"files,omitempty"`
	Invocation *wireInvocation `json:"invocation,omitempty"`
	Usage      *wireUsage      `json:"usage,omitempty"`
	Final      bool            `json:"finalResponse,omitempty"`
	// Completed-response metadata. ModelName is the stable catalog display name;
	// Provider and Model keep the exact identity available to other clients.
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	ModelName string `json:"modelName,omitempty"`
	// queued-message metadata
	Delivery wireDeliveryMode `json:"delivery,omitempty"`
	Queued   bool             `json:"queued,omitempty"`
	// approval_request. Summary is the one-line label; Command carries the
	// complete shell command so the decision is never made against a truncated
	// view of what will run. CommandSegments is a conservative count of the
	// separate commands the shell would run.
	ID              string `json:"id,omitempty"`
	Summary         string `json:"summary,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Command         string `json:"command,omitempty"`
	CommandSegments int    `json:"commandSegments,omitempty"`
	// browser_request
	Disposition wireBrowserDisposition `json:"disposition,omitempty"`
	// browser inspection request
	TabID string `json:"tabID,omitempty"`
	// question_request
	Questions []wireQuestion `json:"questions,omitempty"`
	// title_update
	Title       string `json:"title,omitempty"`
	AITitle     string `json:"aiTitle,omitempty"`
	CustomTitle string `json:"customTitle,omitempty"`
	// title_generation_update
	TitleGenerationStatus      wireTitleGenerationStatus `json:"titleGenerationStatus,omitempty"`
	TitleGenerationProvider    string                    `json:"titleGenerationProvider,omitempty"`
	TitleGenerationModel       string                    `json:"titleGenerationModel,omitempty"`
	TitleGenerationErrorCode   string                    `json:"titleGenerationErrorCode,omitempty"`
	TitleGenerationError       string                    `json:"titleGenerationError,omitempty"`
	TitleGenerationAttemptedAt string                    `json:"titleGenerationAttemptedAt,omitempty"`
	// run timing
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	DurationMS  *int64 `json:"durationMs,omitempty"`
	// task_started / task_notification
	Task *wireBackgroundTask `json:"task,omitempty"`
}

type wireImage struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

type wireFile struct {
	Name     string `json:"name"`
	MIMEType string `json:"mimeType"`
	Size     int    `json:"size"`
}

type wirePreview struct {
	URL          string `json:"url,omitempty"`
	Path         string `json:"path,omitempty"`
	RelativePath string `json:"relativePath,omitempty"`
	Title        string `json:"title,omitempty"`
	GrantID      string `json:"grantID,omitempty"`
	PreviewPath  string `json:"previewPath,omitempty"`
}

type wireBrowserResult struct {
	Status       wireBrowserResultStatus `json:"status"`
	RequestedURL string                  `json:"requestedURL,omitempty"`
	CommittedURL string                  `json:"committedURL,omitempty"`
	Title        string                  `json:"title,omitempty"`
	Error        string                  `json:"error,omitempty"`
}

type wireBrowserInspectionResult struct {
	Status      wireBrowserInspectionStatus `json:"status"`
	URL         string                      `json:"url,omitempty"`
	Title       string                      `json:"title,omitempty"`
	PageStatus  wireBrowserPageStatus       `json:"pageStatus,omitempty"`
	Revision    int                         `json:"revision"`
	VisibleText string                      `json:"visibleText,omitempty"`
	Truncated   bool                        `json:"truncated,omitempty"`
	Error       string                      `json:"error,omitempty"`
}

type wireBrowserOpenTab struct {
	TabID  string               `json:"tabID"`
	URL    string               `json:"url,omitempty"`
	Title  string               `json:"title,omitempty"`
	Status wireBrowserTabStatus `json:"status"`
}

type wireBrowserControlledTab struct {
	TabID        string                         `json:"tabID"`
	Capabilities []wireBrowserControlCapability `json:"capabilities"`
}

type wireBrowserTabsResult struct {
	Status         wireBrowserTabsStatus      `json:"status"`
	OpenTabs       []wireBrowserOpenTab       `json:"openTabs,omitempty"`
	ControlledTabs []wireBrowserControlledTab `json:"controlledTabs,omitempty"`
	Selected       string                     `json:"selected,omitempty"`
	Error          string                     `json:"error,omitempty"`
}

type wireUsage struct {
	Input        int64         `json:"input"`
	InputUnknown bool          `json:"inputUnknown,omitempty"`
	Output       int64         `json:"output"`
	CacheRead    int64         `json:"cacheRead"`
	CacheWrite   int64         `json:"cacheWrite"`
	TotalTokens  int64         `json:"totalTokens"`
	Cost         wireUsageCost `json:"cost"`
}

type wireUsageCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

type wireContextUsage struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	UsedTokens    int64  `json:"usedTokens"`
	ContextWindow int64  `json:"contextWindow"`
	Measured      bool   `json:"measured"`
}

type wireBackgroundTask struct {
	ID          string         `json:"id"`
	Command     string         `json:"command"`
	Description string         `json:"description,omitempty"`
	Status      wireTaskStatus `json:"status"`
	OutputPath  string         `json:"outputPath"`
	ExitCode    *int           `json:"exitCode,omitempty"`
	StartedAt   string         `json:"startedAt"`
	CompletedAt string         `json:"completedAt,omitempty"`
}

type wireTaskOutputResponse struct {
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type wireHistoryResponse struct {
	Events                     []wireEvent               `json:"events"`
	Queue                      []wireEvent               `json:"queue"`
	Context                    wireContextUsage          `json:"context"`
	Tasks                      []wireBackgroundTask      `json:"tasks"`
	Running                    bool                      `json:"running"`
	EventSeq                   uint64                    `json:"eventSeq"`
	Title                      string                    `json:"title"`
	AITitle                    string                    `json:"aiTitle,omitempty"`
	CustomTitle                string                    `json:"customTitle,omitempty"`
	TitleGenerationStatus      wireTitleGenerationStatus `json:"titleGenerationStatus"`
	TitleGenerationProvider    string                    `json:"titleGenerationProvider,omitempty"`
	TitleGenerationModel       string                    `json:"titleGenerationModel,omitempty"`
	TitleGenerationErrorCode   string                    `json:"titleGenerationErrorCode,omitempty"`
	TitleGenerationError       string                    `json:"titleGenerationError,omitempty"`
	TitleGenerationAttemptedAt string                    `json:"titleGenerationAttemptedAt,omitempty"`
}
