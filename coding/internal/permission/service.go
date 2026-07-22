package permission

import (
	"context"
	"fmt"
	"sync"
)

// Service resolves tool effects, applies policy, and coordinates interactive
// approval without coupling the reusable agent loop to product permissions.
type Service struct {
	paths    PathResolver
	policyMu sync.RWMutex
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

// SetPolicy replaces the policy used by subsequent calls. Product code changes
// it only while a session is idle; the lock also keeps direct callers safe.
func (s *Service) SetPolicy(policy Policy) {
	if policy == nil {
		policy = DefaultPolicy
	}
	s.policyMu.Lock()
	s.policy = policy
	s.policyMu.Unlock()
}

func (s *Service) currentPolicy() Policy {
	s.policyMu.RLock()
	defer s.policyMu.RUnlock()
	return s.policy
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

	decision := s.currentPolicy()(req)
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

// PolicyForMode returns the baseline policy for one session permission mode.
func PolicyForMode(mode Mode) Policy {
	mode = NormalizeMode(mode)
	return func(req Request) Decision {
		return decideForMode(mode, req)
	}
}

// DefaultPolicy is the conservative ask-before-changes mode.
func DefaultPolicy(req Request) Decision {
	return decideForMode(ModeAsk, req)
}

func decideForMode(mode Mode, req Request) Decision {
	if len(req.Accesses) == 0 {
		if mode == ModeReadOnly {
			return Decision{Behavior: Deny, Reason: "tools without a declared access policy are blocked in read-only mode"}
		}
		return Decision{Behavior: Ask, Reason: "this tool has no declared access policy"}
	}
	decision := Decision{Behavior: Allow, Reason: "allowed by workspace policy"}
	for _, access := range req.Accesses {
		candidate := decideAccess(mode, access)
		if candidate.Behavior == Deny {
			return candidate
		}
		if candidate.Behavior == Ask && decision.Behavior == Allow {
			decision = candidate
		}
	}
	return decision
}

func decideAccess(mode Mode, access Access) Decision {
	switch access.Action {
	case Internal:
		return Decision{Behavior: Allow, Reason: "allowed internal session access"}
	case Read:
		if access.Location == Workspace {
			return Decision{Behavior: Allow, Reason: "allowed workspace read"}
		}
		if access.Location == OutsideWorkspace {
			return Decision{Behavior: Ask, Reason: "reading outside the workspace requires approval"}
		}
		return Decision{Behavior: Ask, Reason: "the read target could not be verified"}
	case Write:
		if mode == ModeReadOnly {
			return Decision{Behavior: Deny, Reason: "file changes are blocked in read-only mode"}
		}
		if access.Location == OutsideWorkspace {
			return Decision{Behavior: Ask, Reason: "writing outside the workspace requires approval"}
		}
		if access.Location == LocationUnknown {
			return Decision{Behavior: Ask, Reason: "the write target could not be verified"}
		}
		if mode == ModeAutoEdit {
			return Decision{Behavior: Allow, Reason: "workspace edits are enabled for this session"}
		}
		return Decision{Behavior: Ask, Reason: "file changes require approval"}
	case Execute:
		if mode == ModeReadOnly {
			return Decision{Behavior: Deny, Reason: "shell commands are blocked in read-only mode"}
		}
		return Decision{Behavior: Ask, Reason: "shell commands require approval"}
	default:
		if mode == ModeReadOnly {
			return Decision{Behavior: Deny, Reason: "unrecognized tool access is blocked in read-only mode"}
		}
		return Decision{Behavior: Ask, Reason: "this tool access is not recognized"}
	}
}
