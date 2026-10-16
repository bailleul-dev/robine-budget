package core

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
)

// fakeUserRepo is a minimal in-memory ports.UserRepository, just enough for
// SeedDevData's Register -> GetByEmail path.
type fakeUserRepo struct {
	byID    map[uuid.UUID]*entities.User
	byEmail map[string]*entities.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[uuid.UUID]*entities.User{}, byEmail: map[string]*entities.User{}}
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*entities.User, error) {
	return r.byID[id], nil
}
func (r *fakeUserRepo) GetByEmail(_ context.Context, email string) (*entities.User, error) {
	return r.byEmail[email], nil
}
func (r *fakeUserRepo) GetByRobineSub(_ context.Context, _ string) (*entities.User, error) {
	return nil, nil
}
func (r *fakeUserRepo) GetByGoogleSub(_ context.Context, _ string) (*entities.User, error) {
	return nil, nil
}
func (r *fakeUserRepo) GetByAppleSub(_ context.Context, _ string) (*entities.User, error) {
	return nil, nil
}
func (r *fakeUserRepo) Add(_ context.Context, u *entities.User) (*entities.User, error) {
	r.byID[u.ID] = u
	r.byEmail[*u.Email] = u
	return u, nil
}
func (r *fakeUserRepo) Update(_ context.Context, u *entities.User) (*entities.User, error) {
	r.byID[u.ID] = u
	return u, nil
}

func (r *fakeUserRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(r.byID, id)
	return nil
}

func (r *fakeUserRepo) BumpTokenVersion(_ context.Context, id uuid.UUID) error {
	if u := r.byID[id]; u != nil {
		u.TokenVersion++
	}
	return nil
}

// fakeBankProfileRepo is a minimal in-memory ports.BankProfileRepository.
type fakeBankProfileRepo struct {
	profiles map[uuid.UUID]*entities.BankProfile
}

func newFakeBankProfileRepo() *fakeBankProfileRepo {
	return &fakeBankProfileRepo{profiles: map[uuid.UUID]*entities.BankProfile{}}
}

func (r *fakeBankProfileRepo) GetByID(_ context.Context, id uuid.UUID) (*entities.BankProfile, error) {
	return r.profiles[id], nil
}
func (r *fakeBankProfileRepo) ListForUser(_ context.Context, _ uuid.UUID) ([]entities.BankProfile, error) {
	out := make([]entities.BankProfile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, *p)
	}
	return out, nil
}
func (r *fakeBankProfileRepo) Add(_ context.Context, p *entities.BankProfile) (*entities.BankProfile, error) {
	r.profiles[p.ID] = p
	return p, nil
}
func (r *fakeBankProfileRepo) Update(_ context.Context, p *entities.BankProfile) (*entities.BankProfile, error) {
	r.profiles[p.ID] = p
	return p, nil
}

func TestSeedDevData_Disabled(t *testing.T) {
	users := newFakeUserRepo()
	auth := services.NewAuthUseCase(users, security.BcryptPasswordHasher{}, SystemClock{})
	settings := Settings{SeedDevUser: false, SeedDevUserEmail: "dev@example.com", SeedDevUserPassword: "devpass123"}

	SeedDevData(context.Background(), settings, auth, users, SystemClock{})

	if len(users.byEmail) != 0 {
		t.Errorf("got %d users, want 0 when SeedDevUser is disabled", len(users.byEmail))
	}
}

func TestSeedDevData_CreatesUserOnce(t *testing.T) {
	users := newFakeUserRepo()
	auth := services.NewAuthUseCase(users, security.BcryptPasswordHasher{}, SystemClock{})
	settings := Settings{SeedDevUser: true, SeedDevUserEmail: "dev@example.com", SeedDevUserPassword: "devpass123"}

	SeedDevData(context.Background(), settings, auth, users, SystemClock{})
	if _, ok := users.byEmail["dev@example.com"]; !ok {
		t.Fatal("expected the dev user to be created")
	}

	// Idempotent: a second call must not error or duplicate the user, since
	// Register returning ErrEmailAlreadyRegistered is the expected (and
	// silently handled) outcome on every startup after the first.
	SeedDevData(context.Background(), settings, auth, users, SystemClock{})
	if len(users.byEmail) != 1 {
		t.Fatalf("got %d users after seeding twice, want 1", len(users.byEmail))
	}
}

func TestSeedGlobalBankProfiles_CreatesCreditMutuelProfile(t *testing.T) {
	profiles := newFakeBankProfileRepo()

	SeedGlobalBankProfiles(context.Background(), profiles, SystemClock{})

	found := false
	for _, p := range profiles.profiles {
		if p.UserID == nil && p.Name == creditMutuelProfileName {
			found = true
			if p.ColumnMapping.Debit == nil || p.ColumnMapping.Credit == nil {
				t.Error("Crédit Mutuel profile should map both Débit and Crédit columns")
			}
		}
	}
	if !found {
		t.Fatal("expected a global Crédit Mutuel profile to be seeded")
	}
}

func TestSeedGlobalBankProfiles_Idempotent(t *testing.T) {
	profiles := newFakeBankProfileRepo()

	SeedGlobalBankProfiles(context.Background(), profiles, SystemClock{})
	SeedGlobalBankProfiles(context.Background(), profiles, SystemClock{})

	count := 0
	for _, p := range profiles.profiles {
		if p.UserID == nil && p.Name == creditMutuelProfileName {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("got %d Crédit Mutuel profiles after seeding twice, want 1", count)
	}
}
