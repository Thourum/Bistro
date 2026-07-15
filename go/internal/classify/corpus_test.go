package classify

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

type corpusEntry struct {
	Author       string `yaml:"author"`
	Body         string `yaml:"body"`
	Path         string `yaml:"path"`
	WantSeverity string `yaml:"want_severity"`
}

func loadCorpus(t testing.TB) []corpusEntry {
	t.Helper()
	data, err := os.ReadFile("testdata/corpus.yml")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var corpus []corpusEntry
	if err := yaml.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("parse corpus: %v", err)
	}
	if len(corpus) < 45 {
		t.Fatalf("corpus too small: %d entries, want ~50", len(corpus))
	}
	return corpus
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// TestCorpusHeuristicAccuracy is the regression gate from the spec: the
// shipped built-ins (vendor rules + zz-fallback heuristics) must label at
// least 70% of the corpus with its human-judged ideal severity.
func TestCorpusHeuristicAccuracy(t *testing.T) {
	files, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	eng := NewEngine(&Loaded{Files: files})

	corpus := loadCorpus(t)
	correct := 0
	for _, c := range corpus {
		got := eng.Classify(c.Author, c.Body, c.Path)
		if got.Severity == c.WantSeverity {
			correct++
		} else {
			t.Logf("MISS %q: got %q want %q (src %s)", truncate(c.Body, 60), got.Severity, c.WantSeverity, got.Source)
		}
	}
	acc := float64(correct) / float64(len(corpus))
	t.Logf("accuracy: %.0f%% (%d/%d)", acc*100, correct, len(corpus))
	if acc < 0.70 {
		t.Errorf("heuristic accuracy %.0f%% < 70%% (%d/%d)", acc*100, correct, len(corpus))
	}
}

// BenchmarkClassify50 sanity-checks the spec's <50ms-per-PR bar: one
// iteration classifies 50 corpus comments through the shipped built-ins.
func BenchmarkClassify50(b *testing.B) {
	files, err := Builtins()
	if err != nil {
		b.Fatal(err)
	}
	eng := NewEngine(&Loaded{Files: files})
	corpus := loadCorpus(b)
	if len(corpus) > 50 {
		corpus = corpus[:50]
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, c := range corpus {
			_ = eng.Classify(c.Author, c.Body, c.Path)
		}
	}
}
