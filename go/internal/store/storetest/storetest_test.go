package storetest_test

import (
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestOpenMigratesIsolatesAndClosesDatabase(t *testing.T) {
	var first *store.DB
	t.Run("migrated independent databases", func(t *testing.T) {
		first = storetest.Open(t)
		if _, err := first.Exec(`INSERT INTO repos (id, provider, owner, repo) VALUES ('github:o/r', 'github', 'o', 'r')`); err != nil {
			t.Fatalf("insert into migrated schema: %v", err)
		}

		second := storetest.Open(t)
		var count int
		if err := second.QueryRow(`SELECT COUNT(*) FROM repos`).Scan(&count); err != nil {
			t.Fatalf("query independent migrated schema: %v", err)
		}
		if count != 0 {
			t.Fatalf("second database contains %d repos from first, want 0", count)
		}
	})
	if first == nil {
		t.Fatal("subtest did not open a database")
	}
	if err := first.Ping(); err == nil {
		t.Fatal("database remains open after test cleanup")
	}
}
