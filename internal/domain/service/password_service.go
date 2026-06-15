package service

// PasswordService hashes and compares passwords.
// Implementations live in infrastructure/security.
type PasswordService interface {
	Hash(password string) (string, error)
	Compare(hash string, password string) error
}
