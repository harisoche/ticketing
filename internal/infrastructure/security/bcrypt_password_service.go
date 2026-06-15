package security

import (
	"errors"

	"golang.org/x/crypto/bcrypt"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/service"
)

type bcryptPasswordService struct {
	cost int
}

// NewBcryptPasswordService returns a PasswordService backed by bcrypt.
// cost=0 uses bcrypt.DefaultCost (currently 10).
func NewBcryptPasswordService(cost int) service.PasswordService {
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	return &bcryptPasswordService{cost: cost}
}

func (s *bcryptPasswordService) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Compare returns domain.ErrInvalidCredentials when the password does not
// match the hash, hiding the underlying library error from callers.
func (s *bcryptPasswordService) Compare(hash string, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err == nil {
		return nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return domain.ErrInvalidCredentials
	}
	return err
}
