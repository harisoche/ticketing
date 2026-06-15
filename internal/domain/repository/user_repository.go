package repository

import (
	"context"

	"ticketing-api/internal/domain/entity"
)

// UserRepository persists and retrieves users. Implementations live in
// the infrastructure layer.
type UserRepository interface {
	Create(ctx context.Context, user *entity.User) error
	FindByID(ctx context.Context, id int64) (*entity.User, error)
	FindByEmail(ctx context.Context, email string) (*entity.User, error)
	UpdateName(ctx context.Context, id int64, name string) (*entity.User, error)

	// FindByIDAndRole returns the user only when their role matches. Used by
	// the ticket use case to validate an assignee is an agent.
	FindByIDAndRole(ctx context.Context, id int64, role string) (*entity.User, error)
}
