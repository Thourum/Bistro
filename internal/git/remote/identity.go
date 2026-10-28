package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

const (
	githubProvider = "github"
	githubHost     = "github.com"
	// SafeIDLength is the number of hex characters in a safe repository ID.
	SafeIDLength = 16
)

// Identity is a normalized repo identity.
type Identity struct {
	Provider string
	Owner    string
	Repo     string
}

// LocationError is a structured repo-location operation error.
type LocationError struct {
	RepoID    string
	Operation string
	Code      string
	Message   string
	Err       error
}

// Error returns the user-facing location failure message.
func (e *LocationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Unwrap preserves the underlying cause for errors.Is and errors.As.
func (e *LocationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ID returns the canonical lowercased repo identity.
func (i Identity) ID() string {
	return strings.ToLower(fmt.Sprintf("%s:%s/%s", i.Provider, i.Owner, i.Repo))
}

// ParseURL normalizes accepted GitHub remote URL forms.
func ParseURL(raw string) (Identity, error) {
	remote := strings.TrimSpace(raw)
	if remote == "" {
		return Identity{}, fmt.Errorf("parse remote URL: empty remote")
	}

	parsed, err := url.Parse(rewriteSCPRemote(remote))
	if err != nil {
		return Identity{}, fmt.Errorf("parse remote URL %q: %w", raw, err)
	}

	if parsed.Scheme != "https" && parsed.Scheme != "ssh" {
		return Identity{}, fmt.Errorf("parse remote URL %q: unsupported scheme %q", raw, parsed.Scheme)
	}

	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return Identity{}, fmt.Errorf("parse remote URL %q: query and fragment are not allowed", raw)
	}

	if strings.ToLower(parsed.Hostname()) != githubHost {
		return Identity{}, fmt.Errorf("parse remote URL %q: unsupported host %q", raw, parsed.Hostname())
	}

	if err := validateRemoteAuthority(parsed, raw); err != nil {
		return Identity{}, err
	}

	parts := strings.Split(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/")
	if len(parts) != 2 {
		return Identity{}, fmt.Errorf("parse remote URL %q: expected owner and repo path", raw)
	}

	owner, err := validateRemotePart(parts[0], "owner")
	if err != nil {
		return Identity{}, fmt.Errorf("parse remote URL %q: %w", raw, err)
	}

	repoPath := strings.TrimSuffix(parts[1], ".git")
	repo, err := validateRemotePart(repoPath, "repo")
	if err != nil {
		return Identity{}, fmt.Errorf("parse remote URL %q: %w", raw, err)
	}

	return Identity{Provider: githubProvider, Owner: owner, Repo: repo}, nil
}

// SafeRepoID returns the first 16 hex chars of sha256 of Identity.ID().
func SafeRepoID(id Identity) string {
	sum := sha256.Sum256([]byte(id.ID()))
	return hex.EncodeToString(sum[:])[:SafeIDLength]
}

func rewriteSCPRemote(raw string) string {
	if strings.HasPrefix(raw, "git@") && !strings.Contains(raw, "://") {
		separator := strings.Index(raw, ":")
		if separator <= len("git@") || separator == len(raw)-1 {
			return raw
		}

		return "ssh://git@" + raw[len("git@"):separator] + "/" + raw[separator+1:]
	}

	return raw
}

func validateRemoteAuthority(parsed *url.URL, raw string) error {
	if parsed.Port() != "" || strings.Contains(hostWithoutUserinfo(parsed.Host), ":") {
		return fmt.Errorf("parse remote URL %q: explicit port is not allowed", raw)
	}

	if parsed.Scheme == "https" && parsed.User != nil {
		return fmt.Errorf("parse remote URL %q: https userinfo is not allowed", raw)
	}

	if parsed.Scheme == "ssh" {
		if parsed.User == nil || parsed.User.Username() != "git" {
			return fmt.Errorf("parse remote URL %q: ssh user must be git", raw)
		}

		if _, hasPassword := parsed.User.Password(); hasPassword {
			return fmt.Errorf("parse remote URL %q: ssh password is not allowed", raw)
		}
	}

	return nil
}

func hostWithoutUserinfo(host string) string {
	if index := strings.LastIndex(host, "@"); index >= 0 {
		return host[index+1:]
	}

	return host
}

func validateRemotePart(part string, label string) (string, error) {
	if part == "" {
		return "", fmt.Errorf("%s is empty", label)
	}

	unescaped, err := url.PathUnescape(part)
	if err != nil {
		return "", fmt.Errorf("%s has invalid percent escape: %w", label, err)
	}

	if unescaped == "." || unescaped == ".." {
		return "", fmt.Errorf("%s %q is not allowed", label, unescaped)
	}

	if unescaped != part {
		return "", fmt.Errorf("%s contains percent escapes", label)
	}

	for _, char := range part {
		if !isSafeIdentifierChar(char) {
			return "", fmt.Errorf("%s %q contains unsupported characters", label, part)
		}
	}

	return part, nil
}

func isSafeIdentifierChar(char rune) bool {
	return (char >= 'A' && char <= 'Z') ||
		(char >= 'a' && char <= 'z') ||
		(char >= '0' && char <= '9') ||
		char == '.' ||
		char == '_' ||
		char == '-'
}
