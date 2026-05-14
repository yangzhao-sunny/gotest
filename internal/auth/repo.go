package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) CreateUser(ctx context.Context, email, hash, displayName string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name)
		 VALUES ($1, $2, $3)
		 RETURNING id, email, password_hash, display_name, created_at`,
		email, hash, displayName,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateUser: %w", err)
	}
	return &u, nil
}

func (r *Repo) FindByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, display_name, created_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindByEmail: %w", err)
	}
	return &u, nil
}

func (r *Repo) FindByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, display_name, created_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindByID: %w", err)
	}
	return &u, nil
}

func (r *Repo) CreateRefreshToken(ctx context.Context, userID, token string, expiresAt time.Time) (*RefreshToken, error) {
	var rt RefreshToken
	err := r.pool.QueryRow(ctx,
		`INSERT INTO refresh_tokens (user_id, token, expires_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, token, expires_at, revoked_at, created_at`,
		userID, token, expiresAt,
	).Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &rt.RevokedAt, &rt.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateRefreshToken: %w", err)
	}
	return &rt, nil
}

func (r *Repo) FindRefreshToken(ctx context.Context, token string) (*RefreshToken, error) {
	var rt RefreshToken
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token, expires_at, revoked_at, created_at
		 FROM refresh_tokens WHERE token = $1`,
		token,
	).Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &rt.RevokedAt, &rt.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindRefreshToken: %w", err)
	}
	return &rt, nil
}

func (r *Repo) RevokeRefreshToken(ctx context.Context, token string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW() WHERE token = $1`,
		token,
	)
	return err
}
