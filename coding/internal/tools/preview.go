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
	URL         string             `json:"url" jsonschema:"description=An HTTP(S) URL or absolute workspace HTML path to open in Coding's Browser view,minLength=1"`
	Title       string             `json:"title,omitempty" jsonschema:"description=A short title for the preview"`
	Disposition BrowserDisposition `json:"disposition,omitempty" jsonschema:"description=Where to open the page. Reuse the session Agent tab unless the user explicitly asks for a new or background tab,enum=reuse_agent_tab,enum=new_foreground_tab,enum=new_background_tab"`
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

// BrowserDisposition describes where a product shell should apply an agent
// navigation request. Reuse is the default; new tabs require an explicit user
// request surfaced by the model.
type BrowserDisposition string

const (
	BrowserReuseAgentTab    BrowserDisposition = "reuse_agent_tab"
	BrowserNewForegroundTab BrowserDisposition = "new_foreground_tab"
	BrowserNewBackgroundTab BrowserDisposition = "new_background_tab"
)

// BrowserResultStatus is a terminal browser-command outcome. A controller must
// return exactly one terminal result for each accepted request.
type BrowserResultStatus string

const (
	BrowserCommitted BrowserResultStatus = "committed"
	BrowserFailed    BrowserResultStatus = "failed"
	BrowserCancelled BrowserResultStatus = "cancelled"
	BrowserTimeout   BrowserResultStatus = "timeout"
)

// BrowserRequest is the validated navigation intent handed to the product
// transport. It contains no Electron view or renderer state.
type BrowserRequest struct {
	Preview     PreviewRequest
	Disposition BrowserDisposition
}

// BrowserResult is the product shell's terminal acknowledgement. RequestedURL
// and CommittedURL remain separate so redirects are visible to the model.
type BrowserResult struct {
	ID           string
	Status       BrowserResultStatus
	RequestedURL string
	CommittedURL string
	Title        string
	Error        string
}

// BrowserController delivers a navigation command to the product shell and
// waits for its terminal acknowledgement.
type BrowserController interface {
	OpenBrowser(context.Context, BrowserRequest) (BrowserResult, error)
}

// OpenPreview returns a product tool that asks a connected Coding client to
// display a web page or workspace HTML document. The tool does not claim
// success until the configured browser controller acknowledges the navigation.
func OpenPreview(root string, controllers ...BrowserController) Tool {
	var controller BrowserController
	if len(controllers) > 0 {
		controller = controllers[0]
	}
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
				if controller == nil {
					return textResult("Could not open preview: browser confirmation is unavailable"), nil
				}
				disposition, err := normalizeBrowserDisposition(in.Disposition)
				if err != nil {
					return textResult(fmt.Sprintf("Could not open preview: %v", err)), nil
				}
				result, err := controller.OpenBrowser(ctx, BrowserRequest{
					Preview:     preview,
					Disposition: disposition,
				})
				if err != nil {
					return agent.ToolResult{}, err
				}
				return browserToolResult(destination, preview, result), nil
			},
		},
		AccessFor:     previewAccess,
		PromptSnippet: openPreviewText.snippet,
		Guidelines:    openPreviewText.guidelines,
	}
}

func normalizeBrowserDisposition(disposition BrowserDisposition) (BrowserDisposition, error) {
	switch disposition {
	case "", BrowserReuseAgentTab:
		return BrowserReuseAgentTab, nil
	case BrowserNewForegroundTab, BrowserNewBackgroundTab:
		return disposition, nil
	default:
		return "", fmt.Errorf("browser disposition is invalid")
	}
}

func browserToolResult(destination string, preview PreviewRequest, result BrowserResult) agent.ToolResult {
	switch result.Status {
	case BrowserCommitted:
		committed := strings.TrimSpace(result.CommittedURL)
		if committed == "" {
			committed = destination
		}
		return resultWith(fmt.Sprintf("Opened preview at %s", committed), preview)
	case BrowserFailed:
		detail := strings.TrimSpace(result.Error)
		if detail == "" {
			detail = "navigation failed"
		}
		return textResult(fmt.Sprintf("Could not open preview at %s: %s", destination, detail))
	case BrowserTimeout:
		return textResult("The browser did not confirm the navigation")
	case BrowserCancelled:
		return textResult("The browser navigation was cancelled")
	default:
		return textResult("Could not open preview: browser returned an invalid result")
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
