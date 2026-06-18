package provider

import "sort"

// DeriveReviewStatus reduces a PR's review history to a single status plus
// the deciding reviewers. Each reviewer's latest *vote* counts: "approved"
// and "changes_requested" set the vote, "dismissed" clears it, and
// "commented"/"pending" leave it untouched. Any outstanding
// changes_requested vote outranks approvals.
func DeriveReviewStatus(reviews []*Review) (status, approvedBy, changesRequestedBy string) {
	sorted := make([]*Review, len(reviews))
	copy(sorted, reviews)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SubmittedAt.Before(sorted[j].SubmittedAt)
	})

	votes := make(map[string]string)
	var voteOrder []string
	for _, r := range sorted {
		if r == nil || r.Author == "" {
			continue
		}
		switch r.State {
		case "approved", "changes_requested":
			if _, seen := votes[r.Author]; !seen {
				voteOrder = append(voteOrder, r.Author)
			}
			votes[r.Author] = r.State
		case "dismissed":
			delete(votes, r.Author)
		}
	}

	for _, author := range voteOrder {
		if votes[author] == "changes_requested" && changesRequestedBy == "" {
			changesRequestedBy = author
		}
		if votes[author] == "approved" && approvedBy == "" {
			approvedBy = author
		}
	}

	switch {
	case changesRequestedBy != "":
		return "changes_requested", approvedBy, changesRequestedBy
	case approvedBy != "":
		return "approved", approvedBy, ""
	default:
		return "pending", "", ""
	}
}
