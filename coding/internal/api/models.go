package api

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/llm"
)

type modelOption struct {
	Provider       string                   `json:"provider"`
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	ContextWindow  int64                    `json:"contextWindow"`
	ThinkingLevels []llm.ModelThinkingLevel `json:"thinkingLevels"`
	SupportsImages bool                     `json:"supportsImages"`
}

func (s *Server) handleModels(c *gin.Context) {
	includeCatalog := c.Query("scope") == "catalog"
	models := make([]modelOption, 0)
	for _, providerID := range llm.GetProviders() {
		if !includeCatalog && !s.providerAvailable(providerID) {
			continue
		}
		for _, model := range llm.GetRunnableModels(providerID) {
			name := model.Name
			if name == "" {
				name = model.ID
			}
			models = append(models, modelOption{
				Provider:       model.Provider,
				ID:             model.ID,
				Name:           name,
				ContextWindow:  model.ContextWindow,
				ThinkingLevels: llm.SupportedThinkingLevels(model),
				SupportsImages: slices.Contains(model.Input, llm.Image),
			})
		}
	}
	defaultProvider := ""
	defaultModelID := ""
	defaultThinking := llm.ModelThinkingOff
	if selection, ok := s.providers.ActiveModel(); ok && s.providerAvailable(selection.Provider) {
		if model, found := llm.LookupModel(selection.Provider, selection.Model); found {
			defaultProvider = model.Provider
			defaultModelID = model.ID
			defaultThinking = llm.ClampThinkingLevel(model, selection.ThinkingLevel)
		}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"models":               models,
		"defaultProvider":      defaultProvider,
		"defaultModel":         defaultModelID,
		"defaultThinkingLevel": defaultThinking,
	})
}

func (s *Server) providerAvailable(providerID string) bool {
	status, ok := s.registry.AuthStatus(providerID, nil)
	return ok && status.Configured
}

// mountModels serves the model catalog available to the browser.
func (s *Server) mountModels(r gin.IRouter) {
	r.GET("/models", s.handleModels)
}
