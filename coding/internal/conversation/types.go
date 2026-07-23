package conversation

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

const (
	defaultTitle  = "New session"
	MaxTitleRunes = 120
	ScopeChat     = "chat"
	ScopeProject  = "project"
	KindScratch   = "scratch"
	KindFolder    = "folder"
)

// ErrSessionActive prevents deleting a conversation while its run or approval
// gate still owns live resources.
var ErrSessionActive = errors.New("session: session is running or waiting for approval")

// ErrImagesUnsupported rejects image attachments before a run is reserved
// when the session's selected model accepts text only.
var ErrImagesUnsupported = errors.New("session: selected model does not support images")

// ErrInvalidSessionScope reports a create request that is neither a standalone
// chat nor a project-backed conversation.
var ErrInvalidSessionScope = errors.New("session: invalid session scope")

// ErrInvalidPermissionMode reports a mode outside the product's supported
// session permission presets.
var ErrInvalidPermissionMode = errors.New("session: invalid permission mode")

// ErrManagerClosed rejects new work after product shutdown has started.
var ErrManagerClosed = errors.New("session: conversation manager is closed")

// Summary is the browser-facing metadata for one independent coding
// conversation. Runtime-only state is sampled when the list is requested.
type Summary struct {
	ID             string                 `json:"id"`
	Title          string                 `json:"title"`
	AITitle        string                 `json:"aiTitle,omitempty"`
	CustomTitle    string                 `json:"customTitle,omitempty"`
	WorkspacePath  string                 `json:"workspacePath"`
	WorkspaceName  string                 `json:"workspaceName"`
	Scope          string                 `json:"scope"`
	WorkspaceKind  string                 `json:"workspaceKind"`
	CreatedAt      time.Time              `json:"createdAt"`
	UpdatedAt      time.Time              `json:"updatedAt"`
	Running        bool                   `json:"running"`
	HasApproval    bool                   `json:"hasApproval"`
	ModelProvider  string                 `json:"modelProvider"`
	ModelID        string                 `json:"modelId"`
	ModelName      string                 `json:"modelName"`
	ThinkingLevel  llm.ModelThinkingLevel `json:"thinkingLevel"`
	PermissionMode permission.Mode        `json:"permissionMode"`
}

type record struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	AITitle        string    `json:"aiTitle,omitempty"`
	CustomTitle    string    `json:"customTitle,omitempty"`
	WorkspacePath  string    `json:"workspacePath,omitempty"`
	Scope          string    `json:"scope,omitempty"`
	WorkspaceKind  string    `json:"workspaceKind,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Transcript     string    `json:"transcript"`
	AutoTitle      bool      `json:"autoTitle,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	Model          string    `json:"model,omitempty"`
	Thinking       string    `json:"thinkingLevel,omitempty"`
	PermissionMode string    `json:"permissionMode,omitempty"`
}

type Runtime struct {
	record    record
	session   *engine.Session
	transport Transport
	running   atomic.Bool // reserves the session until EndRun completes
	live      atomic.Bool // state exposed to clients; clears before done is published

	pendingMu sync.Mutex
	pending   []QueuedMessage

	// titleGenerating is held only while an attempt is in flight, so a failed
	// attempt is retried when the next user message enters the session.
	titleGenerating atomic.Bool
}

type Delivery string

const (
	DeliverySteer    Delivery = "steer"
	DeliveryFollowUp Delivery = "followup"
)

// NewID returns an identifier for a session or a queued message.
func NewID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
