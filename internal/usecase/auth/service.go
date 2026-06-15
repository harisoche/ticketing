package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/domain/service"
)

const (
	minPasswordLength = 8
	maxPasswordBytes  = 72
	minNameLength     = 2
	maxNameLength     = 100
	maxEmailLength    = 255
	tokenType         = "Bearer"
)

// Clock allows tests to inject a fixed time source.
type Clock func() time.Time

// Service is the authentication use-case layer.
type Service struct {
	users     repository.UserRepository
	sessions  repository.AuthSessionRepository
	password  service.PasswordService
	tokens    service.TokenService
	accessTTL time.Duration
	clock     Clock
}

func NewService(
	users repository.UserRepository,
	sessions repository.AuthSessionRepository,
	password service.PasswordService,
	tokens service.TokenService,
	accessTTL time.Duration,
) *Service {
	return &Service{
		users:     users,
		sessions:  sessions,
		password:  password,
		tokens:    tokens,
		accessTTL: accessTTL,
		clock:     func() time.Time { return time.Now().UTC() },
	}
}

// WithClock returns a copy of the service using a custom time source.
// Used by tests.
func (s *Service) WithClock(clock Clock) *Service {
	cp := *s
	cp.clock = clock
	return &cp
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (*AuthResult, error) {
	name := strings.TrimSpace(input.Name)
	email := strings.ToLower(strings.TrimSpace(input.Email))

	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	if err := validatePassword(input.Password); err != nil {
		return nil, err
	}

	hash, err := s.password.Hash(input.Password)
	if err != nil {
		return nil, err
	}

	user := &entity.User{
		Name:         name,
		Email:        email,
		PasswordHash: hash,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	return s.issueSession(ctx, user)
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*AuthResult, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if email == "" || input.Password == "" {
		return nil, domain.ErrInvalidCredentials
	}

	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}

	if err := s.password.Compare(user.PasswordHash, input.Password); err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}

	return s.issueSession(ctx, user)
}

// Logout revokes the bearer-token session. Idempotent — revoking an
// already-revoked session is not an error.
func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	return s.sessions.Revoke(ctx, sessionID, s.clock())
}

func (s *Service) issueSession(ctx context.Context, user *entity.User) (*AuthResult, error) {
	now := s.clock()
	expiresAt := now.Add(s.accessTTL)

	session := &entity.AuthSession{
		ID:        uuid.New(),
		UserID:    user.ID,
		ExpiresAt: expiresAt,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, err
	}

	token, err := s.tokens.GenerateAccessToken(user.ID, session.ID, expiresAt)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		AccessToken:      token,
		TokenType:        tokenType,
		ExpiresInSeconds: int64(s.accessTTL.Seconds()),
		User: PublicUser{
			ID:        user.ID,
			Name:      user.Name,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	}, nil
}

func validateName(name string) error {
	if len(name) < minNameLength || len(name) > maxNameLength {
		return domain.ErrInvalidInput
	}
	return nil
}

func validateEmail(email string) error {
	if email == "" || len(email) > maxEmailLength {
		return domain.ErrInvalidInput
	}
	if !strings.Contains(email, "@") {
		return domain.ErrInvalidInput
	}
	return nil
}

func validatePassword(pw string) error {
	if len(pw) < minPasswordLength || len(pw) > maxPasswordBytes {
		return domain.ErrInvalidInput
	}
	return nil
}
