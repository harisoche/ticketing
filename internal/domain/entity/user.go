package entity

import "time"

// User is the framework-independent domain representation of an application
// user. The password hash is part of the entity but must never be returned
// from API responses.
type User struct {
	ID           int64
	Name         string
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// User roles. Persisted as VARCHAR in the users.role column; new users default
// to RoleCustomer.
const (
	RoleCustomer = "customer"
	RoleAgent    = "agent"
	RoleAdmin    = "admin"
)

// IsValidRole reports whether r is one of the supported roles.
func IsValidRole(r string) bool {
	switch r {
	case RoleCustomer, RoleAgent, RoleAdmin:
		return true
	}
	return false
}
