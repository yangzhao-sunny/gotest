package project

import "time"

type Project struct {
	ID          string
	OwnerID     string
	Name        string
	Description *string
	CreatedAt   time.Time
}
