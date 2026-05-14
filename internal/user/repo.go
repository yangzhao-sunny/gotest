package user

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) FindByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, display_name, created_at FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindByID: %w", err)
	}
	return &u, nil
}

func (r *Repo) UpdateDisplayName(ctx context.Context, id, displayName string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`UPDATE users SET display_name = $2 WHERE id = $1
		 RETURNING id, email, display_name, created_at`,
		id, displayName,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("UpdateDisplayName: %w", err)
	}
	return &u, nil
}
