package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func newTestAuthUseCase(users *fakeUserRepo) *AuthUseCase {
	clock := fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return NewAuthUseCase(users, fakePasswordHasher{}, clock)
}

func TestAuthUseCase_Register(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	user, err := uc.Register(ctx, "new@example.com", "s3cret-password", "New User")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Email == nil || *user.Email != "new@example.com" {
		t.Fatalf("Email = %v, want new@example.com", user.Email)
	}
	if user.DisplayName != "New User" {
		t.Fatalf("DisplayName = %q, want %q", user.DisplayName, "New User")
	}
	if user.PasswordHash == nil || *user.PasswordHash != "hashed:s3cret-password" {
		t.Fatalf("PasswordHash = %v, want the hashed password", user.PasswordHash)
	}
}

func TestAuthUseCase_Register_DuplicateEmail(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	if _, err := uc.Register(ctx, "dup@example.com", "s3cret-password", "First"); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := uc.Register(ctx, "dup@example.com", "different-password", "Second")
	if !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("err = %v, want ErrEmailAlreadyRegistered", err)
	}
}

func TestAuthUseCase_AuthenticateWithPassword(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	if _, err := uc.Register(ctx, "me@example.com", "correct-password", "Me"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	user, err := uc.AuthenticateWithPassword(ctx, "me@example.com", "correct-password")
	if err != nil {
		t.Fatalf("AuthenticateWithPassword: %v", err)
	}
	if user.Email == nil || *user.Email != "me@example.com" {
		t.Fatalf("got user %v", user)
	}
}

func TestAuthUseCase_AuthenticateWithPassword_WrongPassword(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)
	if _, err := uc.Register(ctx, "me@example.com", "correct-password", "Me"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := uc.AuthenticateWithPassword(ctx, "me@example.com", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthUseCase_AuthenticateWithPassword_UnknownEmail(t *testing.T) {
	ctx := context.Background()
	uc := newTestAuthUseCase(newFakeUserRepo())

	if _, err := uc.AuthenticateWithPassword(ctx, "nobody@example.com", "anything"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

// A user who signed up via Google/Robine only (no password set) must not
// be authenticatable by password at all, even with an empty/blank guess -
// PasswordHash is nil, not an easily-forged hash of "".
func TestAuthUseCase_AuthenticateWithPassword_NoPasswordSet(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	email := "oauth-only@example.com"
	id := uuid.New()
	users.byID[id] = &entities.User{ID: id, Email: &email, DisplayName: "OAuth Only"}
	uc := newTestAuthUseCase(users)

	if _, err := uc.AuthenticateWithPassword(ctx, email, ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthUseCase_AuthenticateOrRegisterWithGoogle_CreatesNewUser(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	user, err := uc.AuthenticateOrRegisterWithGoogle(ctx, "google-sub-1", "person@example.com", true, "Person")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithGoogle: %v", err)
	}
	if user.GoogleSub == nil || *user.GoogleSub != "google-sub-1" {
		t.Fatalf("GoogleSub = %v, want google-sub-1", user.GoogleSub)
	}
	if user.DisplayName != "Person" {
		t.Fatalf("DisplayName = %q, want Person", user.DisplayName)
	}
}

func TestAuthUseCase_AuthenticateOrRegisterWithGoogle_ReturnsExistingBySub(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	first, err := uc.AuthenticateOrRegisterWithGoogle(ctx, "google-sub-1", "person@example.com", true, "Person")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := uc.AuthenticateOrRegisterWithGoogle(ctx, "google-sub-1", "person@example.com", true, "Person")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second call created a new user (%s) instead of reusing the first (%s)", second.ID, first.ID)
	}
}

func TestAuthUseCase_AuthenticateOrRegisterWithApple_CreatesThenReuses(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	first, err := uc.AuthenticateOrRegisterWithApple(ctx, "apple-sub-1", "person@privaterelay.appleid.com", true, "Person")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithApple: %v", err)
	}
	if first.AppleSub == nil || *first.AppleSub != "apple-sub-1" {
		t.Fatalf("AppleSub = %v, want apple-sub-1", first.AppleSub)
	}

	// Apple only sends the name on the first sign-in; later ones must find
	// the same user anyway, from the subject alone.
	second, err := uc.AuthenticateOrRegisterWithApple(ctx, "apple-sub-1", "", true, "")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if second.ID != first.ID || second.DisplayName != "Person" {
		t.Fatalf("second call = %+v, want the first user back", second)
	}
}

// Same as Google: signing in with Apple on an address that already has an
// account links the two instead of creating a duplicate.
func TestAuthUseCase_AuthenticateOrRegisterWithApple_LinksExistingEmailAccount(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	existing, err := uc.Register(ctx, "person@example.com", "password123", "Person")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	linked, err := uc.AuthenticateOrRegisterWithApple(ctx, "apple-sub-1", "person@example.com", true, "Person")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithApple: %v", err)
	}
	if linked.ID != existing.ID {
		t.Fatalf("created a new user (%s) instead of linking %s", linked.ID, existing.ID)
	}
	if linked.AppleSub == nil || *linked.AppleSub != "apple-sub-1" {
		t.Fatalf("AppleSub = %v, want apple-sub-1", linked.AppleSub)
	}
}

// Signing in with Google using an email that already has a password
// account should link the two (set GoogleSub on the existing user) rather
// than creating a duplicate account for the same person.
func TestAuthUseCase_AuthenticateOrRegisterWithGoogle_LinksExistingEmailAccount(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	existing, err := uc.Register(ctx, "shared@example.com", "s3cret-password", "Existing")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	linked, err := uc.AuthenticateOrRegisterWithGoogle(ctx, "google-sub-2", "shared@example.com", true, "")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithGoogle: %v", err)
	}
	if linked.ID != existing.ID {
		t.Fatalf("expected the existing password account to be reused, got a new user %s", linked.ID)
	}
	if linked.GoogleSub == nil || *linked.GoogleSub != "google-sub-2" {
		t.Fatalf("GoogleSub = %v, want google-sub-2", linked.GoogleSub)
	}
	// The existing display name is kept, not overwritten by an empty one
	// from the Google profile.
	if linked.DisplayName != "Existing" {
		t.Fatalf("DisplayName = %q, want the original %q to survive", linked.DisplayName, "Existing")
	}
}

func TestAuthUseCase_AuthenticateOrRegisterWithGoogle_NoEmailFallsBackToGenericName(t *testing.T) {
	ctx := context.Background()
	uc := newTestAuthUseCase(newFakeUserRepo())

	user, err := uc.AuthenticateOrRegisterWithGoogle(ctx, "google-sub-3", "", true, "")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithGoogle: %v", err)
	}
	if user.Email != nil {
		t.Fatalf("Email = %v, want nil", user.Email)
	}
	if user.DisplayName != "Google User" {
		t.Fatalf("DisplayName = %q, want the generic fallback", user.DisplayName)
	}
}

func TestAuthUseCase_AuthenticateOrRegisterWithRobine_CreatesNewUser(t *testing.T) {
	ctx := context.Background()
	uc := newTestAuthUseCase(newFakeUserRepo())

	user, err := uc.AuthenticateOrRegisterWithRobine(ctx, "robine-sub-1", "person@example.com", true, "Person")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithRobine: %v", err)
	}
	if user.RobineSub == nil || *user.RobineSub != "robine-sub-1" {
		t.Fatalf("RobineSub = %v, want robine-sub-1", user.RobineSub)
	}
}

func TestAuthUseCase_AuthenticateOrRegisterWithRobine_LinksExistingEmailAccount(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	existing, err := uc.Register(ctx, "shared@example.com", "s3cret-password", "Existing")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	linked, err := uc.AuthenticateOrRegisterWithRobine(ctx, "robine-sub-2", "shared@example.com", true, "")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithRobine: %v", err)
	}
	if linked.ID != existing.ID {
		t.Fatalf("expected the existing password account to be reused, got a new user %s", linked.ID)
	}
	if linked.RobineSub == nil || *linked.RobineSub != "robine-sub-2" {
		t.Fatalf("RobineSub = %v, want robine-sub-2", linked.RobineSub)
	}
}

func TestAuthUseCase_AuthenticateOrRegisterWithRobine_NoEmailFallsBackToGenericName(t *testing.T) {
	ctx := context.Background()
	uc := newTestAuthUseCase(newFakeUserRepo())

	user, err := uc.AuthenticateOrRegisterWithRobine(ctx, "robine-sub-3", "", true, "")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithRobine: %v", err)
	}
	if user.DisplayName != "Robine ID User" {
		t.Fatalf("DisplayName = %q, want the generic fallback", user.DisplayName)
	}
}

// Google and Robine subs are independent identities on the same account -
// linking one must never clobber the other, and a lookup by one provider's
// sub should never match a user linked only via the other.
func TestAuthUseCase_GoogleAndRobineAreIndependentLinks(t *testing.T) {
	ctx := context.Background()
	users := newFakeUserRepo()
	uc := newTestAuthUseCase(users)

	viaGoogle, err := uc.AuthenticateOrRegisterWithGoogle(ctx, "google-sub-4", "both@example.com", true, "Both")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithGoogle: %v", err)
	}
	viaRobine, err := uc.AuthenticateOrRegisterWithRobine(ctx, "robine-sub-4", "both@example.com", true, "")
	if err != nil {
		t.Fatalf("AuthenticateOrRegisterWithRobine: %v", err)
	}
	if viaRobine.ID != viaGoogle.ID {
		t.Fatalf("expected the same account (matched by email) to gain a Robine link too, got a different user")
	}
	if viaRobine.GoogleSub == nil || *viaRobine.GoogleSub != "google-sub-4" {
		t.Fatalf("GoogleSub was lost after linking Robine: %v", viaRobine.GoogleSub)
	}
	if viaRobine.RobineSub == nil || *viaRobine.RobineSub != "robine-sub-4" {
		t.Fatalf("RobineSub = %v, want robine-sub-4", viaRobine.RobineSub)
	}
}
