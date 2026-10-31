package services

import "strings"

const githubIdentityPrefix = "github:"

// NormalizeID canonicalizes case-insensitive GitHub identities; other providers
// retain their original casing.
func NormalizeID(id string) string {
	if !strings.HasPrefix(strings.ToLower(id), githubIdentityPrefix) {
		return id
	}
	return strings.ToLower(id)
}

// NormalizeIDs preserves nil input and leaves the caller's identities unchanged.
func NormalizeIDs(ids []string) []string {
	if ids == nil {
		return nil
	}
	normalized := make([]string, len(ids))
	for i, id := range ids {
		normalized[i] = NormalizeID(id)
	}
	return normalized
}
