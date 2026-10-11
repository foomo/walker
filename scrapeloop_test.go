package walker

import (
	"testing"
	"time"

	"github.com/foomo/walker/vo"
	"github.com/stretchr/testify/assert"
)

func TestPublishLatest(t *testing.T) {
	ch := make(chan vo.Status, 1)
	first := vo.Status{Completed: time.Unix(1, 0)}
	second := vo.Status{Completed: time.Unix(2, 0)}

	done := make(chan struct{})
	go func() {
		// nobody receives - must not block
		publishLatest(ch, first)
		publishLatest(ch, second)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publishLatest blocked")
	}

	assert.Equal(t, second.Completed, (<-ch).Completed, "the latest status wins")
	select {
	case <-ch:
		t.Fatal("older status must have been dropped")
	default:
	}
}
