// Use-case: password, Google, Apple and Robine ID (self-hosted OIDC)
// registration and login, plus the emailed flows around a password account
// (address verification, password reset) and session revocation.
package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
)

var ErrEmailAlreadyRegistered = errors.New("email already registered")
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrSignupClosed is returned when a new account may not be created (private
// beta). Signing in to an existing account is never affected.
var ErrSignupClosed = errors.New("registration is closed")

// ErrInvalidToken is returned for an emailed token that is unknown, already
// used or expired — deliberately one error, so a caller can't tell which.
var ErrInvalidToken = errors.New("invalid or expired token")

// Password policy. The floor follows NIST 800-63B (length over composition
// rules); the ceiling is bcrypt's own limit — it silently ignores everything
// past 72 bytes, so a longer password would be a lie.
const (
	MinPasswordLength = 10
	MaxPasswordBytes  = 72
)

const (
	emailVerificationTTL = 48 * time.Hour
	passwordResetTTL     = time.Hour
)

// ValidationError says why user input was refused; its message is safe to
// show to the user.
type ValidationError struct{ Reason string }

func (e *ValidationError) Error() string { return e.Reason }

// ValidatePassword enforces the password policy. email may be empty.
func ValidatePassword(password, email string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return &ValidationError{fmt.Sprintf("password must be at least %d characters", MinPasswordLength)}
	}
	if len(password) > MaxPasswordBytes {
		return &ValidationError{fmt.Sprintf("password must be at most %d bytes", MaxPasswordBytes)}
	}
	if email != "" && strings.EqualFold(strings.TrimSpace(password), strings.TrimSpace(email)) {
		return &ValidationError{"password must not be your email address"}
	}
	return nil
}

// EmailFlows wires what verification and reset emails need. Without it the
// AuthUseCase still registers and authenticates; it just can't send mail.
type EmailFlows struct {
	Tokens ports.AuthTokenRepository
	Mailer ports.Mailer
	// PublicURL is the origin the links in the emails point to.
	PublicURL string
}

type AuthUseCase struct {
	users          ports.UserRepository
	passwordHasher ports.PasswordHasher
	clock          ports.Clock
	email          *EmailFlows
	// signupAllowed decides whether an address may create an account. Nil means
	// open; the composition root always sets it from configuration.
	signupAllowed func(email string) bool

	// dummyHash is verified against when the email is unknown, so that
	// "no such user" costs the same bcrypt run as "wrong password" and the
	// response time doesn't reveal which addresses have an account.
	dummyOnce sync.Once
	dummyHash string
}

func NewAuthUseCase(users ports.UserRepository, passwordHasher ports.PasswordHasher, clock ports.Clock) *AuthUseCase {
	return &AuthUseCase{users: users, passwordHasher: passwordHasher, clock: clock}
}

// WithEmailFlows enables verification and password-reset emails.
func (uc *AuthUseCase) WithEmailFlows(flows EmailFlows) *AuthUseCase {
	uc.email = &flows
	return uc
}

// WithSignupPolicy restricts who can create an account (see ErrSignupClosed).
// The function gets the address, possibly empty (Apple may hide it).
func (uc *AuthUseCase) WithSignupPolicy(allowed func(email string) bool) *AuthUseCase {
	uc.signupAllowed = allowed
	return uc
}

func (uc *AuthUseCase) mayRegister(email string) bool {
	return uc.signupAllowed == nil || uc.signupAllowed(email)
}

// DeleteUser permanently deletes a user and everything they own — the
// in-app "Supprimer mon compte" the App Store requires of any app that lets
// people create an account. See UserRepository.Delete for exactly what goes.
func (uc *AuthUseCase) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	return uc.users.Delete(ctx, userID)
}

// LogoutAll revokes every session token issued so far for the user.
func (uc *AuthUseCase) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	return uc.users.BumpTokenVersion(ctx, userID)
}

func (uc *AuthUseCase) Register(ctx context.Context, email, password, displayName string) (*entities.User, error) {
	email = strings.TrimSpace(email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return nil, &ValidationError{"invalid email address"}
	}
	if err := ValidatePassword(password, email); err != nil {
		return nil, err
	}
	// Checked before the "already registered" lookup so that, while closed, the
	// answer doesn't reveal which addresses have an account.
	if !uc.mayRegister(email) {
		return nil, ErrSignupClosed
	}
	// New addresses are stored lowercase; lookups are case-insensitive (see
	// UserRepository.GetByEmail).
	email = strings.ToLower(email)

	existing, err := uc.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailAlreadyRegistered
	}
	hash, err := uc.passwordHasher.Hash(password)
	if err != nil {
		return nil, err
	}
	user := &entities.User{
		ID:           uuid.New(),
		Email:        &email,
		PasswordHash: &hash,
		DisplayName:  displayName,
		CreatedAt:    uc.clock.Now(),
	}
	user, err = uc.users.Add(ctx, user)
	if err != nil {
		return nil, err
	}
	if err := uc.sendVerification(ctx, user); err != nil && !errors.Is(err, errEmailFlowsDisabled) {
		// The account exists and works; the user can ask for another email.
		log.Printf("auth: verification email for %s not sent: %v", user.ID, err)
	}
	return user, nil
}

func (uc *AuthUseCase) AuthenticateWithPassword(ctx context.Context, email, password string) (*entities.User, error) {
	user, err := uc.users.GetByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		return nil, err
	}
	if user == nil || user.PasswordHash == nil {
		uc.passwordHasher.Verify(password, uc.dummyPasswordHash())
		return nil, ErrInvalidCredentials
	}
	if !uc.passwordHasher.Verify(password, *user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

func (uc *AuthUseCase) dummyPasswordHash() string {
	uc.dummyOnce.Do(func() {
		uc.dummyHash, _ = uc.passwordHasher.Hash(uuid.NewString())
	})
	return uc.dummyHash
}

// --- Emailed flows -----------------------------------------------------------

func newAuthToken() (token string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashAuthToken(token), nil
}

func hashAuthToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

var errEmailFlowsDisabled = errors.New("email is not configured")

// issueToken stores a fresh single-use token for the user and returns the
// clear value to put in the email link. Older outstanding tokens of the same
// kind stop working.
func (uc *AuthUseCase) issueToken(ctx context.Context, userID uuid.UUID, kind entities.AuthTokenKind, ttl time.Duration) (string, error) {
	if uc.email == nil || uc.email.Tokens == nil || uc.email.Mailer == nil || uc.email.PublicURL == "" {
		return "", errEmailFlowsDisabled
	}
	now := uc.clock.Now()
	if err := uc.email.Tokens.InvalidateForUser(ctx, userID, kind, now); err != nil {
		return "", err
	}
	token, hash, err := newAuthToken()
	if err != nil {
		return "", err
	}
	if err := uc.email.Tokens.Create(ctx, &entities.AuthToken{
		ID: uuid.New(), UserID: userID, Kind: kind, TokenHash: hash, ExpiresAt: now.Add(ttl), CreatedAt: now,
	}); err != nil {
		return "", err
	}
	return token, nil
}

func (uc *AuthUseCase) link(path, token string) string {
	return uc.email.PublicURL + path + "?token=" + url.QueryEscape(token)
}

func (uc *AuthUseCase) sendVerification(ctx context.Context, user *entities.User) error {
	if user.Email == nil || user.EmailVerified() {
		return nil
	}
	token, err := uc.issueToken(ctx, user.ID, entities.AuthTokenEmailVerification, emailVerificationTTL)
	if err != nil {
		return err
	}
	body := "Bonjour,\n\nPour confirmer votre adresse email et activer votre compte Lagune, ouvrez ce lien (valable 48 heures) :\n\n" +
		uc.link("/verify-email", token) +
		"\n\nSi vous n'êtes pas à l'origine de cette inscription, ignorez simplement ce message.\n"
	return uc.email.Mailer.Send(ctx, *user.Email, "Confirmez votre adresse email — Lagune", body)
}

// ResendVerification sends a new verification link to a signed-in user whose
// address isn't verified yet. A no-op for a verified one.
func (uc *AuthUseCase) ResendVerification(ctx context.Context, user *entities.User) error {
	return uc.sendVerification(ctx, user)
}

// VerifyEmail consumes an emailed verification token.
func (uc *AuthUseCase) VerifyEmail(ctx context.Context, token string) error {
	if uc.email == nil {
		return ErrInvalidToken
	}
	now := uc.clock.Now()
	userID, ok, err := uc.email.Tokens.Consume(ctx, entities.AuthTokenEmailVerification, hashAuthToken(token), now)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidToken
	}
	user, err := uc.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrInvalidToken
	}
	if !user.EmailVerified() {
		user.EmailVerifiedAt = &now
		if _, err := uc.users.Update(ctx, user); err != nil {
			return err
		}
	}
	return nil
}

// RequestPasswordReset emails a reset link if the address belongs to an
// account. It reports nothing about whether it does: the caller must answer
// the same way either way, or the endpoint becomes an account-existence
// oracle. A returned error is an internal failure to log, never to show.
func (uc *AuthUseCase) RequestPasswordReset(ctx context.Context, email string) error {
	user, err := uc.users.GetByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		return err
	}
	if user == nil || user.Email == nil {
		return nil
	}
	token, err := uc.issueToken(ctx, user.ID, entities.AuthTokenPasswordReset, passwordResetTTL)
	if err != nil {
		return err
	}
	body := "Bonjour,\n\nVous avez demandé à réinitialiser votre mot de passe Lagune. Ouvrez ce lien (valable 1 heure) :\n\n" +
		uc.link("/reset-password", token) +
		"\n\nSi vous n'êtes pas à l'origine de cette demande, ignorez ce message : votre mot de passe reste inchangé.\n"
	return uc.email.Mailer.Send(ctx, *user.Email, "Réinitialisation de votre mot de passe — Lagune", body)
}

// ResetPassword sets a new password from an emailed token. Every existing
// session is revoked, and the address counts as verified (the person just
// proved they read that inbox).
func (uc *AuthUseCase) ResetPassword(ctx context.Context, token, newPassword string) (*entities.User, error) {
	if err := ValidatePassword(newPassword, ""); err != nil {
		return nil, err
	}
	if uc.email == nil {
		return nil, ErrInvalidToken
	}
	now := uc.clock.Now()
	userID, ok, err := uc.email.Tokens.Consume(ctx, entities.AuthTokenPasswordReset, hashAuthToken(token), now)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidToken
	}
	user, err := uc.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidToken
	}
	hash, err := uc.passwordHasher.Hash(newPassword)
	if err != nil {
		return nil, err
	}
	user.PasswordHash = &hash
	if !user.EmailVerified() {
		user.EmailVerifiedAt = &now
	}
	// Bump before Update so the row Update returns already carries the new
	// version.
	if err := uc.users.BumpTokenVersion(ctx, user.ID); err != nil {
		return nil, err
	}
	updated, err := uc.users.Update(ctx, user)
	if err != nil {
		return nil, err
	}
	if err := uc.email.Tokens.InvalidateForUser(ctx, user.ID, entities.AuthTokenPasswordReset, now); err != nil {
		log.Printf("auth: invalidating reset tokens for %s: %v", user.ID, err)
	}
	return updated, nil
}

// --- Social sign-in -----------------------------------------------------------

// linkProviderToExisting attaches a provider identity to an account that
// already has this (provider-verified) email. If that account's own address
// was never verified, whoever registered it may not own the address — they
// could be waiting for the real owner to sign in with Google to inherit that
// session — so its password is dropped and its sessions revoked before the
// link is made. The real owner can set a new password by email reset.
func (uc *AuthUseCase) linkProviderToExisting(ctx context.Context, existing *entities.User, displayName string, attach func(*entities.User)) (*entities.User, error) {
	attach(existing)
	if existing.DisplayName == "" && displayName != "" {
		existing.DisplayName = displayName
	}
	if !existing.EmailVerified() {
		existing.PasswordHash = nil
		now := uc.clock.Now()
		existing.EmailVerifiedAt = &now
		if err := uc.users.BumpTokenVersion(ctx, existing.ID); err != nil {
			return nil, err
		}
	}
	return uc.users.Update(ctx, existing)
}

// signInWithProvider is the shared body of the three provider flows:
// existing link by subject, then link by (verified) email, then a new user.
// emailVerified says whether the provider vouches for email; an address it
// doesn't vouch for is ignored entirely rather than trusted to identify or
// link anyone.
func (uc *AuthUseCase) signInWithProvider(
	ctx context.Context,
	findBySub func() (*entities.User, error),
	email string, emailVerified bool, displayName, fallbackName string,
	attach func(*entities.User),
) (*entities.User, error) {
	user, err := findBySub()
	if err != nil {
		return nil, err
	}
	if user != nil {
		return user, nil
	}
	if !emailVerified {
		email = ""
	}

	if email != "" {
		existing, err := uc.users.GetByEmail(ctx, email)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return uc.linkProviderToExisting(ctx, existing, displayName, attach)
		}
	}

	// Only a brand-new account is gated: linking to an existing one (above) and
	// signing in by subject (further up) are unaffected.
	if !uc.mayRegister(email) {
		return nil, ErrSignupClosed
	}

	var emailPtr *string
	var verifiedAt *time.Time
	if email != "" {
		emailPtr = &email
		now := uc.clock.Now()
		verifiedAt = &now
	}
	if displayName == "" {
		if email != "" {
			displayName = email
		} else {
			displayName = fallbackName
		}
	}
	newUser := &entities.User{
		ID:              uuid.New(),
		Email:           emailPtr,
		DisplayName:     displayName,
		CreatedAt:       uc.clock.Now(),
		EmailVerifiedAt: verifiedAt,
	}
	attach(newUser)
	return uc.users.Add(ctx, newUser)
}

func (uc *AuthUseCase) AuthenticateOrRegisterWithGoogle(ctx context.Context, sub, email string, emailVerified bool, displayName string) (*entities.User, error) {
	return uc.signInWithProvider(ctx,
		func() (*entities.User, error) { return uc.users.GetByGoogleSub(ctx, sub) },
		email, emailVerified, displayName, "Google User",
		func(u *entities.User) { u.GoogleSub = &sub })
}

// AuthenticateOrRegisterWithApple mirrors AuthenticateOrRegisterWithGoogle
// against the apple_sub column. Apple only sends the user's name on their
// very first sign-in, and hides the address behind a private relay when
// asked to, so both are best-effort here.
func (uc *AuthUseCase) AuthenticateOrRegisterWithApple(ctx context.Context, sub, email string, emailVerified bool, displayName string) (*entities.User, error) {
	return uc.signInWithProvider(ctx,
		func() (*entities.User, error) { return uc.users.GetByAppleSub(ctx, sub) },
		email, emailVerified, displayName, "Apple User",
		func(u *entities.User) { u.AppleSub = &sub })
}

// AuthenticateOrRegisterWithRobine mirrors AuthenticateOrRegisterWithGoogle
// exactly, against the robine_sub column instead — same three-way lookup
// (existing link, then email match to link a password/Google account to
// Robine too, then a brand new user).
func (uc *AuthUseCase) AuthenticateOrRegisterWithRobine(ctx context.Context, sub, email string, emailVerified bool, displayName string) (*entities.User, error) {
	return uc.signInWithProvider(ctx,
		func() (*entities.User, error) { return uc.users.GetByRobineSub(ctx, sub) },
		email, emailVerified, displayName, "Robine ID User",
		func(u *entities.User) { u.RobineSub = &sub })
}
