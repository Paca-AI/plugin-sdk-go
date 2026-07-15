package plugintest

import "testing"

// These tests cover InMemoryDB's WHERE-clause evaluation directly (SELECT,
// UPDATE, DELETE), since a regression here silently produces false negatives
// in every plugin's own test suite rather than a visible failure.

func seedWidgets(db *InMemoryDB) {
	db.SeedRows("widgets",
		[]string{"id", "owner_id", "deleted_at"},
		[][]any{
			{"1", "alice", nil},
			{"2", "alice", "2026-01-01"},
			{"3", "bob", nil},
		},
	)
}

func TestInMemoryDB_Select_MultiConditionWhere(t *testing.T) {
	db := newInMemoryDB()
	seedWidgets(db)

	result, err := db.Query(
		"SELECT id FROM widgets WHERE owner_id = $1 AND deleted_at IS NULL",
		[]any{"alice"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row (alice's non-deleted widget), got %+v", result.Rows)
	}
	if result.Rows[0][0] != "1" {
		t.Fatalf("expected widget 1, got %+v", result.Rows[0])
	}
}

func TestInMemoryDB_Select_IsNotNull(t *testing.T) {
	db := newInMemoryDB()
	seedWidgets(db)

	result, err := db.Query("SELECT id FROM widgets WHERE deleted_at IS NOT NULL", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "2" {
		t.Fatalf("expected only widget 2, got %+v", result.Rows)
	}
}

func TestInMemoryDB_Delete_MultiConditionWhere(t *testing.T) {
	db := newInMemoryDB()
	seedWidgets(db)

	// Only widget 1 matches both conditions; widget 3 shares owner_id="bob"
	// but not id=1, and must survive.
	affected, err := db.Exec("DELETE FROM widgets WHERE id = $1 AND owner_id = $2", []any{"1", "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("expected 1 row deleted, got %d", affected)
	}
	remaining := db.AllRows("widgets")
	if len(remaining) != 2 {
		t.Fatalf("expected 2 rows remaining, got %+v", remaining)
	}
}

func TestInMemoryDB_Delete_DoesNotMatchOtherRowsSharingOneCondition(t *testing.T) {
	db := newInMemoryDB()
	seedWidgets(db)

	// id=3 belongs to bob, not alice — must not be deleted even though it
	// shares no id with widget 1/2. This guards against a WHERE evaluator
	// that only checks the first condition (owner_id) and ignores the rest.
	_, err := db.Exec("DELETE FROM widgets WHERE id = $1 AND owner_id = $2", []any{"3", "alice"})
	if err != nil {
		t.Fatal(err)
	}
	remaining := db.AllRows("widgets")
	if len(remaining) != 3 {
		t.Fatalf("expected no rows deleted (id/owner mismatch), got %+v", remaining)
	}
}
