package comment

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) Create(ctx context.Context, taskID, authorID, body string) (*Comment, error) {
	var c Comment
	err := r.pool.QueryRow(ctx,
		`INSERT INTO task_comments (task_id, author_id, body)
		 VALUES ($1, $2, $3)
		 RETURNING id, task_id, author_id, body, created_at`,
		taskID, authorID, body,
	).Scan(&c.ID, &c.TaskID, &c.AuthorID, &c.Body, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("comment.Create: %w", err)
	}
	return &c, nil
}

func (r *Repo) List(ctx context.Context, taskID, cursor string, limit int) ([]*Comment, string, error) {
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT id, task_id, author_id, body, created_at
			 FROM task_comments
			 WHERE task_id = $1
			 ORDER BY created_at DESC, id DESC
			 LIMIT $2`,
			taskID, limit+1,
		)
	} else {
		ts, id, decErr := decodeCursor(cursor)
		if decErr != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", decErr)
		}
		rows, err = r.pool.Query(ctx,
			`SELECT id, task_id, author_id, body, created_at
			 FROM task_comments
			 WHERE task_id = $1
			   AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC
			 LIMIT $4`,
			taskID, ts, id, limit+1,
		)
	}
	if err != nil {
		return nil, "", fmt.Errorf("comment.List: %w", err)
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.TaskID, &c.AuthorID, &c.Body, &c.CreatedAt); err != nil {
			return nil, "", err
		}
		comments = append(comments, &c)
	}
	if rows.Err() != nil {
		return nil, "", rows.Err()
	}

	nextCursor := ""
	if len(comments) > limit {
		last := comments[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		comments = comments[:limit]
	}
	return comments, nextCursor, nil
}

func (r *Repo) TaskExistsForOwner(ctx context.Context, taskID, ownerID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM tasks t
		   JOIN projects p ON p.id = t.project_id AND p.owner_id = $2 AND p.deleted_at IS NULL
		   WHERE t.id = $1 AND t.deleted_at IS NULL
		 )`,
		taskID, ownerID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("comment.TaskExistsForOwner: %w", err)
	}
	return exists, nil
}

func encodeCursor(createdAt time.Time, id string) string {
	raw := createdAt.Format(time.RFC3339Nano) + "," + id
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

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
