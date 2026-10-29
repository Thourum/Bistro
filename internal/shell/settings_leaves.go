package shell

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
)

const minimumWindowSize = 400

type settingsLeaf struct {
	section string
	key     string
	apply   func(json.RawMessage, *Settings) bool
}

// normalise starts from base and applies valid leaves, ignoring unknown keys.
func normalise(raw map[string]json.RawMessage, base Settings) Settings {
	next := cloneSettings(base)
	for _, leaf := range settingsLeaves() {
		value := raw[leaf.section]
		if leaf.key != "" {
			var section map[string]json.RawMessage
			if json.Unmarshal(value, &section) != nil {
				continue
			}
			value = section[leaf.key]
		}
		leaf.apply(value, &next)
	}
	next.Schema = 1
	return next
}

func settingsLeaves() []settingsLeaf {
	return []settingsLeaf{
		{"appearance", "theme", enumLeaf(func(s *Settings) *string { return &s.Appearance.Theme }, "light", "dark")},
		{"appearance", "uiFont", stringLeaf(func(s *Settings) *string { return &s.Appearance.UIFont })},
		{"appearance", "codeFont", stringLeaf(func(s *Settings) *string { return &s.Appearance.CodeFont })},
		{"appearance", "uiFontSize", positiveLeaf(func(s *Settings) *float64 { return &s.Appearance.UIFontSize })},
		{"appearance", "codeFontSize", positiveLeaf(func(s *Settings) *float64 { return &s.Appearance.CodeFontSize })},
		{"appearance", "highDensity", boolLeaf(func(s *Settings) *bool { return &s.Appearance.HighDensity })},
		{"appearance", "onboardingComplete", boolLeaf(func(s *Settings) *bool { return &s.Appearance.OnboardingComplete })},
		{"appearance", "tourComplete", boolLeaf(func(s *Settings) *bool { return &s.Appearance.TourComplete })},
		{"sync", "autoSync", boolLeaf(func(s *Settings) *bool { return &s.Sync.AutoSync })},
		{"sync", "syncInterval", positiveLeaf(func(s *Settings) *float64 { return &s.Sync.SyncInterval })},
		{"sync", "notifications", boolLeaf(func(s *Settings) *bool { return &s.Sync.Notifications })},
		{"prList", "view", enumLeaf(func(s *Settings) *string { return &s.PRList.View }, "flat", "grouped")},
		{"diff", "style", enumLeaf(func(s *Settings) *string { return &s.Diff.Style }, "unified", "split")},
		{"privacy", "telemetryEnabled", boolLeaf(func(s *Settings) *bool { return &s.Privacy.TelemetryEnabled })},
		{"repositories", "devFolderPath", applyDevFolderPath},
		{"classification", "mode", enumLeaf(func(s *Settings) *string { return &s.Classification.Mode }, "rules", "rules+llm", "llm-all", "server")},
		{"classification", "dismissalScope", enumLeaf(func(s *Settings) *string { return &s.Classification.DismissalScope }, "local", "local+provider")},
		{"classification", "provider", enumLeaf(func(s *Settings) *string { return &s.Classification.Provider }, "anthropic", "openai", "openrouter", "ollama", "custom")},
		{"classification", "model", stringLeaf(func(s *Settings) *string { return &s.Classification.Model })},
		{"classification", "endpoint", stringLeaf(func(s *Settings) *string { return &s.Classification.Endpoint })},
		{"navigation", "style", enumLeaf(func(s *Settings) *string { return &s.Navigation.Style }, "vim", "emacs", "standard")},
		{"keybindings", "", applyKeybindings},
		{"window", "", applyWindow},
	}
}

func valueLeaf[T any](field func(*Settings) *T, valid func(T) bool) func(json.RawMessage, *Settings) bool {
	return func(raw json.RawMessage, settings *Settings) bool {
		var value T
		if isNull(raw) || json.Unmarshal(raw, &value) != nil || !valid(value) {
			return false
		}
		*field(settings) = value
		return true
	}
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

func stringLeaf(field func(*Settings) *string) func(json.RawMessage, *Settings) bool {
	return valueLeaf(field, func(string) bool { return true })
}

func boolLeaf(field func(*Settings) *bool) func(json.RawMessage, *Settings) bool {
	return valueLeaf(field, func(bool) bool { return true })
}

func enumLeaf(field func(*Settings) *string, values ...string) func(json.RawMessage, *Settings) bool {
	return valueLeaf(field, func(value string) bool { return slices.Contains(values, value) })
}

func positiveLeaf(field func(*Settings) *float64) func(json.RawMessage, *Settings) bool {
	return valueLeaf(field, func(value float64) bool { return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value) })
}

func applyDevFolderPath(raw json.RawMessage, settings *Settings) bool {
	var path *string
	if json.Unmarshal(raw, &path) != nil {
		return false
	}
	settings.Repositories.DevFolderPath = path
	return true
}

func applyKeybindings(raw json.RawMessage, settings *Settings) bool {
	var entries map[string]json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || entries == nil {
		return false
	}
	bindings := make(map[string][]string, len(entries))
	for action, encoded := range entries {
		var keys []json.RawMessage
		if json.Unmarshal(encoded, &keys) != nil || keys == nil {
			return false
		}
		bindings[action] = make([]string, len(keys))
		for i, key := range keys {
			if isNull(key) || json.Unmarshal(key, &bindings[action][i]) != nil {
				return false
			}
		}
	}
	settings.Keybindings = bindings
	return true
}

func validWindow(bounds WindowBounds) bool {
	return (bounds.Width == 0 || bounds.Width >= minimumWindowSize) && (bounds.Height == 0 || bounds.Height >= minimumWindowSize)
}

func applyWindow(raw json.RawMessage, settings *Settings) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	var bounds WindowBounds
	if json.Unmarshal(raw, &bounds) != nil || !validWindow(bounds) {
		return false
	}
	for _, key := range []string{"x", "y", "width", "height", "maximised"} {
		if isNull(fields[key]) {
			return false
		}
	}
	settings.Window = bounds
	return true
}
