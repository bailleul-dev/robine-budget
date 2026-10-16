package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestOAuthRepositories(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	clients := NewOAuthClientRepository(pool)
	codes := NewOAuthAuthorizationCodeRepository(pool)
	grants := NewOAuthGrantRepository(pool)

	client, err := clients.Add(ctx, &entities.OAuthClient{ID: "test-" + uuid.NewString(), Name: "Claude", RedirectURIs: []string{"http://localhost:1/cb", "https://claude.ai/cb"}})
	if err != nil {
		t.Fatalf("Add client: %v", err)
	}
	// Registered after createTestUser, so it runs first: the grant and code
	// rows cascade from the client.
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM oauth_clients WHERE id = $1", client.ID) })
	got, err := clients.GetByID(ctx, client.ID)
	if err != nil || got == nil || len(got.RedirectURIs) != 2 || got.SecretHash != nil {
		t.Fatalf("GetByID = %+v, %v", got, err)
	}

	// Codes are consumed exactly once.
	code := &entities.OAuthAuthorizationCode{CodeHash: uuid.NewString(), ClientID: client.ID, UserID: user.ID, RedirectURI: "http://localhost:1/cb", CodeChallenge: "c", ExpiresAt: time.Now().Add(time.Minute)}
	if err := codes.Add(ctx, code); err != nil {
		t.Fatalf("Add code: %v", err)
	}
	if c, err := codes.Consume(ctx, code.CodeHash); err != nil || c == nil || c.UserID != user.ID {
		t.Fatalf("Consume = %+v, %v", c, err)
	}
	if c, _ := codes.Consume(ctx, code.CodeHash); c != nil {
		t.Fatal("code consumed twice")
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	grant, err := grants.Add(ctx, &entities.OAuthGrant{
		ID: uuid.New(), ClientID: client.ID, UserID: user.ID,
		AccessTokenHash: "at-" + uuid.NewString(), AccessExpiresAt: now.Add(time.Hour),
		RefreshTokenHash: "rt-" + uuid.NewString(), RefreshExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now, RefreshedAt: now,
	})
	if err != nil {
		t.Fatalf("Add grant: %v", err)
	}
	if grant.ClientName != "Claude" {
		t.Errorf("ClientName = %q, want the joined client name", grant.ClientName)
	}
	if g, _ := grants.GetByAccessTokenHash(ctx, grant.AccessTokenHash); g == nil || g.ID != grant.ID {
		t.Fatalf("GetByAccessTokenHash = %+v", g)
	}

	// Rotation is a compare-and-swap on the old refresh hash.
	rotated := &entities.OAuthGrant{AccessTokenHash: "at-" + uuid.NewString(), AccessExpiresAt: now.Add(time.Hour), RefreshTokenHash: "rt-" + uuid.NewString(), RefreshExpiresAt: now.Add(24 * time.Hour), RefreshedAt: now}
	if ok, err := grants.Rotate(ctx, grant.ID, grant.RefreshTokenHash, rotated); err != nil || !ok {
		t.Fatalf("Rotate = %v, %v", ok, err)
	}
	if ok, _ := grants.Rotate(ctx, grant.ID, grant.RefreshTokenHash, rotated); ok {
		t.Fatal("Rotate succeeded twice with the same old refresh token")
	}
	if g, _ := grants.GetByRefreshTokenHash(ctx, rotated.RefreshTokenHash); g == nil {
		t.Fatal("rotated refresh token not found")
	}

	list, err := grants.ListForUser(ctx, user.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListForUser = %d grants, %v", len(list), err)
	}
	if ok, _ := grants.Delete(ctx, grant.ID, uuid.New()); ok {
		t.Fatal("another user deleted this grant")
	}
	if ok, err := grants.Delete(ctx, grant.ID, user.ID); err != nil || !ok {
		t.Fatalf("Delete = %v, %v", ok, err)
	}
	if g, _ := grants.GetByAccessTokenHash(ctx, rotated.AccessTokenHash); g != nil {
		t.Fatal("grant still there after Delete")
	}
}
