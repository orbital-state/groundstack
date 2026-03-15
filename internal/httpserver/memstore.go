package httpserver

import "sync"

type memStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMemStore() *memStore {
	return &memStore{data: map[string][]byte{}}
}

func (s *memStore) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	if !ok {
		return nil, false
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, true
}

func (s *memStore) Put(key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	s.data[key] = cp
}

func (s *memStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}
