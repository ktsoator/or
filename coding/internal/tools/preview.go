package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

type openPreviewArgs struct {
	URL   string `json:"url" jsonschema:"description=An HTTP(S) URL or absolute workspace HTML path to open in Coding's Browser view,minLength=1"`
	Title string `json:"title,omitempty" jsonschema:"description=A short title for the preview"`
}

// PreviewRequest is the structured UI intent emitted by open_preview. Product
// shells act on it live, and the details sidecar retains it so a reopened
// conversation can offer the same preview again.
type PreviewRequest struct {
	URL          string
	Path         string
	RelativePath string
	Title        string
}

// OpenPreview returns a product tool that asks a connected Coding client to
// display a web page or workspace HTML document. The tool does not open a browser itself;
// its structured result travels over the session's existing tool event stream.
func OpenPreview(root string) Tool {
	def := llm.MustTool[openPreviewArgs]("open_preview", openPreviewText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Open preview",
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in openPreviewArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				preview, err := resolvePreviewRequest(ctx, root, in.URL)
				if err != nil {
					return textResult(fmt.Sprintf("Could not open preview: %v", err)), nil
				}
				preview.Title = strings.TrimSpace(in.Title)
				destination := preview.URL
				if preview.Path != "" {
					destination = preview.Path
				}
				return resultWith(
					fmt.Sprintf("Opened preview at %s", destination),
					preview,
				), nil
			},
		},
		AccessFor:     previewAccess,
		PromptSnippet: openPreviewText.snippet,
		Guidelines:    openPreviewText.guidelines,
	}
}

func resolvePreviewRequest(ctx context.Context, root, raw string) (PreviewRequest, error) {
	input := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(input), "http://") || strings.HasPrefix(strings.ToLower(input), "https://") {
		normalized, err := normalizeWebURL(input)
		if err == nil && isLocalPreviewURL(normalized) {
			normalized, err = CheckPreview(ctx, normalized)
		}
		return PreviewRequest{URL: normalized}, err
	}

	path, err := previewInputPath(input)
	if err != nil {
		return PreviewRequest{}, err
	}
	absolute, relative, err := ResolvePreviewDocument(root, path)
	if err != nil {
		return PreviewRequest{}, err
	}
	return PreviewRequest{Path: absolute, RelativePath: relative}, nil
}

func previewInputPath(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("provide a workspace HTML path or HTTP(S) URL")
	}
	if !strings.HasPrefix(strings.ToLower(input), "file:") {
		if strings.Contains(input, "://") {
			return "", fmt.Errorf("preview source must be a workspace HTML path or use http, https, or file")
		}
		return input, nil
	}

	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme != "file" {
		return "", fmt.Errorf("invalid file URL")
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		return "", fmt.Errorf("file URL must refer to a local path")
	}
	path, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil || path == "" {
		return "", fmt.Errorf("invalid file URL")
	}
	return filepath.FromSlash(path), nil
}

func previewAccess(args map[string]any) []permission.Access {
	source, _ := args["url"].(string)
	source = strings.TrimSpace(source)
	if source == "" || strings.HasPrefix(strings.ToLower(source), "http://") || strings.HasPrefix(strings.ToLower(source), "https://") {
		return InternalAccess(args)
	}
	path, err := previewInputPath(source)
	if err != nil {
		return InternalAccess(args)
	}
	return []permission.Access{{Action: permission.Read, Path: path}}
}

// ResolvePreviewDocument validates an HTML entry point and returns its
// canonical absolute path plus a workspace-relative URL path.
func ResolvePreviewDocument(root, path string) (string, string, error) {
	absolute, relative, err := resolvePreviewFile(root, path)
	if err != nil {
		return "", "", err
	}
	extension := strings.ToLower(filepath.Ext(absolute))
	if extension != ".html" && extension != ".htm" {
		return "", "", fmt.Errorf("preview file must be an HTML document")
	}
	return absolute, relative, nil
}

// ResolvePreviewAsset validates one file requested by a workspace preview.
func ResolvePreviewAsset(root, path string) (string, string, error) {
	return resolvePreviewFile(root, path)
}

func resolvePreviewFile(root, path string) (string, string, error) {
	resolver, err := permission.NewPathResolver(root)
	if err != nil {
		return "", "", err
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve preview workspace: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(rootPath); resolveErr == nil {
		rootPath = resolved
	}

	resolveTarget := func(target string) (string, error) {
		access := resolver.Resolve(permission.Access{Action: permission.Read, Path: target})
		if access.Location != permission.Workspace || access.ResolvedPath == "" {
			return "", fmt.Errorf("preview file must be inside the workspace")
		}
		return access.ResolvedPath, nil
	}

	target, err := resolveTarget(resolve(root, strings.TrimSpace(path)))
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", "", fmt.Errorf("preview file is not available: %w", err)
	}
	if info.IsDir() {
		target, err = resolveTarget(filepath.Join(target, "index.html"))
		if err != nil {
			return "", "", err
		}
		info, err = os.Stat(target)
		if err != nil {
			return "", "", fmt.Errorf("preview directory has no index.html: %w", err)
		}
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("preview path is not a regular file")
	}
	relative, err := filepath.Rel(rootPath, target)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("preview file must be inside the workspace")
	}
	return target, filepath.ToSlash(relative), nil
}

// CheckPreview validates and probes a local preview URL before it is handed to
// a WebView. Both the agent tool and the Browser HTTP endpoint use this path.
func CheckPreview(ctx context.Context, raw string) (string, error) {
	normalized, err := normalizePreviewURL(raw)
	if err != nil {
		return "", err
	}
	if err := probePreviewURL(ctx, normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func probePreviewURL(ctx context.Context, address string) error {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, address, nil)
	if err != nil {
		return fmt.Errorf("invalid preview request")
	}
	request.Header.Set("User-Agent", "Coding-Preview/1.0")
	client := &http.Client{
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			_, err := normalizePreviewURL(request.URL.String())
			return err
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("local server is not reachable at %s; start it and try again", address)
	}
	response.Body.Close()
	return nil
}

func normalizePreviewURL(raw string) (string, error) {
	normalized, err := normalizeWebURL(raw)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid preview URL")
	}

	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	canonicalHost := hostname
	switch hostname {
	case "localhost", "127.0.0.1", "::1":
	case "0.0.0.0":
		canonicalHost = "127.0.0.1"
	case "::":
		canonicalHost = "::1"
	default:
		return "", fmt.Errorf("preview URL must point to localhost")
	}

	if canonicalHost != hostname {
		if port := parsed.Port(); port != "" {
			parsed.Host = net.JoinHostPort(canonicalHost, port)
		} else if strings.Contains(canonicalHost, ":") {
			parsed.Host = "[" + canonicalHost + "]"
		} else {
			parsed.Host = canonicalHost
		}
	}
	return parsed.String(), nil
}

func normalizeWebURL(raw string) (string, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("provide a complete URL such as https://example.com")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("preview URL must use http or https")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("preview URL cannot include credentials")
	}
	return parsed.String(), nil
}

func isLocalPreviewURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSuffix(parsed.Hostname(), ".")) {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "::":
		return true
	default:
		return false
	}
}
