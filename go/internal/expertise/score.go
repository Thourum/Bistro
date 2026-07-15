package expertise

import (
	"math"
	"time"
)

const (
	WeightSurviving          = 55.0
	WeightCommits            = 30.0
	WeightOriginal           = 15.0
	CommitsSaturation        = 4.0
	HalfLifeMonths           = 6.0
	SubstantialLineThreshold = 5
	DormantAfterDays         = 90
	DefaultMinSurvivingLines = 50
	ExpertScoreFloor         = 40.0
	CommitWindowMonths       = 12
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

func IsExpert(surviving, minLines int, score float64) bool {
	return surviving >= minLines || score >= ExpertScoreFloor
}

func clamp(value, minValue, maxValue float64) float64 {
	return math.Max(minValue, math.Min(maxValue, value))
}
