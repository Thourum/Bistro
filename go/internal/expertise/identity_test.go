package expertise

import "testing"

func TestIdentityMatches(t *testing.T) {
	identity := Identity{
		Login:        "samuelasvanyi",
		AccountEmail: "samuel@example.com",
		AliasEmails:  []string{"alias@example.com", "work@example.com"},
	}

	tests := []struct {
		name        string
		authorName  string
		authorEmail string
		want        bool
	}{
		{
			name:        "numeric github noreply email matches login",
			authorEmail: "12345+samuelasvanyi@users.noreply.github.com",
			want:        true,
		},
		{
			name:        "numeric github noreply email matches login case insensitively",
			authorEmail: "12345+SamuelAsvanyi@users.noreply.github.com",
			want:        true,
		},
		{
			name:        "old style github noreply email matches login",
			authorEmail: "samuelasvanyi@users.noreply.github.com",
			want:        true,
		},
		{
			name:       "author name matches login case insensitively",
			authorName: "SamuelAsvanyi",
			want:       true,
		},
		{
			name:        "account email matches case insensitively",
			authorEmail: "SAMUEL@example.com",
			want:        true,
		},
		{
			name:        "alias email matches case insensitively",
			authorEmail: "WORK@example.com",
			want:        true,
		},
		{
			name:        "unrelated name and email do not match",
			authorName:  "other-user",
			authorEmail: "other@example.com",
			want:        false,
		},
		{
			name:        "different noreply login does not match",
			authorEmail: "12345+other-user@users.noreply.github.com",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identity.Matches(tt.authorName, tt.authorEmail)
			if got != tt.want {
				t.Errorf("Matches(%q, %q) = %v, want %v", tt.authorName, tt.authorEmail, got, tt.want)
			}
		})
	}
}

func TestIdentityMatchesWithUnavailableAccountEmail(t *testing.T) {
	identity := Identity{
		Login:        "samuelasvanyi",
		AccountEmail: "",
		AliasEmails:  []string{"alias@example.com"},
	}

	if identity.Matches("", "samuelasvanyi@users.noreply.github.com") != true {
		t.Fatal("expected noreply email to match when account email is unavailable")
	}
	if identity.Matches("", "alias@example.com") != true {
		t.Fatal("expected alias email to match when account email is unavailable")
	}
	if identity.Matches("", "other@example.com") != false {
		t.Fatal("expected unrelated email not to match empty account email")
	}
}
