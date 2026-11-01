package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/prsync/writer"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/prdiff"
)

const (
	defaultDBPath       = "./data/sync.db"
	databaseDirMode     = 0o755
	benchmarkProvider   = "github"
	benchmarkOwner      = "bench"
	benchmarkRepo       = "large-diff"
	benchmarkBaseSHA    = "0000000000000000000000000000000000000000"
	benchmarkBaseBranch = "main"
)

type benchmarkScenario struct {
	number              int
	files               int
	changedLinesPerFile int
	headSHA             string
}

func main() {
	dbPath := flag.String("db-path", defaultDBPath, "SQLite benchmark database path")
	flag.Parse()
	if err := run(context.Background(), *dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "bistro-benchseed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("seeded github:bench/large-diff:1 (1000 files) and github:bench/large-diff:2 (1 file)")
}

func run(ctx context.Context, dbPath string) (runErr error) {
	if strings.TrimSpace(dbPath) == "" {
		return errors.New("db-path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), databaseDirMode); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	lock := flock.New(dbPath + ".lock")
	isLocked, err := lock.TryLock()
	if err != nil {
		return fmt.Errorf("acquire database lock: %w", err)
	}
	if !isLocked {
		return fmt.Errorf("another bistro process is already using %s", dbPath)
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("release database lock: %w", err))
		}
	}()

	db, err := store.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close database: %w", err))
		}
	}()
	if err := store.NewMigrationRunner(slog.Default(), db.DB).Run(ctx); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return seed(ctx, db)
}

func seed(ctx context.Context, db *store.DB) error {
	scenarios := []benchmarkScenario{
		{number: 1, files: 1000, changedLinesPerFile: 10, headSHA: "1111111111111111111111111111111111111111"},
		{number: 2, files: 1, changedLinesPerFile: 10000, headSHA: "2222222222222222222222222222222222222222"},
	}
	results := make([]*writer.FetchedPR, 0, len(scenarios))
	for _, scenario := range scenarios {
		results = append(results, generateBenchmarkPR(scenario))
	}
	// The sync writer can skip failed PRs while committing the rest of a batch.
	written := 0
	prWriter := writer.New(nil, db).WithPostCommit(func(prIDs []string) { written = len(prIDs) })
	if err := prWriter.WriteSyncResultsContext(ctx, results); err != nil {
		return fmt.Errorf("write benchmark PRs: %w", err)
	}
	if written != len(results) {
		return fmt.Errorf("wrote %d of %d benchmark PRs", written, len(results))
	}
	return nil
}

func generateBenchmarkPR(scenario benchmarkScenario) *writer.FetchedPR {
	now := time.Now().UTC()
	result := &writer.FetchedPR{
		PR: &provider.PR{
			ID:         fmt.Sprintf("%s:%s/%s:%d", benchmarkProvider, benchmarkOwner, benchmarkRepo, scenario.number),
			Provider:   benchmarkProvider,
			Owner:      benchmarkOwner,
			Repo:       benchmarkRepo,
			Number:     scenario.number,
			Title:      fmt.Sprintf("bench: %d %s x %d changed lines", scenario.files, fileNoun(scenario.files), scenario.changedLinesPerFile),
			Author:     benchmarkOwner,
			State:      "open",
			BaseBranch: benchmarkBaseBranch,
			HeadBranch: fmt.Sprintf("bench-%d", scenario.number),
			BaseSHA:    benchmarkBaseSHA,
			HeadSHA:    scenario.headSHA,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		DiffComputed: true,
		MergeBaseSHA: benchmarkBaseSHA,
		DiffFiles:    make([]prdiff.File, 0, scenario.files),
	}
	for fileIndex := range scenario.files {
		filePath := fmt.Sprintf("src/generated/file_%05d.ts", fileIndex)
		patch := generateFilePatch(filePath, scenario.changedLinesPerFile)
		result.DiffFiles = append(result.DiffFiles, prdiff.File{
			FilePath: filePath, Status: "modified", DiffStatus: "ok",
			Patch: patch, PatchBytes: len(patch), HunkCount: 1, PatchID: diff.PatchID(patch),
			Additions: scenario.changedLinesPerFile, Deletions: scenario.changedLinesPerFile,
		})
		result.DiffSize += int64(len(patch))
	}
	return result
}

func fileNoun(files int) string {
	if files == 1 {
		return "file"
	}
	return "files"
}

// Match bench-large-diff.mjs generateDiff exactly: full headers, no trailing newline.
func generateFilePatch(filePath string, changedLines int) string {
	var patch strings.Builder
	fmt.Fprintf(&patch, "diff --git a/%s b/%s\nindex 0000000..1111111 100644\n--- a/%s\n+++ b/%s\n@@ -1,%d +1,%d @@",
		filePath, filePath, filePath, filePath, changedLines, changedLines)
	for line := range changedLines {
		fmt.Fprintf(&patch, "\n-export const value_%d = %d;\n+export const value_%d = %d;", line, line, line, line+1)
	}
	return patch.String()
}
