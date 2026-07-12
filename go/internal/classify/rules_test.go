package classify

import "testing"

func TestParseRuleFile(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, rf *RuleFile)
	}{
		{
			name: "valid vendor file with severity_map and direct classifiers",
			yaml: `
name: CodeRabbit
vendor: coderabbit
match:
  author: ["coderabbitai[bot]"]
classifiers:
  - name: emoji_severity
    body_pattern: '^(?P<emoji>A|B)'
    severity_map: { "A": critical, "B": nit }
  - name: nitpick_section
    body_pattern: 'Nitpick'
    severity: nit
`,
			check: func(t *testing.T, rf *RuleFile) {
				if rf.Vendor != "coderabbit" || len(rf.Classifiers) != 2 {
					t.Fatalf("parse shape: %+v", rf)
				}
				if rf.Classifiers[0].SeverityMap["A"] != "critical" {
					t.Errorf("severity_map not parsed")
				}
			},
		},
		{
			name: "weighted heuristic with path glob",
			yaml: `
name: Heuristics
classifiers:
  - name: docs_style
    path: "{**/*.md,**/*.rst}"
    severity: suggestion
    category: docs
    weight: 0.3
`,
			check: func(t *testing.T, rf *RuleFile) {
				if rf.Classifiers[0].Weight != 0.3 {
					t.Errorf("weight not parsed")
				}
			},
		},
		{name: "malformed yaml", yaml: `name: [broken`, wantErr: true},
		{name: "bad regex", yaml: "name: X\nclassifiers:\n  - name: bad\n    body_pattern: '('\n    severity: nit\n", wantErr: true},
		{name: "invalid severity value", yaml: "name: X\nclassifiers:\n  - name: bad\n    body_pattern: 'x'\n    severity: catastrophic\n", wantErr: true},
		{name: "classifier with no outcome", yaml: "name: X\nclassifiers:\n  - name: bad\n    body_pattern: 'x'\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rf, err := Parse([]byte(tt.yaml), tt.name+".yml")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.check != nil && err == nil {
				tt.check(t, rf)
			}
		})
	}
}
