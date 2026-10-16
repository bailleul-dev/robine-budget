// Package ports declares the interfaces the domain depends on; adapters
// implement them. Kept import-free of the adapters package (dependency rule
// enforced by construction) — mirrors backend/app/domain/ports.py.
package ports

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

type UserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entities.User, error)
	GetByEmail(ctx context.Context, email string) (*entities.User, error)
	GetByRobineSub(ctx context.Context, sub string) (*entities.User, error)
	GetByGoogleSub(ctx context.Context, sub string) (*entities.User, error)
	GetByAppleSub(ctx context.Context, sub string) (*entities.User, error)
	Add(ctx context.Context, u *entities.User) (*entities.User, error)
	Update(ctx context.Context, u *entities.User) (*entities.User, error)
	// Delete permanently removes the user and everything they own
	// (accounts and all their data, bank connection, categories, bank
	// profiles, comments, share links). Irreversible.
	Delete(ctx context.Context, id uuid.UUID) error
	// BumpTokenVersion invalidates every session token issued so far for
	// the user (they are signed in again only by logging in).
	BumpTokenVersion(ctx context.Context, id uuid.UUID) error
}

// AuthTokenRepository stores the single-use tokens behind email
// verification and password reset.
type AuthTokenRepository interface {
	Create(ctx context.Context, t *entities.AuthToken) error
	// Consume atomically marks the unused, unexpired token with this hash and
	// kind as used and returns its owner; ok is false when there is no such
	// token (unknown, already used, or expired).
	Consume(ctx context.Context, kind entities.AuthTokenKind, tokenHash []byte, now time.Time) (userID uuid.UUID, ok bool, err error)
	// InvalidateForUser marks every outstanding token of this kind used.
	InvalidateForUser(ctx context.Context, userID uuid.UUID, kind entities.AuthTokenKind, now time.Time) error
}

// Mailer sends a plain-text transactional email.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

type AccountRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entities.Account, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.Account, error)
	// GetByBankSyncAccountID looks up the account a previous sync already
	// linked to this woob account id, if any - lets a sync find-or-create
	// instead of duplicating an account on every run.
	GetByBankSyncAccountID(ctx context.Context, userID uuid.UUID, bankSyncAccountID string) (*entities.Account, error)
	Add(ctx context.Context, a *entities.Account) (*entities.Account, error)
	// Delete removes the account and, via ON DELETE CASCADE (see migration
	// 000014), every operation, comment, category split, planned item and
	// occurrence, import batch, and balance checkpoint that belongs to it.
	Delete(ctx context.Context, id uuid.UUID) error
}

type BankProfileRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entities.BankProfile, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.BankProfile, error)
	Add(ctx context.Context, p *entities.BankProfile) (*entities.BankProfile, error)
	Update(ctx context.Context, p *entities.BankProfile) (*entities.BankProfile, error)
}

type ImportBatchRepository interface {
	Add(ctx context.Context, b *entities.ImportBatch) (*entities.ImportBatch, error)
	ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.ImportBatch, error)
}

type OperationRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entities.Operation, error)
	FindByReference(ctx context.Context, accountID uuid.UUID, rawReference string) (*entities.Operation, error)
	FindByDedupHash(ctx context.Context, accountID uuid.UUID, dedupHash string) (*entities.Operation, error)
	Add(ctx context.Context, op *entities.Operation) (*entities.Operation, error)
	AddMany(ctx context.Context, ops []entities.Operation) ([]entities.Operation, error)
	// ListForAccount: dateFrom/dateTo nil means unbounded on that side. Used
	// where the full (unpaginated) range is needed, e.g. previsions summing
	// every operation in a window — for the operations screen itself, see
	// ListPageForAccount instead.
	ListForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date) ([]entities.Operation, error)
	// ListPageForAccount keyset-paginates newest first (date, created_at, id
	// all descending): after nil fetches the first page, otherwise resumes
	// strictly after that cursor. Returns up to limit+1 rows so the caller
	// can tell whether a further page exists without a separate COUNT.
	// uncategorizedOnly keeps just the operations with neither a category
	// nor category splits.
	ListPageForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date, uncategorizedOnly bool, after *entities.OperationPageCursor, limit int) ([]entities.Operation, error)
	ListUnreconciled(ctx context.Context, accountID uuid.UUID) ([]entities.Operation, error)
	Update(ctx context.Context, op *entities.Operation) (*entities.Operation, error)
	// CategoryCountsForLabels aggregates, across every account and every
	// user in the system (not just one account), how many already-
	// categorized operations share each given normalized label (lower+
	// trim) and which category they were given. This is the whole system's
	// categorization history, both automatic and manual — it's what lets
	// AutoCategorizeUseCase learn from every user's own corrections without
	// any separate "rules" table to maintain. normalizedLabels should
	// already be deduplicated by the caller.
	CategoryCountsForLabels(ctx context.Context, normalizedLabels []string) ([]LabelCategoryCount, error)
}

// LabelCategoryCount is one (label, category) pair one particular user
// chose for at least one operation, and how many times — see
// CategoryCountsForLabels.
type LabelCategoryCount struct {
	NormalizedLabel string
	OwnerUserID     uuid.UUID
	CategoryID      uuid.UUID
	Count           int
}

type CommentRepository interface {
	ListForOperation(ctx context.Context, operationID uuid.UUID) ([]entities.Comment, error)
	// ListForAuthor returns every comment the user wrote, on any account
	// (their own or a shared one) — part of their personal-data export.
	ListForAuthor(ctx context.Context, userID uuid.UUID) ([]entities.Comment, error)
	Add(ctx context.Context, c *entities.Comment) (*entities.Comment, error)
}

type OperationCategorySplitRepository interface {
	ListForOperation(ctx context.Context, operationID uuid.UUID) ([]entities.OperationCategorySplit, error)
	// ListForOperationIDs fetches splits for a set of operations (typically
	// one page of the operations list) in one query — avoids an N+1 query
	// per row when assembling the list response.
	ListForOperationIDs(ctx context.Context, operationIDs []uuid.UUID) ([]entities.OperationCategorySplit, error)
	// ReplaceForOperation atomically swaps every split on operationID for
	// the given ones (IDs are assigned by the repository). An empty slice
	// clears the operation's splits entirely.
	ReplaceForOperation(ctx context.Context, operationID uuid.UUID, splits []entities.OperationCategorySplit) ([]entities.OperationCategorySplit, error)
}

type PlannedItemRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entities.PlannedItem, error)
	ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.PlannedItem, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.PlannedItem, error)
	Add(ctx context.Context, item *entities.PlannedItem) (*entities.PlannedItem, error)
	Update(ctx context.Context, item *entities.PlannedItem) (*entities.PlannedItem, error)
}

type PlannedOccurrenceRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entities.PlannedOccurrence, error)
	ListForItem(ctx context.Context, plannedItemID uuid.UUID) ([]entities.PlannedOccurrence, error)
	ListForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date) ([]entities.PlannedOccurrence, error)
	GetByItemAndDate(ctx context.Context, plannedItemID uuid.UUID, dueDate civil.Date) (*entities.PlannedOccurrence, error)
	Add(ctx context.Context, occ *entities.PlannedOccurrence) (*entities.PlannedOccurrence, error)
	AddMany(ctx context.Context, occs []entities.PlannedOccurrence) ([]entities.PlannedOccurrence, error)
	Update(ctx context.Context, occ *entities.PlannedOccurrence) (*entities.PlannedOccurrence, error)
}

type CategoryRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entities.Category, error)
	ListVisibleTo(ctx context.Context, userID uuid.UUID) ([]entities.Category, error)
	Add(ctx context.Context, c *entities.Category) (*entities.Category, error)
	Update(ctx context.Context, c *entities.Category) (*entities.Category, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type CollaboratorRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entities.Collaborator, error)
	GetByToken(ctx context.Context, token string) (*entities.Collaborator, error)
	ListForOwner(ctx context.Context, ownerUserID uuid.UUID) ([]entities.Collaborator, error)
	// ListActiveForCollaborator: accepted and not revoked — the set of
	// grants that currently give collaboratorUserID access to someone
	// else's accounts.
	ListActiveForCollaborator(ctx context.Context, collaboratorUserID uuid.UUID) ([]entities.Collaborator, error)
	Add(ctx context.Context, c *entities.Collaborator) (*entities.Collaborator, error)
	Update(ctx context.Context, c *entities.Collaborator) (*entities.Collaborator, error)
}

type OAuthClientRepository interface {
	GetByID(ctx context.Context, id string) (*entities.OAuthClient, error)
	Add(ctx context.Context, c *entities.OAuthClient) (*entities.OAuthClient, error)
}

type OAuthAuthorizationCodeRepository interface {
	Add(ctx context.Context, c *entities.OAuthAuthorizationCode) error
	// Consume atomically deletes and returns the code with this hash, or
	// nil if there's none — so a code can be redeemed at most once, even
	// by two concurrent token requests.
	Consume(ctx context.Context, codeHash string) (*entities.OAuthAuthorizationCode, error)
}

type OAuthGrantRepository interface {
	Add(ctx context.Context, g *entities.OAuthGrant) (*entities.OAuthGrant, error)
	GetByAccessTokenHash(ctx context.Context, hash string) (*entities.OAuthGrant, error)
	GetByRefreshTokenHash(ctx context.Context, hash string) (*entities.OAuthGrant, error)
	// Rotate swaps the grant's tokens for new ones, but only if its refresh
	// token hash is still oldRefreshHash — false means another request
	// already rotated (or revoked) it, so a refresh token works only once.
	Rotate(ctx context.Context, id uuid.UUID, oldRefreshHash string, g *entities.OAuthGrant) (bool, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.OAuthGrant, error)
	// Delete revokes one of userID's grants; false if they have no such grant.
	Delete(ctx context.Context, id, userID uuid.UUID) (bool, error)
}

type BalanceCheckpointRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entities.BalanceCheckpoint, error)
	ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.BalanceCheckpoint, error)
	GetLatestOnOrBefore(ctx context.Context, accountID uuid.UUID, asOf civil.Date) (*entities.BalanceCheckpoint, error)
	Add(ctx context.Context, c *entities.BalanceCheckpoint) (*entities.BalanceCheckpoint, error)
	// Upsert inserts a checkpoint, or replaces the balance/note of the one
	// already there for (AccountID, AsOfDate) if any (unique constraint) -
	// a bank sync may run more than once on the same day.
	Upsert(ctx context.Context, c *entities.BalanceCheckpoint) (*entities.BalanceCheckpoint, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// BankSyncConnector fetches a live account snapshot from an external bank
// aggregator. The only implementation today (adapters/banksync) shells out
// to the `bank_sync` Python CLI as a port (stdin/stdout, no filesystem
// exchange) — kept behind this interface because it may become an HTTP call
// to a separate service later, without callers changing.
type BankSyncConnector interface {
	// FetchAccounts logs in with login/password and fetches accounts.
	// session is the connector's own opaque state (cookies + last 2FA
	// validation) from a previous call, nil/empty for a first-ever
	// connection. It returns the accounts and the connector's updated
	// session state — the caller (services.SyncBankAccountsUseCase) is
	// responsible for persisting that returned session, not this
	// interface's job. newSession may be non-empty even when err != nil
	// (e.g. an app-validation that timed out still leaves progress worth
	// keeping for the next attempt) — callers should persist it regardless
	// of whether err is set.
	FetchAccounts(ctx context.Context, login, password string, session []byte) (accounts []entities.BankSyncAccount, newSession []byte, err error)
}

// BankConnectionRepository stores one user's link to a bank aggregator
// backend (woob's "creditmutuel" for now) - credentials and session state,
// both encrypted (see Encryptor). One connection per user today (mono-bank).
type BankConnectionRepository interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*entities.BankConnection, error)
	Add(ctx context.Context, c *entities.BankConnection) (*entities.BankConnection, error)
	// UpdateSession replaces the stored session ciphertext, e.g. after a
	// sync produced fresh cookies/2FA state worth keeping for next time.
	UpdateSession(ctx context.Context, id uuid.UUID, sessionEncrypted []byte) error
	// ListUserIDs returns every user with a bank connection - feeds the
	// daily sync cron (see services.SyncAllBankConnectionsUseCase), which
	// has no other way to discover who to sync.
	ListUserIDs(ctx context.Context) ([]uuid.UUID, error)
}

// Encryptor does symmetric encryption of small secrets at rest (bank
// credentials, session cookies) that the app needs to read back in
// plaintext - unlike PasswordHasher below, which is one-way. Implemented by
// adapters/security.SecretBox (AES-256-GCM).
type Encryptor interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(plain, hash string) bool
}

type Clock interface {
	Today() civil.Date
	Now() time.Time
}

// ErrInvalidUpdatePath is returned by an UpdateStore for a runtime
// version, update name or asset path that isn't a plain, safe path.
var ErrInvalidUpdatePath = errors.New("invalid update path")

// UpdateStore reads published over-the-air updates for the mobile app (see
// adapters/otaupdates, which reads `expo export` output from disk).
type UpdateStore interface {
	// Latest returns the most recently published update — or rollback
	// marker — for this runtime version and platform, or nil when nothing
	// has been published for it yet.
	Latest(ctx context.Context, runtimeVersion, platform string) (*entities.OTAUpdate, error)
	// OpenAsset opens one file of a published update for download. Must
	// reject any assetPath that would escape that update's own directory.
	OpenAsset(ctx context.Context, runtimeVersion, updateName, assetPath string) (io.ReadSeekCloser, time.Time, error)
}

// ManifestSigner signs an update manifest or directive so clients built
// with the matching certificate can refuse anything not published by us
// (Expo's code signing, rsa-v1_5-sha256). Implemented by
// adapters/security.RSASigner.
type ManifestSigner interface {
	// Sign returns the base64 signature of data.
	Sign(data []byte) (string, error)
	// KeyID is the "keyid" the client's codeSigningMetadata expects.
	KeyID() string
}
