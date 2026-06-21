package github_test

import (
	"testing"

	ghprovider "github.com/itterno/bistro/internal/provider/github"
	"github.com/itterno/bistro/internal/provider"
)

func TestNewGraphQLClient_NonNilForToken(t *testing.T) {
	c := ghprovider.NewGraphQLClient("test-token")
	if c == nil {
		t.Error("NewGraphQLClient returned nil")
	}
}

func TestNewGraphQLClient_NonNilForEmptyToken(t *testing.T) {
	// Empty token should still return a client (unauthenticated), not panic.
	c := ghprovider.NewGraphQLClient("")
	if c == nil {
		t.Error("NewGraphQLClient returned nil for empty token")
	}
}

// Compile-time assertion: *github.Client must satisfy provider.ThreadResolver.
var _ provider.ThreadResolver = (*ghprovider.Client)(nil)

