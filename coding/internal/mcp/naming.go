package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxProviderToolName = 64

// ToolName returns the provider-safe, deterministic name Or advertises for an
// MCP tool.
func ToolName(serverName, originalName string) string {
	raw := "mcp__" + sanitizeName(serverName) + "__" + sanitizeName(originalName)
	if len(raw) <= maxProviderToolName {
		return raw
	}
	hash := sha256.Sum256([]byte(raw))
	suffix := "_" + hex.EncodeToString(hash[:4])
	return raw[:maxProviderToolName-len(suffix)] + suffix
}

// DisplayTitle returns the MCP-standard display-name precedence for a tool.
func DisplayTitle(definition *protocol.Tool) string {
	if definition == nil {
		return ""
	}
	if title := strings.TrimSpace(definition.Title); title != "" {
		return title
	}
	if definition.Annotations != nil {
		if title := strings.TrimSpace(definition.Annotations.Title); title != "" {
			return title
		}
	}
	return definition.Name
}

func sanitizeName(value string) string {
	var result strings.Builder
	for _, char := range strings.TrimSpace(value) {
		if char <= unicode.MaxASCII && (unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' || char == '-') {
			result.WriteRune(char)
		} else {
			result.WriteByte('_')
		}
	}
	if result.Len() == 0 {
		return "unnamed"
	}
	return result.String()
}
