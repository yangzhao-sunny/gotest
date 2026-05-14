package task

import "fmt"

// ValidateTransition returns nil if the transition is allowed, otherwise an error
// with code "task_invalid_transition".
// Allowed transitions: todo→doing, doing→done, done→archived, done→doing (reopen).
func ValidateTransition(from, to Status) error {
	allowed := map[Status]map[Status]bool{
		StatusTodo:  {StatusDoing: true},
		StatusDoing: {StatusDone: true},
		StatusDone:  {StatusArchived: true, StatusDoing: true},
	}
	if targets, ok := allowed[from]; ok {
		if targets[to] {
			return nil
		}
	}
	return fmt.Errorf("task_invalid_transition: cannot transition from %s to %s", from, to)
}
