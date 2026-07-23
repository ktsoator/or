package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/provider"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

func init() {
	// Keep gin quiet and production-shaped; this is an app server, not a demo.
	gin.SetMode(gin.ReleaseMode)
}

// Server wires the multi-session API: session discovery plus scoped history,
// SSE, prompt, approval, and abort endpoints. The React application is
// built and deployed independently from this service.
// Each field is the store one group of routes actually reads. Handlers reach
// for the store they need and never through another component to find it.
type Server struct {
	ctx           context.Context
	conversations *conversation.Manager
	ledger        *usage.Store
	workspaces    *workspace.Registry
	registry      *llm.ProviderRegistry
	providers     *provider.Store
	browseRoot    string
	clientOrigin  string
}

// Options contains the product services exposed through HTTP. Construction
// belongs to internal/app; this package only translates between HTTP and those
// services.
type Options struct {
	Context       context.Context
	Conversations *conversation.Manager
	Ledger        *usage.Store
	Workspaces    *workspace.Registry
	Registry      *llm.ProviderRegistry
	Providers     *provider.Store
	BrowseRoot    string
	ClientOrigin  string
}

// NewServer builds the HTTP delivery layer from already-created services.
func NewServer(opts Options) *Server {
	return &Server{
		ctx:           opts.Context,
		conversations: opts.Conversations,
		ledger:        opts.Ledger,
		workspaces:    opts.Workspaces,
		registry:      opts.Registry,
		providers:     opts.Providers,
		browseRoot:    opts.BrowseRoot,
		clientOrigin:  opts.ClientOrigin,
	}
}

// Handler returns the HTTP handler for the coding API: a gin engine serving the
// /api routes, wrapped in the cross-origin gate for a separately deployed
// client.
//
// Each mount function owns one group of routes and lives in its own file. A new
// module is a new file plus one line here; nothing else in this package has to
// learn about it.
func (s *Server) Handler() http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())

	api := r.Group("/api")
	s.mountSessions(api)
	s.mountModels(api)
	s.mountProviders(api)
	s.mountWorkspaces(api)
	s.mountUsage(api)
	s.mountSkills(api)
	s.mountDirectories(api)
	s.mountPreview(api)

	return allowClientOrigin(r, s.clientOrigin, routedMethods(r))
}

// routedMethods reports every verb the mounted routes serve.
func routedMethods(r *gin.Engine) []string {
	methods := make([]string, 0, len(r.Routes()))
	for _, route := range r.Routes() {
		methods = append(methods, route.Method)
	}
	return methods
}

const (
	maxPromptImages       = 4
	maxPromptImageBytes   = 10 << 20
	maxPromptImagesBytes  = 20 << 20
	maxPromptRequestBytes = 28 << 20
)
