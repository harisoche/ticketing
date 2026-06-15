package model

import (
	"time"

	"ticketing-api/internal/domain/entity"
)

// UserModel is the GORM-bound representation of a row in the `users` table.
type UserModel struct {
	ID           int64     `gorm:"primaryKey;column:id"`
	Name         string    `gorm:"column:name;size:100;not null"`
	Email        string    `gorm:"column:email;size:255;not null"`
	PasswordHash string    `gorm:"column:password_hash;size:255;not null"`
	Role         string    `gorm:"column:role;size:30;not null;default:customer"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (UserModel) TableName() string { return "users" }

func UserModelFromEntity(u *entity.User) *UserModel {
	role := u.Role
	if role == "" {
		role = entity.RoleCustomer
	}
	return &UserModel{
		ID:           u.ID,
		Name:         u.Name,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Role:         role,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

func (m *UserModel) ToEntity() *entity.User {
	return &entity.User{
		ID:           m.ID,
		Name:         m.Name,
		Email:        m.Email,
		PasswordHash: m.PasswordHash,
		Role:         m.Role,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}
