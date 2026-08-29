package infrastructure

import (
	"context"
	"sync"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

type MemoryNotifier struct {
	mu          sync.RWMutex
	subscribers map[uint]map[chan models.Notification]struct{}
}

func NewMemoryNotifier() *MemoryNotifier {
	return &MemoryNotifier{subscribers: make(map[uint]map[chan models.Notification]struct{})}
}

func (n *MemoryNotifier) Notify(ctx context.Context, notification models.Notification) error {
	n.mu.RLock()
	channels := make([]chan models.Notification, 0, len(n.subscribers[notification.UserID]))
	for ch := range n.subscribers[notification.UserID] {
		channels = append(channels, ch)
	}
	n.mu.RUnlock()
	for _, ch := range channels {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- notification:
		default:
		}
	}
	return nil
}

func (n *MemoryNotifier) Subscribe(userID uint, buffer int) (<-chan models.Notification, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan models.Notification, buffer)
	n.mu.Lock()
	if n.subscribers[userID] == nil {
		n.subscribers[userID] = make(map[chan models.Notification]struct{})
	}
	n.subscribers[userID][ch] = struct{}{}
	n.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			n.mu.Lock()
			delete(n.subscribers[userID], ch)
			if len(n.subscribers[userID]) == 0 {
				delete(n.subscribers, userID)
			}
			close(ch)
			n.mu.Unlock()
		})
	}
}
