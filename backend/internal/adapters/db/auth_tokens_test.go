package db

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func newToken(userID uuid.UUID, kind entities.AuthTokenKind, secret string, expires time.Time) *entities.AuthToken {
	sum := sha256.Sum256([]byte(secret))
	return &entities.AuthToken{ID: uuid.New(), UserID: userID, Kind: kind, TokenHash: sum[:], ExpiresAt: expires, CreatedAt: time.Now()}
}

func hashOf(secret string) []byte { s := sha256.Sum256([]byte(secret)); return s[:] }

func TestAuthTokenRepository_ConsumeIsSingleUse(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	repo := NewAuthTokenRepository(pool)

	secret := "secret-" + uuid.NewString()
	if err := repo.Create(ctx, newToken(user.ID, entities.AuthTokenPasswordReset, secret, time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	got, ok, err := repo.Consume(ctx, entities.AuthTokenPasswordReset, hashOf(secret), time.Now())
	if err != nil || !ok || got != user.ID {
		t.Fatalf("first Consume = (%v, %v, %v), want the owner", got, ok, err)
	}
	if _, ok, _ := repo.Consume(ctx, entities.AuthTokenPasswordReset, hashOf(secret), time.Now()); ok {
		t.Error("a token must be usable only once")
	}
}

func TestAuthTokenRepository_ConsumeRejectsExpiredWrongKindAndUnknown(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	repo := NewAuthTokenRepository(pool)

	expired := "expired-" + uuid.NewString()
	repo.Create(ctx, newToken(user.ID, entities.AuthTokenPasswordReset, expired, time.Now().Add(-time.Minute)))
	if _, ok, _ := repo.Consume(ctx, entities.AuthTokenPasswordReset, hashOf(expired), time.Now()); ok {
		t.Error("an expired token was accepted")
	}

	verify := "verify-" + uuid.NewString()
	repo.Create(ctx, newToken(user.ID, entities.AuthTokenEmailVerification, verify, time.Now().Add(time.Hour)))
	if _, ok, _ := repo.Consume(ctx, entities.AuthTokenPasswordReset, hashOf(verify), time.Now()); ok {
		t.Error("a verification token must not work as a reset token")
	}
	if _, ok, _ := repo.Consume(ctx, entities.AuthTokenPasswordReset, hashOf("never-issued"), time.Now()); ok {
		t.Error("an unknown token was accepted")
	}
}

func TestAuthTokenRepository_InvalidateForUserOnlyTouchesThatKind(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	repo := NewAuthTokenRepository(pool)

	reset, verify := "r-"+uuid.NewString(), "v-"+uuid.NewString()
	repo.Create(ctx, newToken(user.ID, entities.AuthTokenPasswordReset, reset, time.Now().Add(time.Hour)))
	repo.Create(ctx, newToken(user.ID, entities.AuthTokenEmailVerification, verify, time.Now().Add(time.Hour)))

	if err := repo.InvalidateForUser(ctx, user.ID, entities.AuthTokenPasswordReset, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := repo.Consume(ctx, entities.AuthTokenPasswordReset, hashOf(reset), time.Now()); ok {
		t.Error("the reset token should have been invalidated")
	}
	if _, ok, _ := repo.Consume(ctx, entities.AuthTokenEmailVerification, hashOf(verify), time.Now()); !ok {
		t.Error("the verification token must be untouched")
	}
}

func TestAuthTokens_DeletedWithTheUser(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	repo := NewAuthTokenRepository(pool)
	secret := "gone-" + uuid.NewString()
	repo.Create(ctx, newToken(user.ID, entities.AuthTokenPasswordReset, secret, time.Now().Add(time.Hour)))

	if err := NewUserRepository(pool).Delete(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	pool.QueryRow(ctx, "SELECT count(*) FROM auth_tokens WHERE user_id = $1", user.ID).Scan(&n)
	if n != 0 {
		t.Errorf("%d tokens survived the user's deletion (RGPD erasure)", n)
	}
}

func TestUserRepository_BumpTokenVersionAndVerifiedSurviveUpdate(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewUserRepository(pool)
	user := createTestUser(t, pool)

	stale, _ := repo.GetByID(ctx, user.ID)
	if err := repo.BumpTokenVersion(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	// A flow that loaded the user BEFORE the bump and writes the row back
	// must not undo the revocation.
	now := time.Now()
	stale.EmailVerifiedAt = &now
	if _, err := repo.Update(ctx, stale); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByID(ctx, user.ID)
	if got.TokenVersion != 1 {
		t.Errorf("TokenVersion = %d, want 1: a stale Update un-revoked sessions", got.TokenVersion)
	}
	if !got.EmailVerified() {
		t.Error("EmailVerifiedAt should have been persisted by Update")
	}
}

func TestUserRepository_GetByEmailIsCaseInsensitive(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewUserRepository(pool)
	user := createTestUser(t, pool)

	got, err := repo.GetByEmail(ctx, "  "[:0]+toUpper(*user.Email))
	if err != nil || got == nil || got.ID != user.ID {
		t.Fatalf("GetByEmail(upper-case) = (%v, %v), want the user", got, err)
	}
}

func toUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
