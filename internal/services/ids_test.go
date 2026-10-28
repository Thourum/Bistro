package services

import (
	"slices"
	"testing"
)

func TestNormalizeID(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{id: "github:Thourum/Testing", want: "github:thourum/testing"},
		{id: "github:Acme/API:1", want: "github:acme/api:1"},
		{id: "github:Acme/API:3", want: "github:acme/api:3"},
		{id: "GitHub:Owner/Repo#1", want: "github:owner/repo#1"},
		{id: "gitea:Org/Repo:2", want: "gitea:Org/Repo:2"},
		{id: "Keep Me", want: "Keep Me"},
		{id: "githubish:Owner/Repo", want: "githubish:Owner/Repo"},
		{id: ""},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := NormalizeID(tt.id); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeIDs(t *testing.T) {
	if got := NormalizeIDs(nil); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
	if got := NormalizeIDs([]string{}); got == nil || len(got) != 0 {
		t.Fatalf("got %v, want non-nil empty slice", got)
	}
	ids := []string{"GitHub:Owner/Repo#1", "gitea:Org/Repo:2"}
	original := slices.Clone(ids)
	want := []string{"github:owner/repo#1", "gitea:Org/Repo:2"}
	got := NormalizeIDs(ids)
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if !slices.Equal(ids, original) {
		t.Fatalf("input changed to %v", ids)
	}
	got[0] = "changed"
	if !slices.Equal(ids, original) {
		t.Fatal("returned slice aliases input")
	}
}
