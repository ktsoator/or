package permission

import (
	"context"
	"fmt"
	"sync"
)

// Service resolves tool effects, applies the session mode, and coordinates
// interactive approval without coupling the reusable agent loop to product
// permissions.
type Service struct {
	paths    PathResolver
	modeMu   sync.RWMutex
	mode     Mode
	approver Approver
}

// NewService creates one authorization service for a session workspace.
func NewService(workspace string, mode Mode, approver Approver) (*Service, error) {
	paths, err := NewPathResolver(workspace)
	if err != nil {
		return nil, err
	}
	return &Service{paths: paths, mode: NormalizeMode(mode), approver: approver}, nil
}

// SetMode changes the permission mode used by subsequent calls. Product code
// changes it only while a session is idle; the lock also keeps direct callers
// safe.
func (s *Service) SetMode(mode Mode) {
	s.modeMu.Lock()
	s.mode = NormalizeMode(mode)
	s.modeMu.Unlock()
}

func (s *Service) currentMode() Mode {
	s.modeMu.RLock()
	defer s.modeMu.RUnlock()
	return s.mode
}

// Authorize returns the final result for a validated tool call.
func (s *Service) Authorize(ctx context.Context, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{Reason: "tool approval was cancelled"}, err
	}
	resolved := make([]Access, len(req.Accesses))
	for i, access := range req.Accesses {
		resolved[i] = s.paths.Resolve(access)
	}
	req.Accesses = resolved

	reason := approvalReason(s.currentMode(), req)
	if reason == "" {
		return Result{Allowed: true}, nil
	}
	if s.approver == nil {
		return Result{Reason: "this tool requires approval, but no approver is configured"}, nil
	}

	response, err := s.approver.Decide(ctx, ApprovalRequest{Request: req, Reason: reason})
	if err != nil {
		return Result{Reason: "tool approval was cancelled"}, err
	}
	switch response.Choice {
	case AllowOnce:
		return Result{Allowed: true}, nil
	case Reject:
		return Result{Reason: "the user declined this action"}, nil
	default:
		return Result{Reason: fmt.Sprintf("invalid approval choice %q", response.Choice)}, nil
	}
}

func approvalReason(mode Mode, req Request) string {
	mode = NormalizeMode(mode)
	if mode == ModeFullAccess {
		return ""
	}
	if len(req.Accesses) == 0 {
		return "this tool has no declared access policy"
	}
	for _, access := range req.Accesses {
		if reason := accessApprovalReason(mode, access); reason != "" {
			return reason
		}
	}
	return ""
}

func accessApprovalReason(mode Mode, access Access) string {
	switch access.Action {
	case Internal:
		return ""
	case Read:
		// Checked before Location: a credentials file inside the workspace is
		// still a credentials file, and reading it copies secrets into the
		// model's context and the durable transcript.
		if access.Sensitive == SecretFile {
			return "reading a file that may hold credentials requires approval"
		}
		if access.Location == Workspace {
			return ""
		}
		if access.Location == OutsideWorkspace {
			return "reading outside the workspace requires approval"
		}
		return "the read target could not be verified"
	case Write:
		// Checked before the auto-edit allowance below. Enabling workspace edits
		// is consent to change the project's own files, not to rewrite its
		// credentials or the Git state that decides what later commands run.
		if access.Sensitive == SecretFile {
			return "changing a file that may hold credentials requires approval"
		}
		if access.Sensitive == RepositoryInternals {
			return "changing Git's internal state requires approval"
		}
		if access.Location == OutsideWorkspace {
			return "writing outside the workspace requires approval"
		}
		if access.Location == LocationUnknown {
			return "the write target could not be verified"
		}
		if mode == ModeAutoEdit {
			return ""
		}
		return "file changes require approval"
	case Execute:
		return "shell commands require approval"
	case Network:
		return "browser and network access requires approval"
	default:
		return "this tool access is not recognized"
	}
}
