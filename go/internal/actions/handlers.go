// go/internal/actions/handlers.go
package actions

import (
	"context"
	"fmt"
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
