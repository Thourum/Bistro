// go/internal/actions/handlers.go
package actions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/provider"
)

// Handler defines the interface for action handlers.
type Handler interface {
	Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error)
}

// ApproveHandler executes approve action via the provider.
type ApproveHandler struct {
	provider provider.Provider
}

// NewApproveHandler creates a new ApproveHandler.
func NewApproveHandler(prov provider.Provider) *ApproveHandler {
	return &ApproveHandler{provider: prov}
}

// Handle executes the approve action. Errors are returned inside ActionResult, not as the error return.
func (ah *ApproveHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	result := &ActionResult{
		PRID:      req.PRID,
		Action:    ApproveCommand,
		Timestamp: time.Now(),
	}

	if err := ah.provider.Approve(ctx, req.PRID); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to approve: %v", err)
		return result, nil
	}

	result.Success = true
	return result, nil
}

// RequestChangesHandler executes request_changes action via the provider.
type RequestChangesHandler struct {
	provider provider.Provider
}

// NewRequestChangesHandler creates a new RequestChangesHandler.
func NewRequestChangesHandler(prov provider.Provider) *RequestChangesHandler {
	return &RequestChangesHandler{provider: prov}
}

// Handle executes the request changes action. Body is required; missing body returns a failed ActionResult.
func (rch *RequestChangesHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	result := &ActionResult{
		PRID:      req.PRID,
		Action:    RequestChangesCommand,
		Timestamp: time.Now(),
	}

	if req.Body == "" {
		result.Success = false
		result.Error = "Body is required for request_changes"
		return result, nil
	}

	if err := rch.provider.RequestChanges(ctx, req.PRID, req.Body); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to request changes: %v", err)
		return result, nil
	}

	result.Success = true
	return result, nil
}

// PostCommentHandler executes post_comment action via the provider.
type PostCommentHandler struct {
	provider provider.Provider
}

// NewPostCommentHandler creates a new PostCommentHandler.
func NewPostCommentHandler(prov provider.Provider) *PostCommentHandler {
	return &PostCommentHandler{provider: prov}
}

// Handle executes the post comment action. Body is required; missing body returns a failed ActionResult.
func (pch *PostCommentHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	result := &ActionResult{
		PRID:      req.PRID,
		Action:    PostCommentCommand,
		Timestamp: time.Now(),
	}

	if req.Body == "" {
		result.Success = false
		result.Error = "Body is required for post_comment"
		return result, nil
	}

	if err := pch.provider.PostComment(ctx, req.PRID, req.Line, req.Body); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to post comment: %v", err)
		return result, nil
	}

	result.Success = true
	return result, nil
}

// isPermanentThreadError reports whether the error indicates a permanent
// failure (thread not found / already in desired state) that should not be
// retried. Only 404/not-found and already-resolved/already-unresolved errors
// are treated as permanent.
func isPermanentThreadError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, kw := range []string{"not found", "404", "could not resolve", "not resolve"} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// ResolveThreadHandler executes resolve_thread via the ThreadResolver capability.
type ResolveThreadHandler struct {
	provider provider.Provider
}

// NewResolveThreadHandler creates a ResolveThreadHandler.
func NewResolveThreadHandler(prov provider.Provider) *ResolveThreadHandler {
	return &ResolveThreadHandler{provider: prov}
}

// Handle resolves the thread identified by req.Body (thread node ID).
// Returns a permanent failure if the provider does not support ThreadResolver
// or if the thread is not found (no retry). Transient errors are returned
// with a retryable error message.
func (h *ResolveThreadHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	result := &ActionResult{
		PRID:      req.PRID,
		Action:    ResolveThreadCommand,
		Timestamp: time.Now(),
	}
	resolver, ok := h.provider.(provider.ThreadResolver)
	if !ok {
		result.Success = false
		result.Error = "provider does not support thread resolution (403 permanent)"
		return result, nil
	}
	if err := resolver.ResolveThread(ctx, req.Body); err != nil {
		result.Success = false
		if isPermanentThreadError(err) {
			result.Error = fmt.Sprintf("permanent: failed to resolve thread: %v", err)
		} else {
			result.Error = fmt.Sprintf("failed to resolve thread: %v", err)
		}
		return result, nil
	}
	result.Success = true
	return result, nil
}

// UnresolveThreadHandler executes unresolve_thread via the ThreadResolver capability.
type UnresolveThreadHandler struct {
	provider provider.Provider
}

// NewUnresolveThreadHandler creates an UnresolveThreadHandler.
func NewUnresolveThreadHandler(prov provider.Provider) *UnresolveThreadHandler {
	return &UnresolveThreadHandler{provider: prov}
}

// Handle unresolves the thread identified by req.Body (thread node ID).
func (h *UnresolveThreadHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	result := &ActionResult{
		PRID:      req.PRID,
		Action:    UnresolveThreadCommand,
		Timestamp: time.Now(),
	}
	resolver, ok := h.provider.(provider.ThreadResolver)
	if !ok {
		result.Success = false
		result.Error = "provider does not support thread resolution (403 permanent)"
		return result, nil
	}
	if err := resolver.UnresolveThread(ctx, req.Body); err != nil {
		result.Success = false
		if isPermanentThreadError(err) {
			result.Error = fmt.Sprintf("permanent: failed to unresolve thread: %v", err)
		} else {
			result.Error = fmt.Sprintf("failed to unresolve thread: %v", err)
		}
		return result, nil
	}
	result.Success = true
	return result, nil
}
