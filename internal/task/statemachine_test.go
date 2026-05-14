package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ValidateTransition(t *testing.T) {
	// Allowed transitions
	assert.NoError(t, ValidateTransition(StatusTodo, StatusDoing))
	assert.NoError(t, ValidateTransition(StatusDoing, StatusDone))
	assert.NoError(t, ValidateTransition(StatusDone, StatusArchived))
	assert.NoError(t, ValidateTransition(StatusDone, StatusDoing)) // reopen

	// Forbidden transitions
	assert.Error(t, ValidateTransition(StatusTodo, StatusDone))
	assert.Error(t, ValidateTransition(StatusTodo, StatusArchived))
	assert.Error(t, ValidateTransition(StatusDoing, StatusTodo))
	assert.Error(t, ValidateTransition(StatusDoing, StatusArchived))
	assert.Error(t, ValidateTransition(StatusArchived, StatusDoing))
	assert.Error(t, ValidateTransition(StatusArchived, StatusDone))
}
