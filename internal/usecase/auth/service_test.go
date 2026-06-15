package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/service"
)

// --- fakes ---

type fakeUserRepo struct {
	mu      sync.Mutex
	byID    map[int64]*entity.User
	byEmail map[string]*entity.User
	nextID  int64
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byID:    map[int64]*entity.User{},
		byEmail: map[string]*entity.User{},
	}
}

func (r *fakeUserRepo) Create(_ context.Context, user *entity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byEmail[user.Email]; ok {
		return domain.ErrEmailAlreadyExists
	}
	r.nextID++
	user.ID = r.nextID
	user.CreatedAt = time.Now().UTC()
	user.UpdatedAt = user.CreatedAt
	clone := *user
	r.byID[user.ID] = &clone
	r.byEmail[user.Email] = &clone
	return nil
}

func (r *fakeUserRepo) FindByID(_ context.Context, id int64) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (r *fakeUserRepo) FindByEmail(_ context.Context, email string) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (r *fakeUserRepo) FindByIDAndRole(_ context.Context, id int64, role string) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok || u.Role != role {
		return nil, domain.ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (r *fakeUserRepo) UpdateName(_ context.Context, id int64, name string) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	u.Name = name
	u.UpdatedAt = time.Now().UTC()
	clone := *u
	return &clone, nil
}

type fakeSessionRepo struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]*entity.AuthSession
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: map[uuid.UUID]*entity.AuthSession{}}
}

func (r *fakeSessionRepo) Create(_ context.Context, s *entity.AuthSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s.CreatedAt = time.Now().UTC()
	s.UpdatedAt = s.CreatedAt
	clone := *s
	r.sessions[s.ID] = &clone
	return nil
}

func (r *fakeSessionRepo) FindActiveByID(_ context.Context, id uuid.UUID) (*entity.AuthSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	if s.RevokedAt != nil || s.ExpiresAt.Before(time.Now().UTC()) {
		return nil, domain.ErrSessionNotFound
	}
	clone := *s
	return &clone, nil
}

func (r *fakeSessionRepo) Revoke(_ context.Context, id uuid.UUID, revokedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok || s.RevokedAt != nil {
		return nil
	}
	rt := revokedAt
	s.RevokedAt = &rt
	return nil
}

type fakePassword struct{}

func (fakePassword) Hash(p string) (string, error) { return "hash:" + p, nil }
func (fakePassword) Compare(hash, p string) error {
	if hash == "hash:"+p {
		return nil
	}
	return domain.ErrInvalidCredentials
}

type fakeTokens struct{}

func (fakeTokens) GenerateAccessToken(_ int64, sessionID uuid.UUID, _ time.Time) (string, error) {
	return "token-" + sessionID.String(), nil
}
func (fakeTokens) ParseAccessToken(_ string) (*service.TokenClaims, error) {
	return nil, domain.ErrUnauthorized
}

// --- helpers ---

func buildService() (*Service, *fakeUserRepo, *fakeSessionRepo) {
	ur := newFakeUserRepo()
	sr := newFakeSessionRepo()
	svc := NewService(ur, sr, fakePassword{}, fakeTokens{}, time.Hour)
	return svc, ur, sr
}

// --- tests ---

func TestRegister_Success(t *testing.T) {
	svc, ur, sr := buildService()

	res, err := svc.Register(context.Background(), RegisterInput{
		Name:     "Alice",
		Email:    "  ALICE@Example.com ",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if res.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if res.TokenType != "Bearer" {
		t.Errorf("token type: want Bearer, got %q", res.TokenType)
	}
	if res.ExpiresInSeconds != 3600 {
		t.Errorf("expires_in: want 3600, got %d", res.ExpiresInSeconds)
	}
	if res.User.Email != "alice@example.com" {
		t.Errorf("email should be normalised: got %q", res.User.Email)
	}
	if len(ur.byID) != 1 {
		t.Fatalf("expected one user persisted, got %d", len(ur.byID))
	}
	if len(sr.sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sr.sessions))
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	svc, _, _ := buildService()
	input := RegisterInput{Name: "Alice", Email: "alice@example.com", Password: "password123"}
	if _, err := svc.Register(context.Background(), input); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	_, err := svc.Register(context.Background(), input)
	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}
}

func TestRegister_ValidationFailures(t *testing.T) {
	svc, _, _ := buildService()
	cases := []struct {
		name  string
		input RegisterInput
	}{
		{"short name", RegisterInput{Name: "A", Email: "a@b.com", Password: "password123"}},
		{"no email at-sign", RegisterInput{Name: "Alice", Email: "invalid", Password: "password123"}},
		{"short password", RegisterInput{Name: "Alice", Email: "a@b.com", Password: "short"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Register(context.Background(), tc.input)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestLogin_Success(t *testing.T) {
	svc, _, _ := buildService()
	if _, err := svc.Register(context.Background(), RegisterInput{
		Name: "Alice", Email: "alice@example.com", Password: "password123",
	}); err != nil {
		t.Fatalf("setup register failed: %v", err)
	}
	res, err := svc.Login(context.Background(), LoginInput{
		Email: " ALICE@example.com ", Password: "password123",
	})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if res.AccessToken == "" {
		t.Fatal("expected access token")
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	svc, _, _ := buildService()
	_, err := svc.Login(context.Background(), LoginInput{
		Email: "nobody@example.com", Password: "password123",
	})
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, _, _ := buildService()
	if _, err := svc.Register(context.Background(), RegisterInput{
		Name: "Alice", Email: "alice@example.com", Password: "password123",
	}); err != nil {
		t.Fatalf("setup register failed: %v", err)
	}
	_, err := svc.Login(context.Background(), LoginInput{
		Email: "alice@example.com", Password: "wrong-password",
	})
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogout_Revokes(t *testing.T) {
	svc, _, sr := buildService()
	if _, err := svc.Register(context.Background(), RegisterInput{
		Name: "Alice", Email: "alice@example.com", Password: "password123",
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	var sid uuid.UUID
	for id := range sr.sessions {
		sid = id
	}

	if err := svc.Logout(context.Background(), sid); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}
	if sr.sessions[sid].RevokedAt == nil {
		t.Fatal("expected session to be revoked")
	}

	if err := svc.Logout(context.Background(), sid); err != nil {
		t.Fatalf("Logout (second call) failed: %v", err)
	}
}
