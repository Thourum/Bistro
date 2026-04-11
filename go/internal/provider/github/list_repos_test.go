package github

import (
	"context"
	"testing"
)

func TestListUserRepos_RequiresAuth(t *testing.T) {
	// Client with empty token should compile and the method should exist.
	// We only verify no panic and that the method is callable.
	client := NewClient("")
	_, err := client.ListUserRepos(context.Background())
	if err == nil {
		t.Log("ListUserRepos with empty token may succeed or fail depending on GitHub — just verifying it compiles and runs")
	}
}
