package notify

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_Notifier_PublishAndConsume(t *testing.T) {
	n := New(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track consumed events
	consumed := make(chan AssignedEvent, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-n.ch:
				consumed <- e
			}
		}
	}()

	n.Publish("task-1", "user-1")

	select {
	case e := <-consumed:
		assert.Equal(t, "task-1", e.TaskID)
		assert.Equal(t, "user-1", e.AssigneeID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("event not consumed within 100ms")
	}
}

func Test_Notifier_DropWhenFull(t *testing.T) {
	n := New(1)
	// Fill the buffer
	n.Publish("task-1", "user-1")
	// This should drop without blocking
	n.Publish("task-2", "user-2")

	// Only 1 event should be in channel
	assert.Equal(t, 1, len(n.ch))
}

func Test_Notifier_RunConsumes(t *testing.T) {
	n := New(10)
	ctx, cancel := context.WithCancel(context.Background())

	go n.Run(ctx)

	n.Publish("task-1", "user-1")

	// Give Run time to consume
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Channel should be empty after Run consumed it
	assert.Equal(t, 0, len(n.ch))
}
