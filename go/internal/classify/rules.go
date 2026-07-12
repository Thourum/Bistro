// Package classify implements Bistro's local comment-classification engine:
// bot identification, YAML rule files, and weighted heuristics. Deterministic,
// offline, inspectable — classification_source always names what fired.
package classify

import (
	"fmt"
	"regexp"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// Severity values, ranked. ValidSeverities is the write contract for the
// comments.severity column (migration 026).
var ValidSeverities = map[string]int{
	"critical": 5, "warning": 4, "suggestion": 3, "nit": 2, "info": 1,
}

var ValidCategories = map[string]bool{
	"security": true, "correctness": true, "performance": true,
	"style": true, "readability": true, "docs": true,
}

// Classifier is one rule inside a RuleFile. Conditions (Author, Path,
// BodyPattern) are AND-combined; the outcome is either a direct Severity
// (+ optional Category) or a SeverityMap keyed on a named capture group.
// A non-zero Weight marks it as a heuristic (tallied, not last-match-wins).
type Classifier struct {
	Name        string            `yaml:"name"`
	Author      string            `yaml:"author"`
	Path        string            `yaml:"path"`
	BodyPattern string            `yaml:"body_pattern"`
	Severity    string            `yaml:"severity"`
	Category    string            `yaml:"category"`
	SeverityMap map[string]string `yaml:"severity_map"`
	Weight      float64           `yaml:"weight"`

	bodyRe *regexp.Regexp // compiled at parse; nil when BodyPattern empty
}

type Match struct {
	Author []string `yaml:"author"`
}

// RuleFile is one parsed YAML rule file. FileName is the base name it was
// loaded from — recorded in classification_source as rule:<file>:<classifier>.
type RuleFile struct {
	Name        string       `yaml:"name"`
	Vendor      string       `yaml:"vendor"`
	Match       Match        `yaml:"match"`
	Classifiers []Classifier `yaml:"classifiers"`
	FileName    string       `yaml:"-"`
}

// Parse decodes and validates one rule file. Regexes are compiled here so a
// bad pattern fails the whole file at load time, never during sync.
func Parse(data []byte, fileName string) (*RuleFile, error) {
	var rf RuleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("%s: %w", fileName, err)
	}
	rf.FileName = fileName
	for i := range rf.Classifiers {
		c := &rf.Classifiers[i]
		if c.Name == "" {
			return nil, fmt.Errorf("%s: classifier %d has no name", fileName, i)
		}
		if c.BodyPattern != "" {
			re, err := regexp.Compile(c.BodyPattern)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: bad body_pattern: %w", fileName, c.Name, err)
			}
			c.bodyRe = re
		}
		if c.Path != "" {
			if !doublestar.ValidatePattern(c.Path) {
				return nil, fmt.Errorf("%s/%s: bad path glob %q", fileName, c.Name, c.Path)
			}
		}
		if c.Severity == "" && len(c.SeverityMap) == 0 {
			return nil, fmt.Errorf("%s/%s: no severity or severity_map", fileName, c.Name)
		}
		if c.Severity != "" {
			if _, ok := ValidSeverities[c.Severity]; !ok {
				return nil, fmt.Errorf("%s/%s: invalid severity %q", fileName, c.Name, c.Severity)
			}
		}
		for k, v := range c.SeverityMap {
			if _, ok := ValidSeverities[v]; !ok {
				return nil, fmt.Errorf("%s/%s: severity_map[%s]: invalid severity %q", fileName, c.Name, k, v)
			}
		}
		if c.Category != "" && !ValidCategories[c.Category] {
			return nil, fmt.Errorf("%s/%s: invalid category %q", fileName, c.Name, c.Category)
		}
	}
	return &rf, nil
}
