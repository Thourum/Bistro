package classify

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Classification is the normalized result stored with a synced comment.
type Classification struct {
	AuthorType string // "human" | "bot" | "unknown"
	BotVendor  string // "" for humans; "unknown" for unmatched bots
	Severity   string // "" = unclassified
	Category   string
	Source     string // rule:<file>:<classifier> | unclassified
}

// Loaded is an immutable snapshot of a rule directory.
type Loaded struct {
	Files       []*RuleFile // alphabetical by file name — later overrides earlier
	Fingerprint string      // sha256 over file names+contents+mode; see spec "Data flow — PR open"
}

// Engine classifies one comment against a Loaded rule set. Immutable after
// construction; safe for concurrent use (RE2 regexps are goroutine-safe).
type Engine struct {
	loaded *Loaded
}

func NewEngine(loaded *Loaded) *Engine { return &Engine{loaded: loaded} }

func (e *Engine) Fingerprint() string { return e.loaded.Fingerprint }

// Classify implements the spec's resolution order:
// files alphabetical → run all matching classifiers → direct last-match-wins,
// weighted tally per severity (max wins) → direct outranks weighted.
// path may be empty (issue comments). Never returns an error: no match is
// Source="unclassified", per "classification is enrichment, never a gate".
func (e *Engine) Classify(author, body, path string) Classification {
	classification := Classification{AuthorType: "human", Source: "unclassified"}
	if e == nil || e.loaded == nil {
		return classification
	}

	for _, ruleFile := range e.loaded.Files {
		if contains(ruleFile.Match.Author, author) {
			classification.AuthorType = "bot"
			classification.BotVendor = ruleFile.Vendor
		}
	}
	if classification.AuthorType == "human" && strings.HasSuffix(author, "[bot]") {
		classification.AuthorType = "bot"
		classification.BotVendor = "unknown"
	}

	var direct *Classification
	weights := map[string]float64{}
	weightMeta := map[string]Classification{}

	for _, ruleFile := range e.loaded.Files {
		if len(ruleFile.Match.Author) > 0 && !contains(ruleFile.Match.Author, author) {
			continue
		}
		for index := range ruleFile.Classifiers {
			classifier := &ruleFile.Classifiers[index]
			severity, category, ok := evalClassifier(classifier, author, body, path)
			if !ok {
				continue
			}

			result := Classification{
				Severity: severity,
				Category: category,
				Source:   "rule:" + ruleFile.FileName + ":" + classifier.Name,
			}
			if classifier.Weight > 0 {
				weights[severity] += classifier.Weight
				weightMeta[severity] = result
				continue
			}
			direct = &result
		}
	}

	if direct != nil {
		classification.Severity = direct.Severity
		classification.Category = direct.Category
		classification.Source = direct.Source
		return classification
	}
	if len(weights) == 0 {
		return classification
	}

	bestSeverity := maxWeightSeverity(weights)
	meta := weightMeta[bestSeverity]
	classification.Severity = bestSeverity
	classification.Category = meta.Category
	classification.Source = meta.Source
	return classification
}

func evalClassifier(classifier *Classifier, author, body, path string) (string, string, bool) {
	if classifier.Author != "" && classifier.Author != author {
		return "", "", false
	}
	if classifier.Path != "" {
		if path == "" {
			return "", "", false
		}
		matched, err := doublestar.Match(classifier.Path, path)
		if err != nil || !matched {
			return "", "", false
		}
	}
	if classifier.bodyRe != nil && !classifier.bodyRe.MatchString(body) {
		return "", "", false
	}
	if len(classifier.SeverityMap) > 0 {
		severity, ok := severityFromMap(classifier, body)
		if !ok {
			return "", "", false
		}
		return severity, classifier.Category, true
	}
	return classifier.Severity, classifier.Category, true
}

func severityFromMap(classifier *Classifier, body string) (string, bool) {
	if classifier.bodyRe == nil {
		return "", false
	}
	matches := classifier.bodyRe.FindStringSubmatch(body)
	if matches == nil {
		return "", false
	}
	names := classifier.bodyRe.SubexpNames()
	for index, name := range names {
		if index == 0 || name == "" || matches[index] == "" {
			continue
		}
		severity, ok := classifier.SeverityMap[matches[index]]
		if ok {
			return severity, true
		}
		return "", false
	}
	return "", false
}

func maxWeightSeverity(weights map[string]float64) string {
	bestSeverity := ""
	bestWeight := 0.0
	for severity, weight := range weights {
		if bestSeverity == "" || weight > bestWeight || weight == bestWeight && ValidSeverities[severity] > ValidSeverities[bestSeverity] {
			bestSeverity = severity
			bestWeight = weight
		}
	}
	return bestSeverity
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
