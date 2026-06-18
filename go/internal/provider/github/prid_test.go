package github

import (
	"testing"
)

// TestParsePRID_Valid covers the happy path of parsePRID.
func TestParsePRID_Valid(t *testing.T) {
	cases := []struct {
		id             string
		wantOwner      string
		wantRepo       string
		wantNumber     int
	}{
		{"github:myorg/myrepo:42", "myorg", "myrepo", 42},
		{"github:a/b:1", "a", "b", 1},
		{"github:owner/my-repo:9999", "owner", "my-repo", 9999},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			owner, repo, number, err := parsePRID(tc.id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tc.wantOwner {
				t.Errorf("owner: want %q, got %q", tc.wantOwner, owner)
			}
			if repo != tc.wantRepo {
				t.Errorf("repo: want %q, got %q", tc.wantRepo, repo)
			}
			if number != tc.wantNumber {
				t.Errorf("number: want %d, got %d", tc.wantNumber, number)
			}
		})
	}
}

// TestParsePRID_Invalid covers all malformed ID cases.
func TestParsePRID_Invalid(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{"wrong provider", "gitlab:org/repo:1"},
		{"missing number", "github:org/repo"},
		{"empty owner", "github:/repo:1"},
		{"empty repo", "github:owner/:1"},
		{"non-numeric number", "github:org/repo:abc"},
		{"zero number", "github:org/repo:0"},
		{"negative number", "github:org/repo:-1"},
		{"empty string", ""},
		{"no colons", "githuborgrepo1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := parsePRID(tc.id)
			if err == nil {
				t.Errorf("expected error for ID %q, got nil", tc.id)
			}
		})
	}
}
