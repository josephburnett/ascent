package textedit

import "sync"

// SaveQueue serializes content writes per document, SaveQueueKey naming the
// chain: one in flight, the rest FIFO. Text saves claim a version, so two in
// flight would claim the same one and the loser's typed input would be
// reconciled away.
type SaveQueue struct {
	mu      sync.Mutex
	busy    map[string]bool
	pending map[string][]func()
}

// SaveQueueKey names the chain a text save belongs on: one document, one
// chain. The id that owns the bytes names it, never the row the edit was
// viewed through, because the flush paths hold different ids for one document
// and two chains would let them claim the same version concurrently. An empty
// content id falls back to the viewed row, as rpc.ContentID does, and never
// to the empty chain every document would share.
func SaveQueueKey(viewedID, contentID string) string {
	if contentID != "" {
		return contentID
	}
	return viewedID
}

// NewSaveQueue returns an empty queue.
func NewSaveQueue() *SaveQueue {
	return &SaveQueue{busy: map[string]bool{}, pending: map[string][]func(){}}
}

// Enqueue schedules task on key's serial chain; different keys are
// independent. task runs on a queue goroutine and should do the blocking send
// itself rather than spawn.
func (q *SaveQueue) Enqueue(key string, task func()) {
	q.mu.Lock()
	if q.busy[key] {
		q.pending[key] = append(q.pending[key], task)
		q.mu.Unlock()
		return
	}
	q.busy[key] = true
	q.mu.Unlock()
	go q.run(key, task)
}

func (q *SaveQueue) run(key string, task func()) {
	for {
		task()
		q.mu.Lock()
		next := q.pending[key]
		if len(next) == 0 {
			q.busy[key] = false
			delete(q.pending, key)
			q.mu.Unlock()
			return
		}
		task = next[0]
		q.pending[key] = next[1:]
		q.mu.Unlock()
	}
}
