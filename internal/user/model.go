package user

import "time"

type User struct {
	ID          string
	Email       string
	DisplayName string
	CreatedAt   time.Time
}
