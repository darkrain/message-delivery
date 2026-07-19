package delivery

import "sync"

type MemoryIdempotencyStore struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{seen: map[string]struct{}{}}
}

func (s *MemoryIdempotencyStore) Seen(eventID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[eventID]
	return ok
}

func (s *MemoryIdempotencyStore) Mark(eventID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[eventID] = struct{}{}
}
