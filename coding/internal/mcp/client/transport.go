package client

import (
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

func buildTransport(config Config, workspace string) (protocol.Transport, error) {
	if strings.TrimSpace(config.Command) != "" {
		command, err := Expand(config.Command, workspace)
		if err != nil {
			return nil, err
		}
		args := make([]string, len(config.Args))
		for index, arg := range config.Args {
			args[index], err = Expand(arg, workspace)
			if err != nil {
				return nil, err
			}
		}
		cmd := exec.Command(command, args...)
		cwd := workspace
		if strings.TrimSpace(config.Cwd) != "" {
			cwd, err = Expand(config.Cwd, workspace)
			if err != nil {
				return nil, err
			}
			cwd, err = ExpandHome(cwd)
			if err != nil {
				return nil, err
			}
		}
		if !filepath.IsAbs(cwd) {
			cwd = filepath.Join(workspace, cwd)
		}
		cmd.Dir = filepath.Clean(cwd)
		cmd.Env, err = mergedEnvironment(config.Env, workspace)
		if err != nil {
			return nil, err
		}
		return &protocol.CommandTransport{Command: cmd}, nil
	}

	endpoint, err := Expand(config.URL, workspace)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid Streamable HTTP URL %q", endpoint)
	}
	headers := make(http.Header, len(config.Headers))
	for key, value := range config.Headers {
		expanded, err := Expand(value, workspace)
		if err != nil {
			return nil, err
		}
		headers.Set(key, expanded)
	}
	httpClient := &http.Client{Transport: &headerTransport{base: http.DefaultTransport, origin: parsed.Scheme + "://" + parsed.Host, headers: headers}}
	return &protocol.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient}, nil
}

type headerTransport struct {
	base    http.RoundTripper
	origin  string
	headers http.Header
}

func (transport *headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	if clone.URL.Scheme+"://"+clone.URL.Host == transport.origin {
		for key, values := range transport.headers {
			clone.Header.Del(key)
			for _, value := range values {
				clone.Header.Add(key, value)
			}
		}
	}
	return transport.base.RoundTrip(clone)
}

func transportName(config Config) string {
	if strings.TrimSpace(config.Command) != "" {
		return "stdio"
	}
	if strings.TrimSpace(config.URL) != "" {
		return "streamable_http"
	}
	return ""
}
