package classify

import "testing"

func TestClassify(t *testing.T) {
	engine := newTestEngine(t, map[string]string{
		"coderabbit.yml": `
name: CodeRabbit
vendor: coderabbit
match:
  author: ["coderabbitai[bot]", "coderabbit-ai[bot]"]
classifiers:
  - name: emoji_severity
    body_pattern: '^(?P<emoji>🔒|⚠️|🛠️|💡)'
    severity_map: { "🔒": critical, "⚠️": warning, "🛠️": suggestion, "💡": nit }
  - name: vendor_warning
    body_pattern: 'vendor only'
    severity: warning
`,
		"a.yml": `
name: Early
classifiers:
  - name: early_direct
    body_pattern: 'direct collision'
    severity: nit
  - name: weighted_nit_first
    body_pattern: 'nit tally'
    severity: nit
    weight: 0.4
  - name: weighted_category
    body_pattern: 'category tally'
    severity: warning
    category: correctness
    weight: 0.5
  - name: direct_warning
    body_pattern: 'direct beats weighted'
    severity: warning
`,
		"m.yml": `
name: Middle
classifiers:
  - name: weighted_critical
    body_pattern: '(nit tally|direct beats weighted)'
    severity: critical
    category: security
    weight: 0.7
`,
		"p.yml": `
name: Path gated
classifiers:
  - name: all_conditions
    author: special-author
    path: '**/auth/**'
    body_pattern: 'secret'
    severity: critical
    category: security
`,
		"z.yml": `
name: Late
classifiers:
  - name: late_direct
    body_pattern: 'direct collision'
    severity: critical
  - name: weighted_nit_last
    body_pattern: 'nit tally'
    severity: nit
    category: style
    weight: 0.4
`,
		"zz-fallback.yml": `
name: Fallback
classifiers:
  - name: fallback_nit
    body_pattern: 'fallback nit'
    severity: nit
`,
	})

	tests := []struct {
		name   string
		author string
		body   string
		path   string
		want   Classification
	}{
		{
			name:   "bot id from vendor match author",
			author: "coderabbitai[bot]",
			body:   "no classifier match",
			want: Classification{
				AuthorType: "bot",
				BotVendor:  "coderabbit",
				Source:     "unclassified",
			},
		},
		{
			name:   "generic bot suffix is unknown vendor",
			author: "random-thing[bot]",
			body:   "no classifier match",
			want: Classification{
				AuthorType: "bot",
				BotVendor:  "unknown",
				Source:     "unclassified",
			},
		},
		{
			name:   "plain login is human",
			author: "octocat",
			body:   "no classifier match",
			want: Classification{
				AuthorType: "human",
				Source:     "unclassified",
			},
		},
		{
			name:   "severity map resolves named capture group",
			author: "coderabbitai[bot]",
			body:   "🔒 SQL injection",
			want: Classification{
				AuthorType: "bot",
				BotVendor:  "coderabbit",
				Severity:   "critical",
				Source:     "rule:coderabbit.yml:emoji_severity",
			},
		},
		{
			name:   "direct classifiers are last match wins by file order",
			author: "octocat",
			body:   "direct collision",
			want: Classification{
				AuthorType: "human",
				Severity:   "critical",
				Source:     "rule:z.yml:late_direct",
			},
		},
		{
			name:   "weighted tally sums severity and keeps last contributor source",
			author: "octocat",
			body:   "nit tally",
			want: Classification{
				AuthorType: "human",
				Severity:   "nit",
				Category:   "style",
				Source:     "rule:z.yml:weighted_nit_last",
			},
		},
		{
			name:   "direct match outranks weighted tally",
			author: "octocat",
			body:   "direct beats weighted",
			want: Classification{
				AuthorType: "human",
				Severity:   "warning",
				Source:     "rule:a.yml:direct_warning",
			},
		},
		{
			name:   "weighted classifier carries category",
			author: "octocat",
			body:   "category tally",
			want: Classification{
				AuthorType: "human",
				Severity:   "warning",
				Category:   "correctness",
				Source:     "rule:a.yml:weighted_category",
			},
		},
		{
			name:   "author path and body conditions must all match",
			author: "special-author",
			body:   "secret",
			want: Classification{
				AuthorType: "human",
				Source:     "unclassified",
			},
		},
		{
			name:   "author path and body conditions match together",
			author: "special-author",
			body:   "secret",
			path:   "src/auth/login.go",
			want: Classification{
				AuthorType: "human",
				Severity:   "critical",
				Category:   "security",
				Source:     "rule:p.yml:all_conditions",
			},
		},
		{
			name:   "no severity match still identifies bot",
			author: "random-thing[bot]",
			body:   "nothing to classify",
			want: Classification{
				AuthorType: "bot",
				BotVendor:  "unknown",
				Source:     "unclassified",
			},
		},
		{
			name:   "vendor file classifiers are scoped to vendor authors",
			author: "octocat",
			body:   "vendor only fallback nit",
			want: Classification{
				AuthorType: "human",
				Severity:   "nit",
				Source:     "rule:zz-fallback.yml:fallback_nit",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.Classify(tt.author, tt.body, tt.path)
			if got != tt.want {
				t.Fatalf("Classify() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestClassifyBreaksWeightTiesBySeverityRank(t *testing.T) {
	engine := newTestEngine(t, map[string]string{
		"rules.yml": `
name: Heuristics
classifiers:
  - name: warning_weight
    body_pattern: 'tie'
    severity: warning
    weight: 0.5
  - name: nit_weight
    body_pattern: 'tie'
    severity: nit
    weight: 0.5
`,
	})

	got := engine.Classify("octocat", "tie", "")
	want := Classification{AuthorType: "human", Severity: "warning", Source: "rule:rules.yml:warning_weight"}
	if got != want {
		t.Fatalf("Classify() = %+v, want %+v", got, want)
	}
}

func TestClassifyIgnoresUnmappedSeverityMapCapture(t *testing.T) {
	engine := newTestEngine(t, map[string]string{
		"rules.yml": `
name: Maps
classifiers:
  - name: mapped
    body_pattern: '^(?P<token>A|B)'
    severity_map: { "A": critical }
`,
	})

	got := engine.Classify("octocat", "B", "")
	want := Classification{AuthorType: "human", Source: "unclassified"}
	if got != want {
		t.Fatalf("Classify() = %+v, want %+v", got, want)
	}
}

func newTestEngine(t *testing.T, files map[string]string) *Engine {
	t.Helper()

	fileNames := []string{"a.yml", "coderabbit.yml", "m.yml", "p.yml", "rules.yml", "z.yml", "zz-fallback.yml"}
	loaded := &Loaded{Fingerprint: "test"}
	for _, fileName := range fileNames {
		data, ok := files[fileName]
		if !ok {
			continue
		}
		ruleFile, err := Parse([]byte(data), fileName)
		if err != nil {
			t.Fatalf("Parse(%s): %v", fileName, err)
		}
		loaded.Files = append(loaded.Files, ruleFile)
	}
	return NewEngine(loaded)
}
