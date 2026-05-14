package task

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

func (r *Repo) Create(ctx context.Context, projectID, title string, priority int, assigneeID *string, dueDate *string) (*Task, error) {
	var t Task
	err := r.pool.QueryRow(ctx,
		`INSERT INTO tasks (project_id, title, priority, assignee_id, due_date)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, project_id, title, status, priority, assignee_id, due_date::text, created_at, updated_at`,
		projectID, title, priority, assigneeID, dueDate,
	).Scan(&t.ID, &t.ProjectID, &t.Title, &t.Status, &t.Priority, &t.AssigneeID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("task.Create: %w", err)
	}
	return &t, nil
}

func (r *Repo) ListByProject(ctx context.Context, projectID string, statusFilter *Status, assigneeFilter *string, cursor string, limit int) ([]*Task, string, error) {
	conditions := []string{"t.project_id = $1", "t.deleted_at IS NULL"}
	args := []any{projectID}
	argIdx := 2

	if statusFilter != nil {
		conditions = append(conditions, fmt.Sprintf("t.status = $%d", argIdx))
		args = append(args, string(*statusFilter))
		argIdx++
	}
	if assigneeFilter != nil {
		conditions = append(conditions, fmt.Sprintf("t.assignee_id = $%d", argIdx))
		args = append(args, *assigneeFilter)
		argIdx++
	}

	if cursor != "" {
		ts, id, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		conditions = append(conditions, fmt.Sprintf("(t.created_at, t.id) < ($%d, $%d)", argIdx, argIdx+1))
		args = append(args, ts, id)
		argIdx += 2
	}

	where := strings.Join(conditions, " AND ")
	query := fmt.Sprintf(
		`SELECT t.id, t.project_id, t.title, t.status, t.priority, t.assignee_id, t.due_date::text, t.created_at, t.updated_at
		 FROM tasks t
		 WHERE %s
		 ORDER BY t.created_at DESC, t.id DESC
		 LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("task.ListByProject: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Status, &t.Priority, &t.AssigneeID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, "", err
		}
		tasks = append(tasks, &t)
	}
	if rows.Err() != nil {
		return nil, "", rows.Err()
	}

	nextCursor := ""
	if len(tasks) > limit {
		last := tasks[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		tasks = tasks[:limit]
	}
	return tasks, nextCursor, nil
}

func (r *Repo) FindByIDForOwner(ctx context.Context, id, ownerID string) (*Task, error) {
	var t Task
	err := r.pool.QueryRow(ctx,
		`SELECT t.id, t.project_id, t.title, t.status, t.priority, t.assignee_id, t.due_date::text, t.created_at, t.updated_at
		 FROM tasks t
		 JOIN projects p ON p.id = t.project_id AND p.owner_id = $2 AND p.deleted_at IS NULL
		 WHERE t.id = $1 AND t.deleted_at IS NULL`,
		id, ownerID,
	).Scan(&t.ID, &t.ProjectID, &t.Title, &t.Status, &t.Priority, &t.AssigneeID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("task.FindByIDForOwner: %w", err)
	}
	return &t, nil
}

func (r *Repo) Update(ctx context.Context, id string, fields UpdateFields) (*Task, error) {
	var t Task
	err := r.pool.QueryRow(ctx,
		`UPDATE tasks SET
		   title = COALESCE($2, title),
		   status = COALESCE($3::task_status, status),
		   priority = COALESCE($4, priority),
		   assignee_id = CASE WHEN $5::boolean THEN $6::uuid ELSE assignee_id END,
		   due_date = CASE WHEN $7::boolean THEN $8::date ELSE due_date END,
		   updated_at = NOW()
		 WHERE id = $1 AND deleted_at IS NULL
		 RETURNING id, project_id, title, status, priority, assignee_id, due_date::text, created_at, updated_at`,
		id,
		fields.Title,
		fields.Status,
		fields.Priority,
		fields.AssigneeID != nil, fields.AssigneeID,
		fields.DueDate != nil, fields.DueDate,
	).Scan(&t.ID, &t.ProjectID, &t.Title, &t.Status, &t.Priority, &t.AssigneeID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("task.Update: %w", err)
	}
	return &t, nil
}

func (r *Repo) SoftDelete(ctx context.Context, id, ownerID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tasks SET deleted_at = NOW()
		 WHERE id = $1
		   AND deleted_at IS NULL
		   AND project_id IN (SELECT id FROM projects WHERE owner_id = $2 AND deleted_at IS NULL)`,
		id, ownerID,
	)
	return err
}

// SoftDeleteByProject soft-deletes all tasks in a project using the given transaction.
// This satisfies project.TaskDeleter interface.
func (r *Repo) SoftDeleteByProject(ctx context.Context, tx pgx.Tx, projectID string) error {
	_, err := tx.Exec(ctx,
		`UPDATE tasks SET deleted_at = NOW() WHERE project_id = $1 AND deleted_at IS NULL`,
		projectID,
	)
	return err
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
