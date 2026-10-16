package services

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

// --- fakes local to these tests ------------------------------------------------

type steppingClock struct{ now time.Time }

func (c *steppingClock) Now() time.Time    { return c.now }
func (c *steppingClock) Today() civil.Date { return civil.Date{} }

type fakeAuthTokenRepo struct {
	mu     sync.Mutex
	tokens []*fakeStoredToken
}

type fakeStoredToken struct {
	entities.AuthToken
	used bool
}

func (f *fakeAuthTokenRepo) Create(_ context.Context, t *entities.AuthToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens = append(f.tokens, &fakeStoredToken{AuthToken: *t})
	return nil
}

func (f *fakeAuthTokenRepo) Consume(_ context.Context, kind entities.AuthTokenKind, hash []byte, now time.Time) (uuid.UUID, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.Kind == kind && string(t.TokenHash) == string(hash) && !t.used && t.ExpiresAt.After(now) {
			t.used = true
			return t.UserID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

func (f *fakeAuthTokenRepo) InvalidateForUser(_ context.Context, userID uuid.UUID, kind entities.AuthTokenKind, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.UserID == userID && t.Kind == kind {
			t.used = true
		}
	}
	return nil
}

type sentMail struct{ to, subject, body string }

type fakeMailer struct{ sent []sentMail }

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	m.sent = append(m.sent, sentMail{to, subject, body})
	return nil
}

// tokenFromMail pulls the token out of the single link in a mail body.
func tokenFromMail(t *testing.T, m sentMail) string {
	t.Helper()
	i := strings.Index(m.body, "https://")
	if i < 0 {
		t.Fatalf("no link in mail body: %q", m.body)
	}
	link := strings.Fields(m.body[i:])[0]
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("bad link %q: %v", link, err)
	}
	token := u.Query().Get("token")
	if token == "" {
		t.Fatalf("no token in link %q", link)
	}
	return token
}

type authHarness struct {
	uc     *AuthUseCase
	users  *fakeUserRepo
	tokens *fakeAuthTokenRepo
	mailer *fakeMailer
	clock  *steppingClock
}

func newAuthHarness() authHarness {
	users := newFakeUserRepo()
	clock := &steppingClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	tokens := &fakeAuthTokenRepo{}
	mailer := &fakeMailer{}
	uc := NewAuthUseCase(users, fakePasswordHasher{}, clock).WithEmailFlows(EmailFlows{
		Tokens: tokens, Mailer: mailer, PublicURL: "https://lagune.test",
	})
	return authHarness{uc, users, tokens, mailer, clock}
}

// --- password policy -----------------------------------------------------------

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name, password, email string
		wantErr               bool
	}{
		{"long enough", "correct horse", "", false},
		{"too short", "short", "", true},
		{"nine characters", "123456789", "", true},
		{"ten characters", "1234567890", "", false},
		{"ten multi-byte characters count as ten", "éééééééééé", "", false},
		{"over bcrypt's 72 bytes", strings.Repeat("a", 73), "", true},
		{"exactly 72 bytes", strings.Repeat("a", 72), "", false},
		{"same as the email", "person@example.com", "Person@Example.com", true},
	}
	for _, c := range cases {
		err := ValidatePassword(c.password, c.email)
		var v *ValidationError
		if c.wantErr && !errors.As(err, &v) {
			t.Errorf("%s: err = %v, want a *ValidationError", c.name, err)
		}
		if !c.wantErr && err != nil {
			t.Errorf("%s: err = %v, want nil", c.name, err)
		}
	}
}

func TestRegister_RejectsBadInput(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	for _, email := range []string{"", "not-an-email", "Name <a@example.com>", "a@example.com\r\nBcc: x@y.z"} {
		if _, err := h.uc.Register(ctx, email, "long-enough-password", "X"); err == nil {
			t.Errorf("Register(%q) succeeded, want a validation error", email)
		}
	}
	if _, err := h.uc.Register(ctx, "a@example.com", "short", "X"); err == nil {
		t.Error("a short password must be refused")
	}
}

func TestRegister_LowercasesAndLoginIsCaseInsensitive(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	user, err := h.uc.Register(ctx, "Person@Example.COM", "long-enough-password", "P")
	if err != nil {
		t.Fatal(err)
	}
	if *user.Email != "person@example.com" {
		t.Errorf("stored email = %q, want lowercase", *user.Email)
	}
	if _, err := h.uc.AuthenticateWithPassword(ctx, "PERSON@example.com", "long-enough-password"); err != nil {
		t.Errorf("login with a differently-cased address failed: %v", err)
	}
	if _, err := h.uc.Register(ctx, "person@EXAMPLE.com", "long-enough-password", "P2"); !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Errorf("re-registering in another case: err = %v, want ErrEmailAlreadyRegistered", err)
	}
}

// --- constant-time login ---------------------------------------------------------

type countingHasher struct {
	fakePasswordHasher
	verifies int
}

func (h *countingHasher) Verify(plain, hash string) bool {
	h.verifies++
	return h.fakePasswordHasher.Verify(plain, hash)
}

func TestAuthenticate_UnknownEmailStillRunsAHashComparison(t *testing.T) {
	hasher := &countingHasher{}
	uc := NewAuthUseCase(newFakeUserRepo(), hasher, &steppingClock{now: time.Now()})
	if _, err := uc.AuthenticateWithPassword(context.Background(), "nobody@example.com", "whatever-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if hasher.verifies != 1 {
		t.Errorf("Verify ran %d times for an unknown email, want 1 (same cost as a wrong password)", hasher.verifies)
	}
}

// --- pre-hijacking protection ------------------------------------------------------

func TestProviderLink_UnverifiedPasswordAccountLosesItsPassword(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	// An attacker registers the victim's address first...
	planted, err := h.uc.Register(ctx, "victim@example.com", "attacker-known-password", "Attacker")
	if err != nil {
		t.Fatal(err)
	}
	if planted.EmailVerified() {
		t.Fatal("a fresh password account must start unverified")
	}
	versionBefore := planted.TokenVersion

	// ...then the real owner signs in with Google.
	linked, err := h.uc.AuthenticateOrRegisterWithGoogle(ctx, "google-victim", "victim@example.com", true, "Victim")
	if err != nil {
		t.Fatal(err)
	}
	if linked.ID != planted.ID {
		t.Fatal("expected the existing account to be linked")
	}
	if linked.PasswordHash != nil {
		t.Error("the attacker's password must be wiped when an unverified account is linked")
	}
	if linked.TokenVersion == versionBefore {
		t.Error("the attacker's sessions must be revoked (token version unchanged)")
	}
	if !linked.EmailVerified() {
		t.Error("the provider-verified address should now count as verified")
	}
	if _, err := h.uc.AuthenticateWithPassword(ctx, "victim@example.com", "attacker-known-password"); err == nil {
		t.Error("the attacker can still log in with the planted password")
	}
}

func TestProviderLink_VerifiedPasswordAccountKeepsItsPassword(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	user, _ := h.uc.Register(ctx, "owner@example.com", "owner-password-123", "Owner")
	now := h.clock.now
	user.EmailVerifiedAt = &now
	versionBefore := user.TokenVersion

	linked, err := h.uc.AuthenticateOrRegisterWithGoogle(ctx, "google-owner", "owner@example.com", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if linked.PasswordHash == nil || linked.TokenVersion != versionBefore {
		t.Error("a verified account must keep its password and sessions when a provider is linked")
	}
}

func TestProviderSignIn_UnverifiedProviderEmailIsIgnored(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	existing, _ := h.uc.Register(ctx, "someone@example.com", "some-long-password", "Someone")

	got, err := h.uc.AuthenticateOrRegisterWithRobine(ctx, "robine-x", "someone@example.com", false, "Mallory")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == existing.ID {
		t.Fatal("an unverified provider email must never link to an existing account")
	}
	if got.Email != nil {
		t.Errorf("Email = %v, want nil: an unverified address must not be stored", *got.Email)
	}
}

func TestProviderSignIn_NewUserWithVerifiedEmailIsVerified(t *testing.T) {
	h := newAuthHarness()
	user, err := h.uc.AuthenticateOrRegisterWithApple(context.Background(), "apple-1", "a@privaterelay.appleid.com", true, "A")
	if err != nil {
		t.Fatal(err)
	}
	if !user.EmailVerified() {
		t.Error("a user created from a verified provider email should be verified")
	}
}

// --- email verification ---------------------------------------------------------------

func TestVerifyEmail_FullFlow(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	user, err := h.uc.Register(ctx, "new@example.com", "long-enough-password", "New")
	if err != nil {
		t.Fatal(err)
	}
	if len(h.mailer.sent) != 1 || h.mailer.sent[0].to != "new@example.com" {
		t.Fatalf("expected one verification mail to new@example.com, got %+v", h.mailer.sent)
	}
	token := tokenFromMail(t, h.mailer.sent[0])

	if err := h.uc.VerifyEmail(ctx, token); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	if !h.users.byID[user.ID].EmailVerified() {
		t.Error("user should be verified")
	}
	if err := h.uc.VerifyEmail(ctx, token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("second use: err = %v, want ErrInvalidToken (single use)", err)
	}
}

func TestVerifyEmail_ExpiredAndUnknownTokens(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	h.uc.Register(ctx, "new@example.com", "long-enough-password", "New")
	token := tokenFromMail(t, h.mailer.sent[0])

	h.clock.now = h.clock.now.Add(49 * time.Hour)
	if err := h.uc.VerifyEmail(ctx, token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expired: err = %v, want ErrInvalidToken", err)
	}
	if err := h.uc.VerifyEmail(ctx, "not-a-real-token"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("unknown: err = %v, want ErrInvalidToken", err)
	}
}

func TestResendVerification_InvalidatesThePreviousLink(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	user, _ := h.uc.Register(ctx, "new@example.com", "long-enough-password", "New")
	first := tokenFromMail(t, h.mailer.sent[0])
	if err := h.uc.ResendVerification(ctx, user); err != nil {
		t.Fatal(err)
	}
	if len(h.mailer.sent) != 2 {
		t.Fatalf("want 2 mails, got %d", len(h.mailer.sent))
	}
	if err := h.uc.VerifyEmail(ctx, first); !errors.Is(err, ErrInvalidToken) {
		t.Error("the first link should stop working once a new one is issued")
	}
	if err := h.uc.VerifyEmail(ctx, tokenFromMail(t, h.mailer.sent[1])); err != nil {
		t.Errorf("the new link should work: %v", err)
	}
}

// --- password reset ---------------------------------------------------------------------

func TestPasswordReset_FullFlow(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	user, _ := h.uc.Register(ctx, "me@example.com", "old-long-password", "Me")
	h.mailer.sent = nil
	versionBefore := user.TokenVersion

	if err := h.uc.RequestPasswordReset(ctx, "ME@example.com"); err != nil {
		t.Fatal(err)
	}
	if len(h.mailer.sent) != 1 || !strings.Contains(h.mailer.sent[0].body, "https://lagune.test/reset-password?token=") {
		t.Fatalf("expected a reset mail with a link, got %+v", h.mailer.sent)
	}
	token := tokenFromMail(t, h.mailer.sent[0])

	updated, err := h.uc.ResetPassword(ctx, token, "brand-new-password")
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if updated.TokenVersion <= versionBefore {
		t.Error("existing sessions must be revoked by a password reset")
	}
	if _, err := h.uc.AuthenticateWithPassword(ctx, "me@example.com", "brand-new-password"); err != nil {
		t.Errorf("the new password doesn't work: %v", err)
	}
	if _, err := h.uc.AuthenticateWithPassword(ctx, "me@example.com", "old-long-password"); err == nil {
		t.Error("the old password still works")
	}
	if !updated.EmailVerified() {
		t.Error("completing a reset proves the inbox: the address should be verified")
	}
	if _, err := h.uc.ResetPassword(ctx, token, "another-new-password"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("reusing the link: err = %v, want ErrInvalidToken", err)
	}
}

func TestPasswordReset_UnknownAddressSendsNothingAndReportsNothing(t *testing.T) {
	h := newAuthHarness()
	if err := h.uc.RequestPasswordReset(context.Background(), "ghost@example.com"); err != nil {
		t.Errorf("err = %v, want nil: the caller must not learn the address is unknown", err)
	}
	if len(h.mailer.sent) != 0 {
		t.Errorf("sent %d mails for an unknown address", len(h.mailer.sent))
	}
}

func TestPasswordReset_WeakPasswordDoesNotBurnTheLink(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	h.uc.Register(ctx, "me@example.com", "old-long-password", "Me")
	h.mailer.sent = nil
	h.uc.RequestPasswordReset(ctx, "me@example.com")
	token := tokenFromMail(t, h.mailer.sent[0])

	var v *ValidationError
	if _, err := h.uc.ResetPassword(ctx, token, "short"); !errors.As(err, &v) {
		t.Fatalf("err = %v, want a *ValidationError", err)
	}
	if _, err := h.uc.ResetPassword(ctx, token, "long-enough-password"); err != nil {
		t.Errorf("the link should still work after a rejected password: %v", err)
	}
}

func TestPasswordReset_ExpiresAfterAnHour(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	h.uc.Register(ctx, "me@example.com", "old-long-password", "Me")
	h.mailer.sent = nil
	h.uc.RequestPasswordReset(ctx, "me@example.com")
	token := tokenFromMail(t, h.mailer.sent[0])

	h.clock.now = h.clock.now.Add(61 * time.Minute)
	if _, err := h.uc.ResetPassword(ctx, token, "long-enough-password"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken after the 1h lifetime", err)
	}
}

func TestPasswordReset_NewRequestInvalidatesTheOldLink(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	h.uc.Register(ctx, "me@example.com", "old-long-password", "Me")
	h.mailer.sent = nil
	h.uc.RequestPasswordReset(ctx, "me@example.com")
	h.uc.RequestPasswordReset(ctx, "me@example.com")
	if _, err := h.uc.ResetPassword(ctx, tokenFromMail(t, h.mailer.sent[0]), "long-enough-password"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("older link: err = %v, want ErrInvalidToken", err)
	}
	if _, err := h.uc.ResetPassword(ctx, tokenFromMail(t, h.mailer.sent[1]), "long-enough-password"); err != nil {
		t.Errorf("latest link: %v", err)
	}
}

func TestLogoutAll_BumpsTokenVersion(t *testing.T) {
	h := newAuthHarness()
	ctx := context.Background()
	user, _ := h.uc.Register(ctx, "me@example.com", "old-long-password", "Me")
	before := user.TokenVersion
	if err := h.uc.LogoutAll(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if h.users.byID[user.ID].TokenVersion != before+1 {
		t.Error("LogoutAll must bump the token version")
	}
}

func TestEmailFlows_DisabledWithoutConfig(t *testing.T) {
	uc := NewAuthUseCase(newFakeUserRepo(), fakePasswordHasher{}, &steppingClock{now: time.Now()})
	ctx := context.Background()
	if _, err := uc.Register(ctx, "me@example.com", "long-enough-password", "Me"); err != nil {
		t.Fatalf("registration must work without email configured: %v", err)
	}
	if err := uc.VerifyEmail(ctx, "x"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("VerifyEmail without config: err = %v, want ErrInvalidToken", err)
	}
	if _, err := uc.ResetPassword(ctx, "x", "long-enough-password"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("ResetPassword without config: err = %v, want ErrInvalidToken", err)
	}
}

// --- private beta: who may create an account ------------------------------------------

func newClosedHarness(allowed ...string) authHarness {
	h := newAuthHarness()
	h.uc.WithSignupPolicy(func(email string) bool {
		for _, a := range allowed {
			if a == "*" || (email != "" && strings.EqualFold(a, email)) {
				return true
			}
		}
		return false
	})
	return h
}

func TestSignup_ClosedRefusesPasswordRegistration(t *testing.T) {
	h := newClosedHarness()
	_, err := h.uc.Register(context.Background(), "stranger@example.com", "long-enough-password", "S")
	if !errors.Is(err, ErrSignupClosed) {
		t.Fatalf("err = %v, want ErrSignupClosed", err)
	}
	if len(h.users.byID) != 0 {
		t.Error("no account may be created while registration is closed")
	}
}

func TestSignup_ClosedAnswersTheSameForAKnownAndAnUnknownAddress(t *testing.T) {
	// Otherwise the "closed" wall would still leak which addresses have an account.
	h := newClosedHarness("member@example.com")
	if _, err := h.uc.Register(context.Background(), "member@example.com", "long-enough-password", "M"); err != nil {
		t.Fatal(err)
	}
	_, errKnown := h.uc.Register(context.Background(), "member@example.com", "long-enough-password", "M")
	_, errStranger := h.uc.Register(context.Background(), "stranger@example.com", "long-enough-password", "S")
	if !errors.Is(errKnown, ErrEmailAlreadyRegistered) {
		t.Fatalf("an allowed, already-registered address: err = %v", errKnown)
	}
	if !errors.Is(errStranger, ErrSignupClosed) {
		t.Fatalf("an address off the list: err = %v", errStranger)
	}
	// An address off the list never gets as far as the "already registered" check.
	h.users.byID[uuid.New()] = &entities.User{ID: uuid.New(), Email: strPtr("taken@example.com"), DisplayName: "T"}
	if _, err := h.uc.Register(context.Background(), "taken@example.com", "long-enough-password", "T"); !errors.Is(err, ErrSignupClosed) {
		t.Errorf("off-list + existing account: err = %v, want ErrSignupClosed (not a leak of existence)", err)
	}
}

func TestSignup_AllowListIsCaseInsensitiveAndSelective(t *testing.T) {
	h := newClosedHarness("Friend@Example.com")
	if _, err := h.uc.Register(context.Background(), "friend@example.COM", "long-enough-password", "F"); err != nil {
		t.Errorf("an allow-listed address in another case must be admitted: %v", err)
	}
	if _, err := h.uc.Register(context.Background(), "other@example.com", "long-enough-password", "O"); !errors.Is(err, ErrSignupClosed) {
		t.Errorf("an address off the list: err = %v, want ErrSignupClosed", err)
	}
}

func TestSignup_OpenWithWildcard(t *testing.T) {
	h := newClosedHarness("*")
	if _, err := h.uc.Register(context.Background(), "anyone@example.com", "long-enough-password", "A"); err != nil {
		t.Fatalf("open registration refused someone: %v", err)
	}
}

func TestSignup_ClosedAlsoGatesProviderSignIns(t *testing.T) {
	h := newClosedHarness("member@example.com")
	ctx := context.Background()

	if _, err := h.uc.AuthenticateOrRegisterWithGoogle(ctx, "g-1", "stranger@example.com", true, "S"); !errors.Is(err, ErrSignupClosed) {
		t.Errorf("Google, address off the list: err = %v, want ErrSignupClosed", err)
	}
	if _, err := h.uc.AuthenticateOrRegisterWithApple(ctx, "a-1", "", false, "Hidden"); !errors.Is(err, ErrSignupClosed) {
		t.Errorf("Apple with a hidden address can't be matched to the list: err = %v, want ErrSignupClosed", err)
	}
	if _, err := h.uc.AuthenticateOrRegisterWithRobine(ctx, "r-1", "stranger@example.com", false, "S"); !errors.Is(err, ErrSignupClosed) {
		t.Errorf("Robine, unverified address: err = %v, want ErrSignupClosed", err)
	}
	if len(h.users.byID) != 0 {
		t.Errorf("%d account(s) were created through a provider while closed", len(h.users.byID))
	}

	if _, err := h.uc.AuthenticateOrRegisterWithGoogle(ctx, "g-2", "member@example.com", true, "M"); err != nil {
		t.Errorf("an allow-listed address may register with Google: %v", err)
	}
}

func TestSignup_ClosedNeverBlocksExistingAccounts(t *testing.T) {
	h := newAuthHarness() // open while the account is created
	ctx := context.Background()
	existing, _ := h.uc.Register(ctx, "old@example.com", "long-enough-password", "Old")
	now := h.clock.now
	existing.EmailVerifiedAt = &now
	sub := "google-old"
	existing.GoogleSub = &sub

	h.uc.WithSignupPolicy(func(string) bool { return false }) // beta closes afterwards

	if _, err := h.uc.AuthenticateWithPassword(ctx, "old@example.com", "long-enough-password"); err != nil {
		t.Errorf("an existing account can no longer sign in with its password: %v", err)
	}
	if got, err := h.uc.AuthenticateOrRegisterWithGoogle(ctx, sub, "old@example.com", true, "Old"); err != nil || got.ID != existing.ID {
		t.Errorf("an existing Google-linked account can no longer sign in: (%v, %v)", got, err)
	}
	// Linking a provider to an existing account by (verified) address isn't creating one either.
	if got, err := h.uc.AuthenticateOrRegisterWithApple(ctx, "apple-old", "old@example.com", true, "Old"); err != nil || got.ID != existing.ID {
		t.Errorf("linking Apple to an existing account was blocked: (%v, %v)", got, err)
	}
}
