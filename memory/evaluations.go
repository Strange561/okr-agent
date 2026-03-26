package memory

import (
	"context"
	"fmt"
	"time"
)

// Evaluation represents a stored evaluation record.
type Evaluation struct {
	ID         int64
	UserID     string
	Evaluation string
	CreatedAt  time.Time
}

// SaveEvaluation stores an evaluation result.
func (s *Store) SaveEvaluation(ctx context.Context, userID, evaluation string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO evaluation_history (user_id, evaluation) VALUES (?, ?)`,
		userID, evaluation)
	if err != nil {
		return fmt.Errorf("save evaluation: %w", err)
	}
	return nil
}

// GetEvaluations returns recent evaluations for a user.
func (s *Store) GetEvaluations(ctx context.Context, userID string, limit int) ([]Evaluation, error) {
	if limit <= 0 {
		limit = 5
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, evaluation, created_at
		 FROM evaluation_history WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`,
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query evaluations: %w", err)
	}
	defer rows.Close()

	var evals []Evaluation
	for rows.Next() {
		var e Evaluation
		if err := rows.Scan(&e.ID, &e.UserID, &e.Evaluation, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan evaluation: %w", err)
		}
		evals = append(evals, e)
	}
	return evals, rows.Err()
}
