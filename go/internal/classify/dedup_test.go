package classify

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestDedup(t *testing.T) {
	path := "src/app.go"
	otherPath := "src/other.go"
	line10 := 10
	line11 := 11
	line12 := 12
	line13 := 13
	line14 := 14
	line20 := 20
	line30 := 30
	anchored50 := 50
	line200 := 200

	tests := []struct {
		name     string
		comments []DedupInput
		want     map[string]string
	}{
		{
			name: "groups near-identical bot comments on same file within three display lines",
			comments: []DedupInput{
				botComment("a", &path, &line10, nil, "Potential nil pointer dereference in this branch."),
				botComment("b", &path, &line12, nil, "Potential nil pointer dereference in this branch!"),
			},
			want: groupMap([]string{"a", "b"}),
		},
		{
			name: "excludes candidate pairs more than three display lines apart",
			comments: []DedupInput{
				botComment("a", &path, &line10, nil, "Potential nil pointer dereference in this branch."),
				botComment("b", &path, &line14, nil, "Potential nil pointer dereference in this branch!"),
			},
			want: map[string]string{},
		},
		{
			name: "excludes candidate pairs at or below similarity threshold",
			comments: []DedupInput{
				botComment("a", &path, &line10, nil, "Potential nil pointer dereference in this branch."),
				botComment("b", &path, &line12, nil, "Rename the local variable for clarity."),
			},
			want: map[string]string{},
		},
		{
			name: "excludes human authors",
			comments: []DedupInput{
				botComment("a", &path, &line10, nil, "Potential nil pointer dereference in this branch."),
				{
					ID:         "b",
					AuthorType: "human",
					FilePath:   &path,
					LineNumber: &line12,
					Body:       "Potential nil pointer dereference in this branch!",
				},
			},
			want: map[string]string{},
		},
		{
			name: "excludes comments without file path",
			comments: []DedupInput{
				botComment("a", nil, &line10, nil, "Potential nil pointer dereference in this branch."),
				botComment("b", nil, &line12, nil, "Potential nil pointer dereference in this branch!"),
			},
			want: map[string]string{},
		},
		{
			name: "excludes comments without display line",
			comments: []DedupInput{
				botComment("a", &path, nil, nil, "Potential nil pointer dereference in this branch."),
				botComment("b", &path, nil, nil, "Potential nil pointer dereference in this branch!"),
			},
			want: map[string]string{},
		},
		{
			name: "display line prefers anchored line over line number",
			comments: []DedupInput{
				botComment("a", &path, &line10, nil, "Potential nil pointer dereference in this branch."),
				botComment("b", &path, &line200, &anchored50, "Potential nil pointer dereference in this branch!"),
				botComment("c", &path, &line200, &line11, "Potential nil pointer dereference in this branch?"),
			},
			want: groupMap([]string{"a", "c"}),
		},
		{
			name: "keeps components transitive when endpoints are dissimilar",
			comments: []DedupInput{
				botComment("a", &path, &line10, nil, "Potential null pointer dereference in response body."),
				botComment("b", &path, &line11, nil, "Potential null pointer dereference in response body without guard."),
				botComment("c", &path, &line13, nil, "Null pointer dereference in response body without guard."),
			},
			want: groupMap([]string{"a", "b", "c"}),
		},
		{
			name: "uses file path as candidate boundary",
			comments: []DedupInput{
				botComment("a", &path, &line10, nil, "Potential nil pointer dereference in this branch."),
				botComment("b", &otherPath, &line12, nil, "Potential nil pointer dereference in this branch!"),
			},
			want: map[string]string{},
		},
		{
			name: "markdown and fenced code stripping affect similarity",
			comments: []DedupInput{
				botComment("a", &path, &line20, nil, "### Warning\n**Potential nil pointer dereference** in this branch.\n\n```go\nfunc noisyA() { panic(\"very different code A\") }\n```"),
				botComment("b", &path, &line20, nil, "> Warning\nPotential nil pointer dereference in this branch!\n\n```go\nfunc noisyB() { return }\n```"),
			},
			want: groupMap([]string{"a", "b"}),
		},
		{
			name: "returns only components with at least two members",
			comments: []DedupInput{
				botComment("a", &path, &line10, nil, "Potential nil pointer dereference in this branch."),
				botComment("b", &path, &line12, nil, "Potential nil pointer dereference in this branch!"),
				botComment("c", &path, &line30, nil, "This singleton stays ungrouped."),
			},
			want: groupMap([]string{"a", "b"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Dedup(tt.comments)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Dedup() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDedupIsIdempotent(t *testing.T) {
	path := "src/app.go"
	line10 := 10
	line12 := 12
	comments := []DedupInput{
		botComment("b", &path, &line12, nil, "Potential nil pointer dereference in this branch!"),
		botComment("a", &path, &line10, nil, "Potential nil pointer dereference in this branch."),
	}

	first := Dedup(comments)
	second := Dedup(comments)

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Dedup() was not idempotent: first=%#v second=%#v", first, second)
	}
	if !reflect.DeepEqual(first, groupMap([]string{"a", "b"})) {
		t.Fatalf("Dedup() = %#v, want stable sha256(sorted ids) group", first)
	}
}

func botComment(id string, filePath *string, lineNumber *int, anchoredLine *int, body string) DedupInput {
	return DedupInput{
		ID:           id,
		AuthorType:   "bot",
		FilePath:     filePath,
		AnchoredLine: anchoredLine,
		LineNumber:   lineNumber,
		Body:         body,
	}
}

func groupMap(ids []string) map[string]string {
	groupID := expectedGroupID(ids)
	groups := make(map[string]string, len(ids))
	for _, id := range ids {
		groups[id] = groupID
	}
	return groups
}

func expectedGroupID(ids []string) string {
	sortedIDs := append([]string(nil), ids...)
	sort.Strings(sortedIDs)
	sum := sha256.Sum256([]byte(strings.Join(sortedIDs, "\n")))
	return hex.EncodeToString(sum[:])
}
