package security

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"ticketing-api/internal/domain"
)

const testSecret = "super-secret-test-key-with-at-least-32-chars"
const testIssuer = "ticketing-api"

func TestJWT_GenerateAndParse(t *testing.T) {
	svc := NewJWTTokenService(testSecret, testIssuer)
	sessionID := uuid.New()
	exp := time.Now().Add(time.Hour)

	token, err := svc.GenerateAccessToken(42, sessionID, exp)
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	claims, err := svc.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("ParseAccessToken failed: %v", err)
	}

	if claims.UserID != 42 {
		t.Errorf("UserID: want 42, got %d", claims.UserID)
	}
	if claims.SessionID != sessionID {
		t.Errorf("SessionID: want %s, got %s", sessionID, claims.SessionID)
	}
	if !claims.ExpiresAt.Equal(exp.UTC().Truncate(time.Second)) {
		// jwt encodes seconds precision
		if claims.ExpiresAt.Unix() != exp.Unix() {
			t.Errorf("ExpiresAt: want %v, got %v", exp.Unix(), claims.ExpiresAt.Unix())
		}
	}
}

func TestJWT_RejectsExpired(t *testing.T) {
	svc := NewJWTTokenService(testSecret, testIssuer)
	token, err := svc.GenerateAccessToken(1, uuid.New(), time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}
	_, err = svc.ParseAccessToken(token)
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for expired token, got %v", err)
	}
}

func TestJWT_RejectsWrongIssuer(t *testing.T) {
	signer := NewJWTTokenService(testSecret, "other-issuer")
	parser := NewJWTTokenService(testSecret, testIssuer)
	token, err := signer.GenerateAccessToken(1, uuid.New(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}
	_, err = parser.ParseAccessToken(token)
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for wrong issuer, got %v", err)
	}
}

func TestJWT_RejectsUnexpectedAlgorithm(t *testing.T) {
	// Hand-craft an HS512-signed token with the right secret and verify the
	// HS256-only parser refuses it.
	sessionID := uuid.New()
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(7, 10),
		ID:        sessionID.String(),
		Issuer:    testIssuer,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	signed, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign HS512 token: %v", err)
	}

	svc := NewJWTTokenService(testSecret, testIssuer)
	_, err = svc.ParseAccessToken(signed)
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for HS512 token, got %v", err)
	}
}

func TestJWT_RejectsWrongSecret(t *testing.T) {
	signer := NewJWTTokenService("another-secret-that-is-also-32-chars-long", testIssuer)
	parser := NewJWTTokenService(testSecret, testIssuer)
	token, err := signer.GenerateAccessToken(1, uuid.New(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}
	_, err = parser.ParseAccessToken(token)
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for wrong secret, got %v", err)
	}
}
