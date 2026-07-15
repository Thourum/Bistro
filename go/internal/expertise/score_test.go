package expertise

import (
	"math"
	"testing"
	"time"
)

func TestCommitScore(t *testing.T) {
	tests := []struct {
		name string
		ages []time.Duration
		want float64
	}{
		{
			name: "four substantial commits today saturates score",
			ages: []time.Duration{0, 0, 0, 0},
			want: 1.0,
		},
		{
			name: "one substantial commit six months ago decays then normalizes",
			ages: []time.Duration{time.Hour * 24 * 30 * 6},
			want: 0.125,
		},
		{
			name: "zero substantial commits returns zero",
			ages: nil,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CommitScore(tt.ages)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("CommitScore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScore(t *testing.T) {
	tests := []struct {
		name        string
		surviving   int
		total       int
		commitScore float64
		isOriginal  bool
		want        float64
	}{
		{
			name:        "combines surviving ratio commits and original authorship",
			surviving:   312,
			total:       480,
			commitScore: 0.4,
			isOriginal:  true,
			want:        62.75,
		},
		{
			name:        "empty file has no surviving term",
			surviving:   12,
			total:       0,
			commitScore: 0.5,
			isOriginal:  false,
			want:        15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Score(tt.surviving, tt.total, tt.commitScore, tt.isOriginal)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("Score() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsExpert(t *testing.T) {
	tests := []struct {
		name      string
		surviving int
		minLines  int
		score     float64
		want      bool
	}{
		{name: "surviving lines at threshold is expert", surviving: 50, minLines: 50, score: 0, want: true},
		{name: "score floor with few lines is expert", surviving: 3, minLines: 50, score: 40, want: true},
		{name: "below both thresholds is not expert", surviving: 49, minLines: 50, score: 39.99, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsExpert(tt.surviving, tt.minLines, tt.score)
			if got != tt.want {
				t.Errorf("IsExpert() = %v, want %v", got, tt.want)
			}
		})
	}
}
