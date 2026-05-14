package notify

import (
	"context"
	"log/slog"
)

type AssignedEvent struct {
	TaskID     string
	AssigneeID string
}

type Notifier struct {
	ch chan AssignedEvent
}

func New(bufSize int) *Notifier {
	return &Notifier{ch: make(chan AssignedEvent, bufSize)}
}

// Publish sends an event non-blocking; drops and warns if the channel is full.
func (n *Notifier) Publish(taskID, assigneeID string) {
	select {
	case n.ch <- AssignedEvent{TaskID: taskID, AssigneeID: assigneeID}:
	default:
		slog.Warn("notify: channel full, dropping event", "task_id", taskID, "assignee_id", assigneeID)
	}
}

// Run consumes the event channel until the context is cancelled.
func (n *Notifier) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-n.ch:
			slog.Info("assigned", "task_id", e.TaskID, "assignee_id", e.AssigneeID)
		}
	}
}
