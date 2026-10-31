package expertise

import (
	"math"
	"time"
)

const (
	// WeightSurviving is the maximum surviving-lines contribution to the score.
	WeightSurviving = 55.0
	// WeightCommits is the maximum commit-frequency contribution to the score.
	WeightCommits = 30.0
	// WeightOriginal is the original-author contribution to the score.
	WeightOriginal = 15.0
	// CommitsSaturation is the weighted-commit sum that fills the commit score.
	CommitsSaturation = 4.0
	// HalfLifeMonths is the decay period for the commit signal.
	HalfLifeMonths = 6.0
	// SubstantialLineThreshold excludes smaller commits from scoring.
	SubstantialLineThreshold = 5
	// DormantAfterDays marks authors inactive after this many days without a commit.
	DormantAfterDays = 90
	// DefaultMinSurvivingLines is the minimum surviving-line count for expert status.
	DefaultMinSurvivingLines = 50
	// ExpertScoreFloor is the minimum score for expert status regardless of lines.
	ExpertScoreFloor = 40.0
	// CommitWindowMonths is the history window considered for commit scoring.
	CommitWindowMonths = 12
)

const daysPerMonth = 30.0

// CommitScore returns the normalized, half-life-decayed commit signal for
// substantial commits. The caller is responsible for filtering to the
// 12-month commit window and to commits above SubstantialLineThreshold.
func CommitScore(commitAges []time.Duration) float64 {
	weightedSum := 0.0
	for _, age := range commitAges {
		ageMonths := age.Hours() / (24.0 * daysPerMonth)
		if ageMonths < 0 {
			ageMonths = 0
		}
		weightedSum += math.Pow(0.5, ageMonths/HalfLifeMonths)
	}

	return clamp(weightedSum/CommitsSaturation, 0, 1)
}

// Score combines surviving lines, recent commits, and original authorship.
func Score(surviving, total int, commitScore float64, isOriginal bool) float64 {
	survivingRatio := 0.0
	if total > 0 {
		survivingRatio = float64(surviving) / float64(total)
	}

	originalScore := 0.0
	if isOriginal {
		originalScore = 1.0
	}

	score := WeightSurviving*survivingRatio + WeightCommits*commitScore + WeightOriginal*originalScore
	return clamp(score, 0, 100)
}

// IsExpert reports whether surviving lines or score qualify an author as an expert.
func IsExpert(surviving, minLines int, score float64) bool {
	return surviving >= minLines || score >= ExpertScoreFloor
}

func clamp(value, minValue, maxValue float64) float64 {
	return math.Max(minValue, math.Min(maxValue, value))
}
