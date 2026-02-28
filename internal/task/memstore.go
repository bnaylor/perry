package task

import (
	"context"
	"fmt"
	"sync"
)

// MemStore is a thread-safe in-memory task store.
type MemStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewMemStore() *MemStore {
	return &MemStore{tasks: make(map[string]*Task)}
}

func (s *MemStore) Create(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tasks[t.ID]; exists {
		return fmt.Errorf("task %s already exists", t.ID)
	}
	s.tasks[t.ID] = t
	return nil
}

func (s *MemStore) Get(_ context.Context, id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	return t, nil
}

func (s *MemStore) Update(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tasks[t.ID]; !exists {
		return fmt.Errorf("task %s not found", t.ID)
	}
	s.tasks[t.ID] = t
	return nil
}

func (s *MemStore) List(_ context.Context) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, t)
	}
	return result, nil
}
