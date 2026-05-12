package comment

import "time"

type Comment struct {
	ID        string
	TaskID    string
	AuthorID  string
	Body      string
	CreatedAt time.Time
}
