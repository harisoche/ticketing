package security

import (
	"errors"
	"testing"

	"ticketing-api/internal/domain"
)

func TestBcryptPasswordService_HashAndCompare(t *testing.T) {
	svc := NewBcryptPasswordService(4)

	hash, err := svc.Hash("password123")
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}
	if hash == "" || hash == "password123" {
		t.Fatalf("hash output is invalid: %q", hash)
	}

	if err := svc.Compare(hash, "password123"); err != nil {
		t.Fatalf("expected matching password to verify, got %v", err)
	}
}

func TestBcryptPasswordService_RejectsWrongPassword(t *testing.T) {
	svc := NewBcryptPasswordService(4)

	hash, err := svc.Hash("password123")
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	err = svc.Compare(hash, "wrong-password")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}
