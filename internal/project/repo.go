package project

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TaskDeleter is implemented by the task repo to support cascade soft-deletes.
type TaskDeleter interface {
	SoftDeleteByProject(ctx context.Context, tx pgx.Tx, projectID string) error
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) Create(ctx context.Context, ownerID, name string, description *string) (*Project, error) {
	var p Project
	err := r.pool.QueryRow(ctx,
		`INSERT INTO projects (owner_id, name, description)
		 VALUES ($1, $2, $3)
		 RETURNING id, owner_id, name, description, created_at`,
		ownerID, name, description,
	).Scan(&p.ID, &p.OwnerID, &p.Name, &p.Description, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("project.Create: %w", err)
	}
	return &p, nil
}

func (r *Repo) List(ctx context.Context, ownerID, cursor string, limit int) ([]*Project, string, error) {
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT id, owner_id, name, description, created_at
			 FROM projects
			 WHERE owner_id = $1 AND deleted_at IS NULL
			 ORDER BY created_at DESC, id DESC
			 LIMIT $2`,
			ownerID, limit+1,
		)
	} else {
		ts, id, decErr := decodeCursor(cursor)
		if decErr != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", decErr)
		}
		rows, err = r.pool.Query(ctx,
			`SELECT id, owner_id, name, description, created_at
			 FROM projects
			 WHERE owner_id = $1 AND deleted_at IS NULL
			   AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC
			 LIMIT $4`,
			ownerID, ts, id, limit+1,
		)
	}
	if err != nil {
		return nil, "", fmt.Errorf("project.List: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, "", err
		}
		projects = append(projects, &p)
	}
	if rows.Err() != nil {
		return nil, "", rows.Err()
	}

	nextCursor := ""
	if len(projects) > limit {
		last := projects[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		projects = projects[:limit]
	}
	return projects, nextCursor, nil
}

func (r *Repo) FindByIDAndOwner(ctx context.Context, id, ownerID string) (*Project, error) {
	var p Project
	err := r.pool.QueryRow(ctx,
		`SELECT id, owner_id, name, description, created_at
		 FROM projects
		 WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`,
		id, ownerID,
	).Scan(&p.ID, &p.OwnerID, &p.Name, &p.Description, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("project.FindByIDAndOwner: %w", err)
	}
	return &p, nil
}

func (r *Repo) Update(ctx context.Context, id, ownerID string, name *string, description *string) (*Project, error) {
	var p Project
	err := r.pool.QueryRow(ctx,
		`UPDATE projects
		 SET name = COALESCE($3, name),
		     description = CASE WHEN $4::boolean THEN $5 ELSE description END
		 WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		 RETURNING id, owner_id, name, description, created_at`,
		id, ownerID, name, description != nil, description,
	).Scan(&p.ID, &p.OwnerID, &p.Name, &p.Description, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("project.Update: %w", err)
	}
	return &p, nil
}

func (r *Repo) SoftDelete(ctx context.Context, id, ownerID string, taskRepo TaskDeleter) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("project.SoftDelete begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	ct, err := tx.Exec(ctx,
		`UPDATE projects SET deleted_at = NOW() WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`,
		id, ownerID,
	)
	if err != nil {
		return fmt.Errorf("project.SoftDelete: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil // not found or not owner — handler checks separately
	}

	if err := taskRepo.SoftDeleteByProject(ctx, tx, id); err != nil {
		return fmt.Errorf("project.SoftDelete tasks: %w", err)
	}

	return tx.Commit(ctx)
}

// encodeCursor encodes created_at + id into a base64 cursor string.
func encodeCursor(createdAt time.Time, id string) string {
	raw := createdAt.Format(time.RFC3339Nano) + "," + id
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// decodeCursor decodes a base64 cursor into (ts, id).
func decodeCursor(cursor string) (string, string, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	s := string(b)
	idx := strings.Index(s, ",")
	if idx < 0 {
		return "", "", fmt.Errorf("malformed cursor")
	}
	return s[:idx], s[idx+1:], nil
}
