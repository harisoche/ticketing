package user

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
)

type fakeUserRepo struct {
	mu     sync.Mutex
	byID   map[int64]*entity.User
	nextID int64
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[int64]*entity.User{}}
}

func (r *fakeUserRepo) Create(_ context.Context, u *entity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	u.ID = r.nextID
	u.CreatedAt = time.Now().UTC()
	u.UpdatedAt = u.CreatedAt
	clone := *u
	r.byID[u.ID] = &clone
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

func (r *fakeUserRepo) FindByEmail(_ context.Context, _ string) (*entity.User, error) {
	return nil, domain.ErrUserNotFound
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

func seed(repo *fakeUserRepo) *entity.User {
	u := &entity.User{Name: "Alice", Email: "alice@example.com", PasswordHash: "x"}
	_ = repo.Create(context.Background(), u)
	return u
}

func TestGetProfile_Success(t *testing.T) {
	repo := newFakeUserRepo()
	user := seed(repo)
	svc := NewService(repo)

	out, err := svc.GetProfile(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}
	if out.Email != "alice@example.com" {
		t.Errorf("unexpected email: %q", out.Email)
	}
}

func TestGetProfile_Missing(t *testing.T) {
	svc := NewService(newFakeUserRepo())
	_, err := svc.GetProfile(context.Background(), 999)
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestUpdateProfile_Success(t *testing.T) {
	repo := newFakeUserRepo()
	user := seed(repo)
	svc := NewService(repo)

	out, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileInput{Name: " Alice 2 "})
	if err != nil {
		t.Fatalf("UpdateProfile failed: %v", err)
	}
	if out.Name != "Alice 2" {
		t.Errorf("expected trimmed name, got %q", out.Name)
	}
}

func TestUpdateProfile_RejectsBlank(t *testing.T) {
	repo := newFakeUserRepo()
	user := seed(repo)
	svc := NewService(repo)

	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileInput{Name: "   "})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestUpdateProfile_RejectsTooLong(t *testing.T) {
	repo := newFakeUserRepo()
	user := seed(repo)
	svc := NewService(repo)

	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileInput{Name: strings.Repeat("x", 101)})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}
