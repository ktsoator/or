package httpapi

import (
	"errors"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/mcpclient"
)

var mcpServerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type mcpServerInfo struct {
	Name       string                 `json:"name"`
	Config     mcpclient.ServerConfig `json:"config"`
	InScope    *bool                  `json:"inScope,omitempty"`
	Diagnostic string                 `json:"diagnostic,omitempty"`
}

type mcpListResponse struct {
	Path    string          `json:"path"`
	Servers []mcpServerInfo `json:"servers"`
}

type mcpServerSaveRequest struct {
	Name         string                 `json:"name"`
	PreviousName string                 `json:"previousName,omitempty"`
	Config       mcpclient.ServerConfig `json:"config"`
}

func (s *Server) handleMCPServers(c *gin.Context) {
	if s.mcpConfigPath == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP configuration is unavailable"})
		return
	}
	s.mcpConfigMu.Lock()
	config, err := readMCPConfig(s.mcpConfigPath)
	s.mcpConfigMu.Unlock()
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error(), "path": s.mcpConfigPath})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, projectMCPConfig(s.mcpConfigPath, c.Query("workspace"), config))
}

func (s *Server) handleSaveMCPServer(c *gin.Context) {
	if s.mcpConfigPath == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP configuration is unavailable"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	var request mcpServerSaveRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid MCP server settings"})
		return
	}
	if !mcpServerName.MatchString(request.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server name must use 1-64 letters, numbers, dots, dashes, or underscores"})
		return
	}
	if request.PreviousName != "" && !mcpServerName.MatchString(request.PreviousName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid previous server name"})
		return
	}
	if err := request.Config.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.mcpConfigMu.Lock()
	defer s.mcpConfigMu.Unlock()
	config, err := readMCPConfig(s.mcpConfigPath)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	_, targetExists := config.MCPServers[request.Name]
	if request.PreviousName == "" {
		if targetExists {
			c.JSON(http.StatusConflict, gin.H{"error": "an MCP server with this name already exists"})
			return
		}
	} else {
		if _, exists := config.MCPServers[request.PreviousName]; !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
			return
		}
		if request.PreviousName != request.Name && targetExists {
			c.JSON(http.StatusConflict, gin.H{"error": "an MCP server with this name already exists"})
			return
		}
		delete(config.MCPServers, request.PreviousName)
	}
	config.MCPServers[request.Name] = request.Config
	if err := mcpclient.WriteConfig(s.mcpConfigPath, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, projectMCPServer(request.Name, request.Config, c.Query("workspace")))
}

func (s *Server) handleDeleteMCPServer(c *gin.Context) {
	if s.mcpConfigPath == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP configuration is unavailable"})
		return
	}
	name := c.Param("name")
	if !mcpServerName.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid MCP server name"})
		return
	}
	s.mcpConfigMu.Lock()
	defer s.mcpConfigMu.Unlock()
	config, err := readMCPConfig(s.mcpConfigPath)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if _, exists := config.MCPServers[name]; !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	}
	delete(config.MCPServers, name)
	if err := mcpclient.WriteConfig(s.mcpConfigPath, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleTestMCPServer(c *gin.Context) {
	if s.mcpConfigPath == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP configuration is unavailable"})
		return
	}
	name := c.Param("name")
	s.mcpConfigMu.Lock()
	config, err := readMCPConfig(s.mcpConfigPath)
	s.mcpConfigMu.Unlock()
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	server, exists := config.MCPServers[name]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	}
	started := time.Now()
	result, err := mcpclient.Probe(c.Request.Context(), name, server, c.Query("workspace"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"transport": result.Transport,
		"tools":     result.Tools,
		"latencyMs": time.Since(started).Milliseconds(),
	})
}

func readMCPConfig(path string) (mcpclient.Config, error) {
	config, err := mcpclient.ReadConfig(path)
	if errors.Is(err, os.ErrNotExist) {
		return mcpclient.Config{Version: 1, MCPServers: make(map[string]mcpclient.ServerConfig)}, nil
	}
	return config, err
}

func projectMCPConfig(path, workspace string, config mcpclient.Config) mcpListResponse {
	names := make([]string, 0, len(config.MCPServers))
	for name := range config.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	response := mcpListResponse{Path: path, Servers: make([]mcpServerInfo, 0, len(names))}
	for _, name := range names {
		response.Servers = append(response.Servers, projectMCPServer(name, config.MCPServers[name], workspace))
	}
	return response
}

func projectMCPServer(name string, config mcpclient.ServerConfig, workspace string) mcpServerInfo {
	info := mcpServerInfo{Name: name, Config: config}
	if err := config.Validate(); err != nil {
		info.Diagnostic = err.Error()
	}
	if strings.TrimSpace(workspace) != "" {
		applies, err := config.AppliesTo(workspace)
		if err != nil {
			info.Diagnostic = err.Error()
		} else {
			info.InScope = &applies
		}
	}
	return info
}

func (s *Server) mountMCP(r gin.IRouter) {
	r.GET("/mcp", s.handleMCPServers)
	r.PUT("/mcp/servers", s.handleSaveMCPServer)
	r.DELETE("/mcp/servers/:name", s.handleDeleteMCPServer)
	r.POST("/mcp/servers/:name/test", s.handleTestMCPServer)
}
