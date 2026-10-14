package git

import (
	"errors"
	"regexp"
	"testing"
)

func TestLocationErrorPreservesCause(t *testing.T) {
	cause := errors.New("fetch failed")
	locationErr := &LocationError{RepoID: "github:owner/repo", Operation: "fetch", Code: "clone_failed", Message: "fetch failed", Err: cause}
	if !errors.Is(locationErr, cause) {
		t.Fatalf("LocationError lost the underlying cause: %v", locationErr)
	}
}

func TestParseRemoteURL_AcceptedGitHubForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want Identity
		id   string
	}{
		{
			name: "https URL",
			raw:  "https://github.com/acme/api",
			want: Identity{Provider: "github", Owner: "acme", Repo: "api"},
			id:   "github:acme/api",
		},
		{
			name: "https URL trims git suffix",
			raw:  "https://github.com/acme/api.git",
			want: Identity{Provider: "github", Owner: "acme", Repo: "api"},
			id:   "github:acme/api",
		},
		{
			name: "scp SSH URL",
			raw:  "git@github.com:acme/api",
			want: Identity{Provider: "github", Owner: "acme", Repo: "api"},
			id:   "github:acme/api",
		},
		{
			name: "scp SSH URL trims git suffix",
			raw:  "git@github.com:acme/api.git",
			want: Identity{Provider: "github", Owner: "acme", Repo: "api"},
			id:   "github:acme/api",
		},
		{
			name: "ssh URL",
			raw:  "ssh://git@github.com/acme/api",
			want: Identity{Provider: "github", Owner: "acme", Repo: "api"},
			id:   "github:acme/api",
		},
		{
			name: "ssh URL trims git suffix",
			raw:  "ssh://git@github.com/acme/api.git",
			want: Identity{Provider: "github", Owner: "acme", Repo: "api"},
			id:   "github:acme/api",
		},
		{
			name: "uppercase host owner and repo lowercased in ID",
			raw:  "https://GITHUB.com/Acme/API.git",
			want: Identity{Provider: "github", Owner: "Acme", Repo: "API"},
			id:   "github:acme/api",
		},
		{
			name: "path safe punctuation",
			raw:  "https://github.com/acme-inc/api.v2_test.git",
			want: Identity{Provider: "github", Owner: "acme-inc", Repo: "api.v2_test"},
			id:   "github:acme-inc/api.v2_test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseRemoteURL(tt.raw)
			if err != nil {
				t.Fatalf("ParseRemoteURL() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("ParseRemoteURL() = %#v, want %#v", got, tt.want)
			}

			if got.ID() != tt.id {
				t.Fatalf("Identity.ID() = %q, want %q", got.ID(), tt.id)
			}
		})
	}
}

func TestParseRemoteURL_RejectsInvalidRemotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "non github host", raw: "https://gitlab.com/acme/api.git"},
		{name: "one path part", raw: "https://github.com/acme.git"},
		{name: "three path parts", raw: "https://github.com/acme/api/extra.git"},
		{name: "dot owner", raw: "https://github.com/./api.git"},
		{name: "dot dot owner", raw: "https://github.com/../api.git"},
		{name: "repo contains path separator", raw: "git@github.com:acme/a/b.git"},
		{name: "repo contains percent encoded traversal", raw: "https://github.com/acme/%2e%2e.git"},
		{name: "owner contains non ascii", raw: "https://github.com/åcme/api.git"},
		{name: "scp missing colon separator", raw: "git@github.com/acme/api.git"},
		{name: "ssh user is not git", raw: "ssh://root@github.com/acme/api.git"},
		{name: "ssh missing git user", raw: "ssh://github.com/acme/api.git"},
		{name: "ssh contains password", raw: "ssh://git:pass@github.com/acme/api.git"},
		{name: "ssh contains explicit port", raw: "ssh://git@github.com:22/acme/api.git"},
		{name: "ssh contains empty explicit port", raw: "ssh://git@github.com:/acme/api.git"},
		{name: "https contains userinfo", raw: "https://user@github.com/acme/api.git"},
		{name: "https contains explicit port", raw: "https://github.com:443/acme/api.git"},
		{name: "https contains empty explicit port", raw: "https://github.com:/acme/api.git"},
		{name: "query is not allowed", raw: "https://github.com/acme/api?x=1"},
		{name: "fragment is not allowed", raw: "https://github.com/acme/api#frag"},
		{name: "trailing slash creates empty path segment", raw: "https://github.com/acme/api/"},
		{name: "unsupported scheme", raw: "ftp://github.com/acme/api.git"},
		{name: "missing host", raw: "https:///acme/api.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got, err := ParseRemoteURL(tt.raw); err == nil {
				t.Fatalf("ParseRemoteURL() = %#v, want error", got)
			}
		})
	}
}

func TestSafeRepoID_DeterministicAndHex(t *testing.T) {
	t.Parallel()

	id := Identity{Provider: "github", Owner: "Acme", Repo: "API"}
	first := SafeRepoID(id)
	second := SafeRepoID(id)

	if first != second {
		t.Fatalf("SafeRepoID() not deterministic: first %q, second %q", first, second)
	}

	if len(first) != 16 {
		t.Fatalf("SafeRepoID() length = %d, want 16", len(first))
	}

	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(first) {
		t.Fatalf("SafeRepoID() = %q, want 16 lowercase hex chars", first)
	}

	if first != "091fbf467097f82c" {
		t.Fatalf("SafeRepoID() = %q, want first 16 hex chars of sha256(Identity.ID())", first)
	}
}
