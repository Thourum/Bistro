package classify

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"unicode"
)

const dedupSimilarityThreshold = 0.7

// DedupInput is what the writer hands the post-pass per comment.
type DedupInput struct {
	ID           string
	AuthorType   string
	FilePath     *string
	AnchoredLine *int
	LineNumber   *int
	Body         string
}

// Dedup returns commentID → groupID for every comment in a group of size ≥ 2.
func Dedup(comments []DedupInput) map[string]string {
	participants := dedupParticipants(comments)
	groups := make(map[string]string)
	if len(participants) < 2 {
		return groups
	}

	finder := newUnionFind(len(participants))

	// ponytail: O(n²) pair scan per PR — fine at PR comment counts; bucket by file+line window if a 1000-comment PR ever shows up.
	for i := 0; i < len(participants); i++ {
		for j := i + 1; j < len(participants); j++ {
			if !isDedupCandidatePair(participants[i], participants[j]) {
				continue
			}
			if trigramJaccard(participants[i].trigrams, participants[j].trigrams) > dedupSimilarityThreshold {
				finder.union(i, j)
			}
		}
	}

	components := make(map[int][]string)
	for index, participant := range participants {
		root := finder.find(index)
		components[root] = append(components[root], participant.id)
	}

	for _, ids := range components {
		if len(ids) < 2 {
			continue
		}
		groupID := dedupGroupID(ids)
		for _, id := range ids {
			groups[id] = groupID
		}
	}

	return groups
}

type dedupParticipant struct {
	id       string
	filePath string
	line     int
	trigrams map[string]struct{}
}

func dedupParticipants(comments []DedupInput) []dedupParticipant {
	participants := make([]dedupParticipant, 0, len(comments))
	for _, comment := range comments {
		line, hasLine := displayLine(comment)
		if comment.AuthorType != "bot" || comment.FilePath == nil || !hasLine {
			continue
		}
		participants = append(participants, dedupParticipant{
			id:       comment.ID,
			filePath: *comment.FilePath,
			line:     line,
			trigrams: trigrams(normalizeDedupBody(comment.Body)),
		})
	}
	return participants
}

func displayLine(comment DedupInput) (int, bool) {
	if comment.AnchoredLine != nil {
		return *comment.AnchoredLine, true
	}
	if comment.LineNumber != nil {
		return *comment.LineNumber, true
	}
	return 0, false
}

func isDedupCandidatePair(left dedupParticipant, right dedupParticipant) bool {
	return left.filePath == right.filePath && abs(left.line-right.line) <= 3
}

func normalizeDedupBody(body string) string {
	withoutFences := stripFencedCodeBlocks(body)
	lower := strings.ToLower(withoutFences)
	withoutMarkdown := strings.Map(func(r rune) rune {
		switch r {
		case '*', '_', '#', '>':
			return -1
		default:
			return r
		}
	}, lower)
	return strings.Join(strings.Fields(withoutMarkdown), " ")
}

func stripFencedCodeBlocks(body string) string {
	var builder strings.Builder
	remaining := body
	for {
		start := strings.Index(remaining, "```")
		if start == -1 {
			builder.WriteString(remaining)
			return builder.String()
		}
		builder.WriteString(remaining[:start])
		afterStart := remaining[start+3:]
		end := strings.Index(afterStart, "```")
		if end == -1 {
			return builder.String()
		}
		remaining = afterStart[end+3:]
	}
}

func trigrams(value string) map[string]struct{} {
	runes := []rune(value)
	sets := make(map[string]struct{})
	for i := 0; i+3 <= len(runes); i++ {
		gram := string(runes[i : i+3])
		if strings.TrimFunc(gram, unicode.IsSpace) == "" {
			continue
		}
		sets[gram] = struct{}{}
	}
	return sets
}

func trigramJaccard(left map[string]struct{}, right map[string]struct{}) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}

	intersection := 0
	for gram := range left {
		if _, ok := right[gram]; ok {
			intersection++
		}
	}
	union := len(left) + len(right) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func dedupGroupID(ids []string) string {
	sortedIDs := append([]string(nil), ids...)
	sort.Strings(sortedIDs)
	sum := sha256.Sum256([]byte(strings.Join(sortedIDs, "\n")))
	return hex.EncodeToString(sum[:])
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

type unionFind struct {
	parents []int
	ranks   []int
}

func newUnionFind(size int) *unionFind {
	parents := make([]int, size)
	ranks := make([]int, size)
	for i := range parents {
		parents[i] = i
	}
	return &unionFind{parents: parents, ranks: ranks}
}

func (uf *unionFind) find(value int) int {
	if uf.parents[value] != value {
		uf.parents[value] = uf.find(uf.parents[value])
	}
	return uf.parents[value]
}

func (uf *unionFind) union(left int, right int) {
	leftRoot := uf.find(left)
	rightRoot := uf.find(right)
	if leftRoot == rightRoot {
		return
	}
	if uf.ranks[leftRoot] < uf.ranks[rightRoot] {
		uf.parents[leftRoot] = rightRoot
		return
	}
	if uf.ranks[leftRoot] > uf.ranks[rightRoot] {
		uf.parents[rightRoot] = leftRoot
		return
	}
	uf.parents[rightRoot] = leftRoot
	uf.ranks[leftRoot]++
}
