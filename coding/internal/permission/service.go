package permission

import (
	"context"
	"fmt"
)

// Service resolves tool effects, applies policy, and coordinates interactive
// approval without coupling the reusable agent loop to product permissions.
type Service struct {
	paths    PathResolver
	policy   Policy
	approver Approver
}

// NewService creates one authorization service for a session workspace.
func NewService(workspace string, policy Policy, approver Approver) (*Service, error) {
	paths, err := NewPathResolver(workspace)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		policy = DefaultPolicy
	}
	return &Service{paths: paths, policy: policy, approver: approver}, nil
}

// Authorize returns the final decision for a validated tool call.
func (s *Service) Authorize(ctx context.Context, req Request) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{Behavior: Deny, Reason: "tool approval was cancelled"}, err
	}
	resolved := make([]Access, len(req.Accesses))
	for i, access := range req.Accesses {
		resolved[i] = s.paths.Resolve(access)
	}
	req.Accesses = resolved

	decision := s.policy(req)
	if decision.Behavior != Ask {
		return decision, nil
	}
	if s.approver == nil {
		return Decision{
			Behavior: Deny,
			Reason:   "this tool requires approval, but no approver is configured",
		}, nil
	}

	response, err := s.approver.Decide(ctx, ApprovalRequest{Request: req, Reason: decision.Reason})
	if err != nil {
		return Decision{Behavior: Deny, Reason: "tool approval was cancelled"}, err
	}
	switch response.Choice {
	case AllowOnce:
		return Decision{Behavior: Allow, Reason: "allowed once by the user"}, nil
	case Reject:
		return Decision{Behavior: Deny, Reason: "the user declined this action"}, nil
	default:
		return Decision{Behavior: Deny, Reason: fmt.Sprintf("invalid approval choice %q", response.Choice)}, nil
	}
}

// DefaultPolicy allows only internal operations and reads proven to remain
// inside the workspace. Mutations, commands, external reads, and unknown calls
// require explicit approval.
func DefaultPolicy(req Request) Decision {
	if len(req.Accesses) == 0 {
		return Decision{Behavior: Ask, Reason: "this tool has no declared access policy"}
	}
	for _, access := range req.Accesses {
		switch access.Action {
		case Internal:
			continue
		case Read:
			if access.Location == Workspace {
				continue
			}
			if access.Location == OutsideWorkspace {
				return Decision{Behavior: Ask, Reason: "reading outside the workspace requires approval"}
			}
			return Decision{Behavior: Ask, Reason: "the read target could not be verified"}
		case Write:
			if access.Location == OutsideWorkspace {
				return Decision{Behavior: Ask, Reason: "writing outside the workspace requires approval"}
			}
			if access.Location == LocationUnknown {
				return Decision{Behavior: Ask, Reason: "the write target could not be verified"}
			}
			return Decision{Behavior: Ask, Reason: "file changes require approval"}
		case Execute:
			return Decision{Behavior: Ask, Reason: "shell commands require approval"}
		default:
			return Decision{Behavior: Ask, Reason: "this tool access is not recognized"}
		}
	}
	return Decision{Behavior: Allow, Reason: "allowed by workspace policy"}
}
