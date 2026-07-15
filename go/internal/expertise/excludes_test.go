package expertise

import "testing"

func TestExcluderBuiltInDefaults(t *testing.T) {
	excluder := New(nil)

	excludedPaths := []string{
		"vendor/x.go",
		"dist/app.js",
		"node_modules/a/b.js",
		"a/b.pb.go",
		"pnpm-lock.yaml",
		"go.sum",
		"internal/testdata/f.json",
	}
	for _, path := range excludedPaths {
		t.Run(path, func(t *testing.T) {
			if !excluder.Excluded(path) {
				t.Fatalf("Excluded(%q) = false, want true", path)
			}
		})
	}
}

func TestExcluderUserGlobsAndNonMatches(t *testing.T) {
	excluder := New([]string{"website/public/**", "generated/*.go"})

	tests := []struct {
		path string
		want bool
	}{
		{path: "website/public/assets/app.js", want: true},
		{path: "generated/client.go", want: true},
		{path: "website/src/index.tsx", want: false},
		{path: "internal/app/main.go", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := excluder.Excluded(tt.path)
			if got != tt.want {
				t.Fatalf("Excluded(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
