package stats

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Stats struct {
	Todo     int `json:"todo"`
	Doing    int `json:"doing"`
	Done     int `json:"done"`
	Archived int `json:"archived"`
	Overdue  int `json:"overdue"`
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) GetStats(ctx context.Context, projectID string) (*Stats, error) {
	var s Stats
	err := r.pool.QueryRow(ctx,
		`SELECT
		   COUNT(*) FILTER (WHERE status = 'todo') AS todo,
		   COUNT(*) FILTER (WHERE status = 'doing') AS doing,
		   COUNT(*) FILTER (WHERE status = 'done') AS done,
		   COUNT(*) FILTER (WHERE status = 'archived') AS archived,
		   COUNT(*) FILTER (WHERE due_date < CURRENT_DATE AND status IN ('todo', 'doing')) AS overdue
		 FROM tasks
		 WHERE project_id = $1 AND deleted_at IS NULL`,
		projectID,
	).Scan(&s.Todo, &s.Doing, &s.Done, &s.Archived, &s.Overdue)
	if err != nil {
		return nil, fmt.Errorf("stats.GetStats: %w", err)
	}
	return &s, nil
}
