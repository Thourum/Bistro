package expertise

import (
	"regexp"
	"strings"
)

var numericNoreplyEmailPattern = regexp.MustCompile(`^\d+\+(.+)@users\.noreply\.github\.com$`)

type Identity struct {
	Login string
	// AccountEmail should be the authenticated GitHub account email when the
	// provider exposes it. The current provider layer has no Email field, so
	// callers wire "" for now and rely on noreply, login, and alias matches.
	AccountEmail string
	AliasEmails  []string
}

func (identity Identity) Matches(authorName, authorEmail string) bool {
	login := strings.TrimSpace(identity.Login)
	name := strings.TrimSpace(authorName)
	email := strings.TrimSpace(authorEmail)

	if login != "" && strings.EqualFold(name, login) {
		return true
	}
	if login != "" && matchesNoreplyEmail(login, email) {
		return true
	}
	if identity.AccountEmail != "" && strings.EqualFold(email, strings.TrimSpace(identity.AccountEmail)) {
		return true
	}
	for _, aliasEmail := range identity.AliasEmails {
		if aliasEmail != "" && strings.EqualFold(email, strings.TrimSpace(aliasEmail)) {
			return true
		}
	}

	return false
}

func matchesNoreplyEmail(login, email string) bool {
	matches := numericNoreplyEmailPattern.FindStringSubmatch(email)
	if len(matches) == 2 {
		return strings.EqualFold(matches[1], login)
	}

	oldStyleEmail := login + "@users.noreply.github.com"
	return strings.EqualFold(email, oldStyleEmail)
}
