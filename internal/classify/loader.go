package classify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadDir reads every *.yml / *.yaml in dir alphabetically. A file that fails
// to parse is skipped and reported in errs; the rest still load — one bad rule
// never kills classification. mode participates in the fingerprint so a
// classification-mode change invalidates lazily like a rule edit.
func LoadDir(dir, mode string) (*Loaded, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return &Loaded{Fingerprint: fingerprint(nil, nil, mode)}, []error{fmt.Errorf("read rules dir: %w", err)}
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".yml" || ext == ".yaml" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	loaded := &Loaded{}
	var errs []error
	var contents [][]byte
	var loadedNames []string
	for _, name := range names {
		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			errs = append(errs, readErr)
			continue
		}
		ruleFile, parseErr := Parse(data, name)
		if parseErr != nil {
			errs = append(errs, parseErr)
			continue
		}
		loaded.Files = append(loaded.Files, ruleFile)
		loadedNames = append(loadedNames, name)
		contents = append(contents, data)
	}

	loaded.Fingerprint = fingerprint(loadedNames, contents, mode)
	return loaded, errs
}

func fingerprint(names []string, contents [][]byte, mode string) string {
	hash := sha256.New()
	for index, name := range names {
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(contents[index])
		hash.Write([]byte{0})
	}
	hash.Write([]byte(mode))
	return hex.EncodeToString(hash.Sum(nil))
}
