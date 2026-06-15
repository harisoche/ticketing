package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/infrastructure/persistence/model"
)

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) repository.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *entity.User) error {
	m := model.UserModelFromEntity(user)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrEmailAlreadyExists
		}
		return err
	}
	user.ID = m.ID
	user.CreatedAt = m.CreatedAt
	user.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *userRepository) FindByID(ctx context.Context, id int64) (*entity.User, error) {
	var m model.UserModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	var m model.UserModel
	if err := r.db.WithContext(ctx).Where("email = ?", normalized).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}

func (r *userRepository) FindByIDAndRole(ctx context.Context, id int64, role string) (*entity.User, error) {
	var m model.UserModel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND role = ?", id, role).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}

func (r *userRepository) UpdateName(ctx context.Context, id int64, name string) (*entity.User, error) {
	tx := r.db.WithContext(ctx).Model(&model.UserModel{}).
		Where("id = ?", id).
		Update("name", name)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, domain.ErrUserNotFound
	}
	return r.FindByID(ctx, id)
}

// isUniqueViolation matches PostgreSQL SQLSTATE 23505.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
