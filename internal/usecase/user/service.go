package user

import (
	"context"
	"strings"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
)

const (
	minNameLength = 2
	maxNameLength = 100
)

type Service struct {
	users repository.UserRepository
}

func NewService(users repository.UserRepository) *Service {
	return &Service{users: users}
}

func (s *Service) GetProfile(ctx context.Context, userID int64) (*UserOutput, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toOutput(u), nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID int64, input UpdateProfileInput) (*UserOutput, error) {
	name := strings.TrimSpace(input.Name)
	if len(name) < minNameLength || len(name) > maxNameLength {
		return nil, domain.ErrInvalidInput
	}
	u, err := s.users.UpdateName(ctx, userID, name)
	if err != nil {
		return nil, err
	}
	return toOutput(u), nil
}

func toOutput(u *entity.User) *UserOutput {
	return &UserOutput{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
