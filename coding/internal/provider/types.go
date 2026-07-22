// Package provider manages the coding product's persisted provider connection
// profiles. The llm package remains the only provider runtime; this package
// selects one profile and projects it into llm.ProviderOverride.
package provider

import "github.com/ktsoator/or/llm"

const (
	fileVersion          = 2
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
	Version     int                `json:"version"`
	ActiveModel *ModelSelection    `json:"activeModel,omitempty"`
	Providers   map[string]Profile `json:"providers"`
}

// ModelSelection is the application-wide model used for new conversations.
// It is deliberately absent on first launch; choosing a provider connection
// alone must never silently select a model.
type ModelSelection struct {
	Provider      string                 `json:"provider"`
	Model         string                 `json:"model"`
	ThinkingLevel llm.ModelThinkingLevel `json:"thinkingLevel"`
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
