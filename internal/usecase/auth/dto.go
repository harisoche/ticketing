package auth

import "time"

// RegisterInput captures the data needed to create a new account.
type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

// LoginInput captures the data needed to authenticate.
type LoginInput struct {
	Email    string
	Password string
}

// PublicUser is the non-sensitive view of a user returned to the API.
type PublicUser struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuthResult is returned after a successful register or login.
type AuthResult struct {
	AccessToken      string     `json:"access_token"`
	TokenType        string     `json:"token_type"`
	ExpiresInSeconds int64      `json:"expires_in"`
	User             PublicUser `json:"user"`
}
