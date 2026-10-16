// Package security holds the bcrypt password hasher and JWT session-cookie
// helpers — mirrors backend/app/adapters/security.py and
// backend/app/core/security.py.
package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const SessionCookieName = "budget_session"

type BcryptPasswordHasher struct{}

func (BcryptPasswordHasher) Hash(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (BcryptPasswordHasher) Verify(plain, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

type SessionTokens struct {
	Secret        string
	ExpireMinutes int
}

// Create signs a session token for the user. version is the user's current
// TokenVersion: Decode returns it so the caller can reject the token once
// the user has revoked their sessions (see UserRepository.BumpTokenVersion).
func (s SessionTokens) Create(userID uuid.UUID, version int) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"ver": version,
		"iat": now.Unix(),
		"exp": now.Add(time.Duration(s.ExpireMinutes) * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.Secret))
}

// Decode verifies the signature and expiry and returns the user id and the
// token version it was issued for. A token minted before versions existed has
// no "ver" claim and counts as version 0, which is what every user starts at.
func (s SessionTokens) Decode(tokenString string) (uuid.UUID, int, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.Secret), nil
	})
	if err != nil || !token.Valid {
		return uuid.UUID{}, 0, errors.New("invalid session token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.UUID{}, 0, errors.New("invalid session token")
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return uuid.UUID{}, 0, errors.New("invalid session token")
	}
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.UUID{}, 0, errors.New("invalid session token")
	}
	version := 0
	if v, ok := claims["ver"].(float64); ok {
		version = int(v)
	}
	return id, version, nil
}
