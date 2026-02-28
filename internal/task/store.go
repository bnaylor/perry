package task

import "context"

// Store persists tasks. Implementations may be in-memory, SQLite, etc.
type Store interface {
	Create(ctx context.Context, t *Task) error
	Get(ctx context.Context, id string) (*Task, error)
	Update(ctx context.Context, t *Task) error
	List(ctx context.Context) ([]*Task, error)
}
