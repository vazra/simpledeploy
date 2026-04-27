package store

import (
	"context"
	"testing"
)

func TestRecipePullsRecordAndCount(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"a", "a", "b", "a", "c", "b"} {
		if err := st.RecordRecipePull(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	counts, err := st.RecipePullCounts(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"a": 3, "b": 2, "c": 1}
	if len(counts) != 3 {
		t.Fatalf("want 3, got %d (%+v)", len(counts), counts)
	}
	for _, rc := range counts {
		if want[rc.RecipeID] != rc.Count {
			t.Fatalf("recipe %s: want %d got %d", rc.RecipeID, want[rc.RecipeID], rc.Count)
		}
	}
	if counts[0].RecipeID != "a" {
		t.Fatalf("expected 'a' first, got %s", counts[0].RecipeID)
	}
}
