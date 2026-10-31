package github_test

import (
	"testing"

	"github.com/itterno/bistro/internal/provider"
	ghprovider "github.com/itterno/bistro/internal/provider/github"
)

func TestNewGraphQLClient_NonNilForToken(t *testing.T) {
	c := ghprovider.NewGraphQLClient("test-token", nil)
	if c == nil {
		t.Error("NewGraphQLClient returned nil")
	}
}

func TestNewGraphQLClient_NonNilForEmptyToken(t *testing.T) {
	// Empty token should still return a client (unauthenticated), not panic.
	c := ghprovider.NewGraphQLClient("", nil)
	if c == nil {
		t.Error("NewGraphQLClient returned nil for empty token")
	}
}

// Compile-time assertion: *github.Client must satisfy provider.ThreadResolver.
var _ provider.ThreadResolver = (*ghprovider.Client)(nil)
