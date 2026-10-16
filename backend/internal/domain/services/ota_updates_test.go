package services

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type fakeUpdateStore struct {
	latest *entities.OTAUpdate
	err    error
}

func (f fakeUpdateStore) Latest(context.Context, string, string) (*entities.OTAUpdate, error) {
	return f.latest, f.err
}

func (f fakeUpdateStore) OpenAsset(context.Context, string, string, string) (io.ReadSeekCloser, time.Time, error) {
	return nil, time.Time{}, errors.New("not implemented")
}

func TestResolveUpdate(t *testing.T) {
	update := &entities.OTAUpdate{ID: "update-2"}
	rollback := &entities.OTAUpdate{ID: "rollback", IsRollback: true}
	storeErr := errors.New("disk on fire")

	cases := []struct {
		name     string
		store    fakeUpdateStore
		req      UpdateRequest
		wantKind UpdateDecisionKind
		wantErr  error
	}{
		{"unsupported platform", fakeUpdateStore{latest: update}, UpdateRequest{ProtocolVersion: 1, Platform: "web", RuntimeVersion: "1.0.0"}, 0, ErrUnsupportedPlatform},
		{"missing runtime version", fakeUpdateStore{latest: update}, UpdateRequest{ProtocolVersion: 1, Platform: "ios"}, 0, ErrMissingRuntime},
		{"store error", fakeUpdateStore{err: storeErr}, UpdateRequest{ProtocolVersion: 1, Platform: "ios", RuntimeVersion: "1.0.0"}, 0, storeErr},
		{"nothing published, v1", fakeUpdateStore{}, UpdateRequest{ProtocolVersion: 1, Platform: "ios", RuntimeVersion: "1.0.0"}, UpdateDecisionNoUpdate, nil},
		{"nothing published, v0", fakeUpdateStore{}, UpdateRequest{ProtocolVersion: 0, Platform: "ios", RuntimeVersion: "1.0.0"}, 0, ErrNoUpdateForRuntime},
		{"new update", fakeUpdateStore{latest: update}, UpdateRequest{ProtocolVersion: 1, Platform: "android", RuntimeVersion: "1.0.0", CurrentUpdateID: "update-1"}, UpdateDecisionUpdate, nil},
		{"already on latest, v1", fakeUpdateStore{latest: update}, UpdateRequest{ProtocolVersion: 1, Platform: "ios", RuntimeVersion: "1.0.0", CurrentUpdateID: "update-2"}, UpdateDecisionNoUpdate, nil},
		{"already on latest, v0 still gets the manifest", fakeUpdateStore{latest: update}, UpdateRequest{ProtocolVersion: 0, Platform: "ios", RuntimeVersion: "1.0.0", CurrentUpdateID: "update-2"}, UpdateDecisionUpdate, nil},
		{"rollback", fakeUpdateStore{latest: rollback}, UpdateRequest{ProtocolVersion: 1, Platform: "ios", RuntimeVersion: "1.0.0", CurrentUpdateID: "update-2", EmbeddedUpdateID: "embedded"}, UpdateDecisionRollBack, nil},
		{"rollback, already on embedded", fakeUpdateStore{latest: rollback}, UpdateRequest{ProtocolVersion: 1, Platform: "ios", RuntimeVersion: "1.0.0", CurrentUpdateID: "embedded", EmbeddedUpdateID: "embedded"}, UpdateDecisionNoUpdate, nil},
		{"rollback, v0", fakeUpdateStore{latest: rollback}, UpdateRequest{ProtocolVersion: 0, Platform: "ios", RuntimeVersion: "1.0.0"}, 0, ErrNoUpdateForRuntime},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewResolveUpdateUseCase(tc.store).Resolve(context.Background(), tc.req)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err == nil && got.Kind != tc.wantKind {
				t.Fatalf("kind = %v, want %v", got.Kind, tc.wantKind)
			}
		})
	}
}
