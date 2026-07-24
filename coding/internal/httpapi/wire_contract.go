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
	wireEventApprovalRequest   wireEventType = "approval_request"
	wireEventApprovalResolved  wireEventType = "approval_resolved"
	wireEventApprovalCancelled wireEventType = "approval_cancelled"
	wireEventBrowserRequest    wireEventType = "browser_request"
	wireEventQueueCancelled    wireEventType = "queue_cancelled"
	wireEventQueueRemoved      wireEventType = "queue_removed"
	wireEventError             wireEventType = "error"
	wireEventDone              wireEventType = "done"
	wireEventSyncRequired      wireEventType = "sync_required"
	wireEventTitleUpdate       wireEventType = "title_update"
)

type wireDeltaKind string

const (
	wireDeltaText     wireDeltaKind = "text"
	wireDeltaThinking wireDeltaKind = "thinking"
)

type wireDeliveryMode string

const (
	wireDeliverySteer    wireDeliveryMode = "steer"
	wireDeliveryFollowUp wireDeliveryMode = "followup"
)

type wireBrowserDisposition string

const (
	wireBrowserReuseAgentTab    wireBrowserDisposition = "reuse_agent_tab"
	wireBrowserNewForegroundTab wireBrowserDisposition = "new_foreground_tab"
	wireBrowserNewBackgroundTab wireBrowserDisposition = "new_background_tab"
)

type wireBrowserResultStatus string

const (
	wireBrowserCommitted wireBrowserResultStatus = "committed"
	wireBrowserFailed    wireBrowserResultStatus = "failed"
	wireBrowserCancelled wireBrowserResultStatus = "cancelled"
	wireBrowserTimeout   wireBrowserResultStatus = "timeout"
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

// wireEvent is the JSON shape streamed to the browser. Fields are populated
// according to Type; the rest stay zero and are omitted.
type wireEvent struct {
	Type wireEventType `json:"type"`
	// delta events
	Kind  wireDeltaKind `json:"kind,omitempty"`
	Delta string        `json:"delta,omitempty"`
	// tool events (ID correlates tool_start with tool_end)
	Tool   string `json:"tool,omitempty"`
	Args   any    `json:"args,omitempty"`
	Result string `json:"result,omitempty"`
	// ToolContentIndex correlates tool argument events before every provider has
	// supplied a stable tool-call ID. Bytes is the size of one argument delta.
	ToolContentIndex *int `json:"toolContentIndex,omitempty"`
	Bytes            int  `json:"bytes,omitempty"`
	// Change is the structured file-change result or failure, when the tool
	// produced one, for rich rendering.
	Change  wireChange   `json:"change,omitempty"`
	Preview *wirePreview `json:"preview,omitempty"`
	IsError bool         `json:"isError,omitempty"`
	// message_end fallback text (used when nothing streamed)
	Text   string      `json:"text,omitempty"`
	Images []wireImage `json:"images,omitempty"`
	Usage  *wireUsage  `json:"usage,omitempty"`
	Final  bool        `json:"finalResponse,omitempty"`
	// Completed-response metadata. ModelName is the stable catalog display name;
	// Provider and Model keep the exact identity available to other clients.
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	ModelName string `json:"modelName,omitempty"`
	// queued-message metadata
	Delivery wireDeliveryMode `json:"delivery,omitempty"`
	Queued   bool             `json:"queued,omitempty"`
	// approval_request
	ID      string `json:"id,omitempty"`
	Summary string `json:"summary,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// browser_request
	Disposition wireBrowserDisposition `json:"disposition,omitempty"`
	// title_update
	Title       string `json:"title,omitempty"`
	AITitle     string `json:"aiTitle,omitempty"`
	CustomTitle string `json:"customTitle,omitempty"`
	// run timing
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	DurationMS  *int64 `json:"durationMs,omitempty"`
}

type wireImage struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
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

type wireUsage struct {
	Input       int64         `json:"input"`
	Output      int64         `json:"output"`
	CacheRead   int64         `json:"cacheRead"`
	CacheWrite  int64         `json:"cacheWrite"`
	TotalTokens int64         `json:"totalTokens"`
	Cost        wireUsageCost `json:"cost"`
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

type wireHistoryResponse struct {
	Events      []wireEvent      `json:"events"`
	Queue       []wireEvent      `json:"queue"`
	Context     wireContextUsage `json:"context"`
	Running     bool             `json:"running"`
	EventSeq    uint64           `json:"eventSeq"`
	Title       string           `json:"title"`
	AITitle     string           `json:"aiTitle,omitempty"`
	CustomTitle string           `json:"customTitle,omitempty"`
}
