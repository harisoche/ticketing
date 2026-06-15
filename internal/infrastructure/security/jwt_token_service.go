package security

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/service"
)

type jwtTokenService struct {
	secret []byte
	issuer string
}

// NewJWTTokenService builds a TokenService that signs and verifies HS256 JWTs.
func NewJWTTokenService(secret string, issuer string) service.TokenService {
	return &jwtTokenService{
		secret: []byte(secret),
		issuer: issuer,
	}
}

func (s *jwtTokenService) GenerateAccessToken(userID int64, sessionID uuid.UUID, expiresAt time.Time) (string, error) {
	now := time.Now().UTC()
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		ID:        sessionID.String(),
		Issuer:    s.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt.UTC()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", err
	}
	return signed, nil
}

func (s *jwtTokenService) ParseAccessToken(tokenString string) (*service.TokenClaims, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %s", t.Method.Alg())
		}
		return s.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid {
		return nil, domain.ErrUnauthorized
	}

	userID, parseErr := strconv.ParseInt(claims.Subject, 10, 64)
	if parseErr != nil {
		return nil, domain.ErrUnauthorized
	}
	sessionID, parseErr := uuid.Parse(claims.ID)
	if parseErr != nil {
		return nil, domain.ErrUnauthorized
	}
	if claims.ExpiresAt == nil {
		return nil, domain.ErrUnauthorized
	}

	return &service.TokenClaims{
		UserID:    userID,
		SessionID: sessionID,
		ExpiresAt: claims.ExpiresAt.Time,
	}, nil
}
