package store

import (
	"context"
)

// RecordRecipePull logs a community-recipe pull event. Best-effort: errors
// are returned but callers may choose to log-and-continue.
func (s *Store) RecordRecipePull(ctx context.Context, recipeID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO recipe_pulls (recipe_id) VALUES (?)`, recipeID)
	return err
}

type RecipePullCount struct {
	RecipeID string `json:"recipe_id"`
	Count    int    `json:"count"`
}

// RecipePullCounts returns top-N recipe pull counts. limit <= 0 means no limit.
func (s *Store) RecipePullCounts(ctx context.Context, limit int) ([]RecipePullCount, error) {
	q := `SELECT recipe_id, COUNT(*) AS c FROM recipe_pulls GROUP BY recipe_id ORDER BY c DESC, recipe_id ASC`
	args := []any{}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecipePullCount
	for rows.Next() {
		var rc RecipePullCount
		if err := rows.Scan(&rc.RecipeID, &rc.Count); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}
