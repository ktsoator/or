// Package provider manages the coding product's persisted provider connection
// profiles. The llm package remains the only provider runtime; this package
// selects one profile and projects it into llm.ProviderOverride.
package provider

import "github.com/ktsoator/or/llm"

const (
	fileVersion          = 4
	OfficialConnectionID = "official"
)

// Key is one locally stored credential. APIKey is write-only at the HTTP
// boundary and must never be returned to the browser.
type Key struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"apiKey"`
}

// Connection groups one endpoint with multiple named credentials. The
// official connection has no stored BaseURL; requests use the model catalog.
type Connection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	BaseURL     string `json:"baseURL,omitempty"`
	ActiveKeyID string `json:"activeKeyId,omitempty"`
	Keys        []Key  `json:"keys,omitempty"`
}

// Profile is the persisted configuration for one llm provider.
type Profile struct {
	ActiveConnectionID string       `json:"activeConnectionId"`
	Connections        []Connection `json:"connections"`
}

type profileFile struct {
	Version      int                    `json:"version"`
	ActiveModel  *ModelSelection        `json:"activeModel,omitempty"`
	UtilityModel *UtilityModelSelection `json:"utilityModel,omitempty"`
	Providers    map[string]Profile     `json:"providers"`
}

// ModelSelection is the application-wide model used for new conversations.
// It is deliberately absent on first launch; choosing a provider connection
// alone must never silently select a model.
type ModelSelection struct {
	Provider      string                 `json:"provider"`
	Model         string                 `json:"model"`
	ThinkingLevel llm.ModelThinkingLevel `json:"thinkingLevel"`
}

// UtilityModelSelection pins the small model used for lightweight product work
// such as session titles. It stores stable identities without copying the
// credential itself.
type UtilityModelSelection struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	ConnectionID string `json:"connectionId"`
	KeyID        string `json:"keyId"`
}

// SelectionRepair records one persisted model selection that was adjusted
// during startup because its route or thinking level is no longer available.
// It is kept in memory for the settings UI and is not written to providers.json.
type SelectionRepair struct {
	Target      SelectionRepairTarget `json:"target"`
	Reason      SelectionRepairReason `json:"reason"`
	Previous    ModelReference        `json:"previous"`
	Replacement *ModelReference       `json:"replacement,omitempty"`
}

type SelectionRepairTarget string

const (
	SelectionRepairActiveModel  SelectionRepairTarget = "active_model"
	SelectionRepairUtilityModel SelectionRepairTarget = "utility_model"
)

type SelectionRepairReason string

const (
	SelectionRepairUnavailable      SelectionRepairReason = "unavailable"
	SelectionRepairUnsupportedLevel SelectionRepairReason = "unsupported_thinking_level"
)

// ModelReference is the non-secret identity used in a selection repair.
type ModelReference struct {
	Provider      string                 `json:"provider"`
	Model         string                 `json:"model"`
	ThinkingLevel llm.ModelThinkingLevel `json:"thinkingLevel,omitempty"`
}

// ModelRoute is the public, secret-free identity of one resolved request path.
type ModelRoute struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	ConnectionID string `json:"connectionId"`
	KeyID        string `json:"keyId"`
}

// ResolvedModelRoute carries the catalog model and request-scoped connection
// settings used inside the product. It must never be serialized to the client.
type ResolvedModelRoute struct {
	Route   ModelRoute
	Model   llm.Model
	Options llm.StreamOptions
}

// Update describes an application-level profile change. Blank APIKey values
// preserve existing secrets with the same key ID.
type Update struct {
	ActiveConnectionID string
	Connections        []ConnectionUpdate
}

type ConnectionUpdate struct {
	ID          string
	Name        string
	BaseURL     string
	ActiveKeyID string
	Keys        []KeyUpdate
}

type KeyUpdate struct {
	ID     string
	Name   string
	APIKey string
}
