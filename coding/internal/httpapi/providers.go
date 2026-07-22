package httpapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/provider"
	"github.com/ktsoator/or/llm"
)

// mountProviders serves provider connections, credentials and model selection.
func (s *Server) mountProviders(r gin.IRouter) {
	r.GET("/providers", s.handleProviders)
	r.PUT("/model-selection", s.handleActivateModel)
	r.PUT("/providers/:providerID", s.handleSetProvider)
	r.PATCH("/providers/:providerID/active-connection", s.handleActivateProviderConnection)
	r.PATCH("/providers/:providerID/connections/:connectionID/active-key", s.handleActivateProviderKey)
	r.DELETE("/providers/:providerID", s.handleClearProvider)
}

// providerInfo is the browser-facing projection of the provider runtime and
// the coding product's saved connection profiles. Secrets are represented only
// by masked previews.
type providerInfo struct {
	ID                 string                   `json:"id"`
	Name               string                   `json:"name"`
	Configured         bool                     `json:"configured"`
	Models             int                      `json:"models"`
	OfficialBaseURL    string                   `json:"officialBaseURL,omitempty"`
	EffectiveBaseURL   string                   `json:"effectiveBaseURL,omitempty"`
	ActiveConnectionID string                   `json:"activeConnectionId"`
	Connections        []providerConnectionInfo `json:"connections"`
}

type providerConnectionInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	BaseURL     string            `json:"baseURL"`
	Official    bool              `json:"official"`
	ActiveKeyID string            `json:"activeKeyId,omitempty"`
	Keys        []providerKeyInfo `json:"keys"`
}

type providerKeyInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Preview string `json:"preview"`
}

type providerProfileRequest struct {
	ActiveConnectionID string                      `json:"activeConnectionId"`
	Connections        []providerConnectionRequest `json:"connections"`
}

type providerConnectionRequest struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	BaseURL     string               `json:"baseURL"`
	ActiveKeyID string               `json:"activeKeyId"`
	Keys        []providerKeyRequest `json:"keys"`
}

type providerKeyRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"apiKey"`
}

type providerListResponse struct {
	Providers   []providerInfo           `json:"providers"`
	ActiveModel *provider.ModelSelection `json:"activeModel,omitempty"`
}

func (s *Server) handleProviders(c *gin.Context) {
	snapshot := s.providers.Snapshot()
	out := make([]providerInfo, 0)
	for _, registered := range s.registry.Providers() {
		if info, ok := s.projectProviderInfo(registered.ID(), snapshot[registered.ID()]); ok {
			out = append(out, info)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Configured != out[j].Configured {
			return out[i].Configured
		}
		return out[i].Name < out[j].Name
	})
	c.Header("Cache-Control", "no-store")
	response := providerListResponse{Providers: out}
	if selection, ok := s.providers.ActiveModel(); ok {
		response.ActiveModel = &selection
	}
	c.JSON(http.StatusOK, response)
}

func (s *Server) projectProviderInfo(id string, profile provider.Profile) (providerInfo, bool) {
	registered, ok := s.registry.Get(id)
	if !ok {
		return providerInfo{}, false
	}
	status, _ := s.registry.AuthStatus(id, nil)
	models := runnableProviderModels(registered)
	if len(models) == 0 {
		models = registered.Models()
	}
	officialBaseURL := ""
	effectiveBaseURL := ""
	if len(models) > 0 {
		officialBaseURL = models[0].BaseURL
		resolvedModel, _ := s.registry.ResolveRequest(models[0], llm.StreamOptions{})
		effectiveBaseURL = resolvedModel.BaseURL
	}

	connections := make([]providerConnectionInfo, 0, len(profile.Connections))
	for _, connection := range profile.Connections {
		baseURL := connection.BaseURL
		official := connection.ID == provider.OfficialConnectionID
		if official {
			baseURL = officialBaseURL
		}
		info := providerConnectionInfo{
			ID:          connection.ID,
			Name:        connection.Name,
			BaseURL:     baseURL,
			Official:    official,
			ActiveKeyID: connection.ActiveKeyID,
			Keys:        make([]providerKeyInfo, 0, len(connection.Keys)),
		}
		for _, key := range connection.Keys {
			info.Keys = append(info.Keys, providerKeyInfo{
				ID:      key.ID,
				Name:    key.Name,
				Preview: maskAPIKey(key.APIKey),
			})
		}
		connections = append(connections, info)
	}
	return providerInfo{
		ID:                 id,
		Name:               status.Label,
		Configured:         status.Configured,
		Models:             len(models),
		OfficialBaseURL:    officialBaseURL,
		EffectiveBaseURL:   effectiveBaseURL,
		ActiveConnectionID: profile.ActiveConnectionID,
		Connections:        connections,
	}, true
}

func (s *Server) handleActivateModel(c *gin.Context) {
	var body provider.ModelSelection
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model selection"})
		return
	}
	selection, err := s.providers.ActivateModel(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, selection)
}

func runnableProviderModels(registered *llm.Provider) []llm.Model {
	models := registered.Models()
	runnable := make([]llm.Model, 0, len(models))
	for _, model := range models {
		if llm.SupportsProtocol(model.Protocol) {
			runnable = append(runnable, model)
		}
	}
	return runnable
}

func maskAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 8 {
		return "••••••••"
	}
	return string(runes[:3]) + "••••" + string(runes[len(runes)-4:])
}

func (s *Server) handleSetProvider(c *gin.Context) {
	id := c.Param("providerID")
	if _, ok := s.registry.AuthStatus(id, nil); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown provider"})
		return
	}
	var body providerProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider settings"})
		return
	}
	profile, err := s.providers.Save(id, body.update())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, _ := s.projectProviderInfo(id, profile)
	c.JSON(http.StatusOK, info)
}

func (s *Server) handleActivateProviderConnection(c *gin.Context) {
	id := c.Param("providerID")
	var body struct {
		ConnectionID string `json:"connectionId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection selection"})
		return
	}
	profile, err := s.providers.ActivateConnection(id, body.ConnectionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, _ := s.projectProviderInfo(id, profile)
	c.JSON(http.StatusOK, info)
}

func (s *Server) handleActivateProviderKey(c *gin.Context) {
	id := c.Param("providerID")
	connectionID := c.Param("connectionID")
	var body struct {
		KeyID string `json:"keyId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key selection"})
		return
	}
	profile, err := s.providers.ActivateKey(id, connectionID, body.KeyID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, _ := s.projectProviderInfo(id, profile)
	c.JSON(http.StatusOK, info)
}

func (request providerProfileRequest) update() provider.Update {
	update := provider.Update{ActiveConnectionID: request.ActiveConnectionID}
	for _, connection := range request.Connections {
		connectionUpdate := provider.ConnectionUpdate{
			ID:          connection.ID,
			Name:        connection.Name,
			BaseURL:     connection.BaseURL,
			ActiveKeyID: connection.ActiveKeyID,
		}
		for _, key := range connection.Keys {
			connectionUpdate.Keys = append(connectionUpdate.Keys, provider.KeyUpdate{
				ID:     key.ID,
				Name:   key.Name,
				APIKey: key.APIKey,
			})
		}
		update.Connections = append(update.Connections, connectionUpdate)
	}
	return update
}

func (s *Server) handleClearProvider(c *gin.Context) {
	id := c.Param("providerID")
	if err := s.providers.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not clear provider settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
