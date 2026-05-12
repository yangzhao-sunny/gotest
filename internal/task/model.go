package task

import "time"

type Status string

const (
	StatusTodo     Status = "todo"
	StatusDoing    Status = "doing"
	StatusDone     Status = "done"
	StatusArchived Status = "archived"
)

type Task struct {
	ID         string
	ProjectID  string
	Title      string
	Status     Status
	Priority   int
	AssigneeID *string
	DueDate    *string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type UpdateFields struct {
	Title      *string
	Status     *Status
	Priority   *int
	AssigneeID *string
	DueDate    *string
}
