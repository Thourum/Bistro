package diff

import (
	"container/heap"
	"errors"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/logging"
)

const (
	flagHead uint8 = 1 << iota
	flagBase

	// Match git revision.c's SLOP: tolerate limited out-of-order timestamps
	// without reading all shared repository history on each request.
	commitWalkSlop = 5
)

type commitQueue []*object.Commit

func (q commitQueue) Len() int { return len(q) }
func (q commitQueue) Less(i, j int) bool {
	ti, tj := q[i].Committer.When, q[j].Committer.When
	if !ti.Equal(tj) {
		return ti.After(tj)
	}
	return q[i].Hash.String() < q[j].Hash.String()
}
func (q commitQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *commitQueue) Push(x any)   { *q = append(*q, x.(*object.Commit)) }
func (q *commitQueue) Pop() any {
	old := *q
	c := old[len(old)-1]
	*q = old[:len(old)-1]
	return c
}

func loadRangeCommit(repo *gitlib.Repository, sha string) (*object.Commit, error) {
	c, err := repo.CommitObject(plumbing.NewHash(sha))
	if errors.Is(err, plumbing.ErrObjectNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrHeadUnavailable, logging.ShortSHA(sha))
	}
	if err != nil {
		return nil, fmt.Errorf("load commit %s: %w", logging.ShortSHA(sha), err)
	}
	return c, nil
}

// rangeCommits returns head's reachable commits excluding base's ancestors,
// with parents before children.
func rangeCommits(repo *gitlib.Repository, headSHA, mergeBaseSHA string) ([]*object.Commit, error) {
	head, err := loadRangeCommit(repo, headSHA)
	if err != nil {
		return nil, err
	}
	if headSHA == mergeBaseSHA {
		return []*object.Commit{}, nil
	}
	base, err := loadRangeCommit(repo, mergeBaseSHA)
	if err != nil {
		return nil, err
	}
	flags := map[plumbing.Hash]uint8{head.Hash: flagHead}
	flags[base.Hash] |= flagBase
	seen := map[plumbing.Hash]*object.Commit{head.Hash: head, base.Hash: base}
	paint := rangePaint{flags: flags, seen: seen, queue: &commitQueue{head}}
	heap.Push(paint.queue, base)
	if err := paintRange(repo, paint); err != nil {
		return nil, err
	}
	var candidates []*object.Commit
	for hash, c := range seen {
		if flags[hash] == flagHead {
			candidates = append(candidates, c)
		}
	}
	return topoOldestFirst(candidates), nil
}

type rangePaint struct {
	flags map[plumbing.Hash]uint8
	seen  map[plumbing.Hash]*object.Commit
	queue *commitQueue
}

func paintRange(repo *gitlib.Repository, paint rangePaint) error {
	slop := commitWalkSlop
	// Like git, this walk does not support skew beyond five all-base pops;
	// an exact skew-independent walk would need a reachability index.
	for paint.queue.Len() > 0 {
		if anyHeadOnly(*paint.queue, paint.flags) {
			slop = commitWalkSlop
		} else {
			if slop == 0 {
				break
			}
			slop--
		}
		commit := heap.Pop(paint.queue).(*object.Commit)
		if err := paintRangeParents(repo, commit, paint); err != nil {
			return err
		}
	}
	return nil
}

func anyHeadOnly(queue commitQueue, flags map[plumbing.Hash]uint8) bool {
	for _, commit := range queue {
		if flags[commit.Hash]&flagBase == 0 {
			return true
		}
	}
	return false
}

func paintRangeParents(repo *gitlib.Repository, commit *object.Commit, paint rangePaint) error {
	for _, parentHash := range commit.ParentHashes {
		before := paint.flags[parentHash]
		after := before | paint.flags[commit.Hash]
		if after == before {
			continue
		}
		paint.flags[parentHash] = after
		parent, ok := paint.seen[parentHash]
		if !ok {
			var err error
			parent, err = loadRangeCommit(repo, parentHash.String())
			if err != nil {
				return err
			}
			paint.seen[parentHash] = parent
		}
		heap.Push(paint.queue, parent)
	}
	return nil
}

// oldestQueue pops the oldest ready commit first, then the smallest SHA.
type oldestQueue struct{ commitQueue }

func (q oldestQueue) Less(i, j int) bool {
	ti, tj := q.commitQueue[i].Committer.When, q.commitQueue[j].Committer.When
	if !ti.Equal(tj) {
		return ti.Before(tj)
	}
	return q.commitQueue[i].Hash.String() < q.commitQueue[j].Hash.String()
}

func topoOldestFirst(commits []*object.Commit) []*object.Commit {
	inSet := make(map[plumbing.Hash]bool, len(commits))
	for _, c := range commits {
		inSet[c.Hash] = true
	}
	pending := make(map[plumbing.Hash]int, len(commits))
	children := make(map[plumbing.Hash][]*object.Commit)
	ready := &oldestQueue{}
	for _, c := range commits {
		for _, p := range c.ParentHashes {
			if inSet[p] {
				pending[c.Hash]++
				children[p] = append(children[p], c)
			}
		}
		if pending[c.Hash] == 0 {
			heap.Push(ready, c)
		}
	}
	ordered := make([]*object.Commit, 0, len(commits))
	for ready.Len() > 0 {
		c := heap.Pop(ready).(*object.Commit)
		ordered = append(ordered, c)
		for _, child := range children[c.Hash] {
			pending[child.Hash]--
			if pending[child.Hash] == 0 {
				heap.Push(ready, child)
			}
		}
	}
	return ordered
}
