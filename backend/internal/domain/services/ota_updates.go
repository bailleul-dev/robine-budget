// Use-case: decide what an installed copy of the mobile app should run
// next — a newer over-the-air update, a rollback to the JS embedded in its
// binary, or nothing — following the Expo Updates protocol's rules. HTTP
// concerns (headers, multipart, signing) stay in adapters/web.
package services

import (
	"context"
	"errors"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
)

var (
	ErrUnsupportedPlatform = errors.New("unsupported platform, expected ios or android")
	ErrMissingRuntime      = errors.New("no runtime version provided")
	// ErrNoUpdateForRuntime is only returned to protocol-0 clients, which
	// can't understand a "no update available" directive.
	ErrNoUpdateForRuntime = errors.New("no update available for this runtime version")
)

type UpdateDecisionKind int

const (
	UpdateDecisionNoUpdate UpdateDecisionKind = iota
	UpdateDecisionUpdate
	UpdateDecisionRollBack
)

// UpdateRequest is what a client tells us about itself when checking for
// an update (the expo-* request headers).
type UpdateRequest struct {
	ProtocolVersion  int
	Platform         string
	RuntimeVersion   string
	CurrentUpdateID  string
	EmbeddedUpdateID string
}

type UpdateDecision struct {
	Kind UpdateDecisionKind
	// Update is the update to serve (UpdateDecisionUpdate) or the rollback
	// marker (UpdateDecisionRollBack).
	Update *entities.OTAUpdate
}

type ResolveUpdateUseCase struct {
	store ports.UpdateStore
}

func NewResolveUpdateUseCase(store ports.UpdateStore) *ResolveUpdateUseCase {
	return &ResolveUpdateUseCase{store: store}
}

func (uc *ResolveUpdateUseCase) Resolve(ctx context.Context, req UpdateRequest) (UpdateDecision, error) {
	if req.Platform != "ios" && req.Platform != "android" {
		return UpdateDecision{}, ErrUnsupportedPlatform
	}
	if req.RuntimeVersion == "" {
		return UpdateDecision{}, ErrMissingRuntime
	}
	// Directives (noUpdateAvailable, rollBackToEmbedded) only exist from
	// protocol 1 on.
	supportsDirectives := req.ProtocolVersion >= 1

	latest, err := uc.store.Latest(ctx, req.RuntimeVersion, req.Platform)
	if err != nil {
		return UpdateDecision{}, err
	}
	if latest == nil {
		if !supportsDirectives {
			return UpdateDecision{}, ErrNoUpdateForRuntime
		}
		return UpdateDecision{Kind: UpdateDecisionNoUpdate}, nil
	}

	if latest.IsRollback {
		if !supportsDirectives {
			return UpdateDecision{}, ErrNoUpdateForRuntime
		}
		// Already running the embedded update: nothing to roll back.
		if req.EmbeddedUpdateID != "" && req.CurrentUpdateID == req.EmbeddedUpdateID {
			return UpdateDecision{Kind: UpdateDecisionNoUpdate}, nil
		}
		return UpdateDecision{Kind: UpdateDecisionRollBack, Update: latest}, nil
	}

	if supportsDirectives && req.CurrentUpdateID == latest.ID {
		return UpdateDecision{Kind: UpdateDecisionNoUpdate}, nil
	}
	return UpdateDecision{Kind: UpdateDecisionUpdate, Update: latest}, nil
}
