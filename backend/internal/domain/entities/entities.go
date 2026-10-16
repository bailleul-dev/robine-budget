// Package entities holds plain domain structs — no db tags, no json tags,
// no framework imports. Mirrors backend/app/domain/entities.py exactly, so
// the two backends can run side by side against the same Postgres schema.
package entities

import (
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

type PlannedItemKind string

const (
	PlannedItemKindCharge PlannedItemKind = "charge"
	PlannedItemKindIncome PlannedItemKind = "income"
)

type RecurrenceType string

const (
	RecurrenceOneOff      RecurrenceType = "one_off"
	RecurrencePerpetual   RecurrenceType = "perpetual"
	RecurrenceFixedCount  RecurrenceType = "fixed_count"
	RecurrenceEveryXWeeks RecurrenceType = "every_x_weeks"
)

type OccurrenceStatus string

const (
	OccurrenceForecast   OccurrenceStatus = "forecast"
	OccurrenceReconciled OccurrenceStatus = "reconciled"
	OccurrenceSkipped    OccurrenceStatus = "skipped"
)

// PlannedItemMatchMode picks what an operation must share with a PlannedItem
// to count toward it. MatchModeLabel is the original behavior (one operation
// reconciles one occurrence, by exact label). MatchModeCategory is a budget
// envelope: every operation in the item's category over a period is summed
// into that period's occurrence (see ReconcileUseCase.CategoryBudgetActuals)
// rather than reconciled one at a time.
type PlannedItemMatchMode string

const (
	PlannedItemMatchModeLabel    PlannedItemMatchMode = "label"
	PlannedItemMatchModeCategory PlannedItemMatchMode = "category"
)

type AmountMode string

const (
	AmountModeSingle            AmountMode = "single"
	AmountModeDebitCredit       AmountMode = "debit_credit"
	AmountModeDebitCreditSigned AmountMode = "debit_credit_signed"
)

type User struct {
	ID           uuid.UUID
	Email        *string
	PasswordHash *string
	RobineSub    *string
	GoogleSub    *string
	AppleSub     *string
	DisplayName  string
	CreatedAt    time.Time
	// TokenVersion is embedded in every session token; bumping it revokes
	// all of the user's outstanding sessions.
	TokenVersion int
	// EmailVerifiedAt is nil until the address is proven (emailed link or a
	// provider-verified identity).
	EmailVerifiedAt *time.Time
}

// EmailVerified reports whether the user's address has been proven.
func (u *User) EmailVerified() bool { return u.EmailVerifiedAt != nil }

// AuthTokenKind is what a single-use emailed token is for.
type AuthTokenKind string

const (
	AuthTokenEmailVerification AuthTokenKind = "email_verification"
	AuthTokenPasswordReset     AuthTokenKind = "password_reset"
)

// AuthToken is a single-use, expiring secret sent by email. Only the SHA-256
// of the token is stored (TokenHash), never the token itself.
type AuthToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Kind      AuthTokenKind
	TokenHash []byte
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Account struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	Currency  money.Currency
	CreatedAt time.Time
	// BankSyncAccountID links this account to a woob-fetched account (see
	// BankSyncAccount.ID) so a sync can find-or-create instead of
	// duplicating it on every run. Nil for accounts created any other way
	// (CSV import, manual).
	BankSyncAccountID *string
}

// BankSyncAccount is a live snapshot fetched from an external bank
// aggregator (woob, via ports.BankSyncConnector) for one sync call — the
// raw shape a use-case in domain/services reconciles against Account (see
// services.SyncBankAccountsUseCase).
type BankSyncAccount struct {
	// ID is the aggregator's own account identifier (e.g. woob's account
	// id), not a uuid.UUID — matched against Account.BankSyncAccountID.
	ID         string
	Label      string
	Balance    money.Amount
	Currency   money.Currency
	Operations []BankSyncTransaction
}

// BankSyncTransaction is one transaction as woob returns it (already
// normalized to the same shape regardless of which bank it came from —
// unlike a CSV import, there's no per-bank format to map). Reconciled
// against entities.Operation the same way a CSV row is: ID (when
// non-empty) as RawReference, otherwise/also the dedup hash.
type BankSyncTransaction struct {
	ID     string
	Date   civil.Date
	Label  string
	Amount money.Amount
}

// BankSyncBank identifies which bank aggregator backend (woob module) a
// BankConnection talks to. Only BankSyncBankCreditMutuel is supported today
// - ConnectBankUseCase.Connect rejects anything else - kept as its own type
// rather than a bare string so adding a second bank later is a matter of
// adding a constant and a case, not hunting down string literals.
type BankSyncBank string

const BankSyncBankCreditMutuel BankSyncBank = "cm"

// BankConnection holds one user's link to a bank aggregator backend: the
// credentials needed to log in, and the session state (cookies + last 2FA
// validation) a sync reuses so it doesn't re-trigger an app-validation
// prompt every time. LoginEncrypted, PasswordEncrypted and
// SessionEncrypted are ciphertext (see ports.Encryptor /
// adapters/security.SecretBox) - a DB leak alone shouldn't hand out live
// bank access.
type BankConnection struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Bank   BankSyncBank

	LoginEncrypted    []byte
	PasswordEncrypted []byte
	// SessionEncrypted is nil until the first successful sync produces a
	// session worth saving.
	SessionEncrypted []byte

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ColumnMapping maps a bank's CSV column names to the fields a row needs.
type ColumnMapping struct {
	Date      string
	Label     string
	Amount    *string
	Debit     *string
	Credit    *string
	Reference *string
}

type BankProfile struct {
	ID uuid.UUID
	// UserID is nil for global profiles shared with every user, and set for
	// a custom profile owned by a single user.
	UserID           *uuid.UUID
	Name             string
	Delimiter        string
	Encoding         string
	DateFormat       string
	DecimalSeparator string
	AmountMode       AmountMode
	ColumnMapping    ColumnMapping
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ImportBatch struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	BankProfileID  uuid.UUID
	Filename       string
	ImportedAt     time.Time
	RowCount       int
	ImportedCount  int
	DuplicateCount int
	Errors         []string
}

type Operation struct {
	ID            uuid.UUID
	AccountID     uuid.UUID
	ImportBatchID *uuid.UUID
	Date          civil.Date
	Label         string
	Amount        money.Amount
	BalanceAfter  *money.Amount
	RawReference  *string
	DedupHash     string
	CreatedAt     time.Time
	CategoryID    *uuid.UUID
}

// OperationPageCursor identifies where a keyset-paginated operations page
// left off — the last row's sort key (date, created_at, id), matching
// ListPageForAccount's ORDER BY. Keyset (not OFFSET) pagination so a page
// stays cheap and stable however deep into the account's history it is, and
// however many operations get imported between page fetches.
type OperationPageCursor struct {
	Date      civil.Date
	CreatedAt time.Time
	ID        uuid.UUID
}

// OperationCategorySplit divides part of an Operation's amount into a
// category — used when a single transaction covers several categories (e.g.
// a supermarket receipt with both groceries and clothing). An operation with
// splits has these instead of (not in addition to) a plain CategoryID.
type OperationCategorySplit struct {
	ID          uuid.UUID
	OperationID uuid.UUID
	CategoryID  uuid.UUID
	Amount      money.Amount
}

type Comment struct {
	ID          uuid.UUID
	OperationID uuid.UUID
	UserID      uuid.UUID
	Body        string
	CreatedAt   time.Time
}

type PlannedItem struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	Kind      PlannedItemKind
	// Label is the match key when MatchMode is MatchModeLabel — nil for a
	// category-mode budget, which matches by CategoryID instead.
	Label           *string
	MatchMode       PlannedItemMatchMode
	Amount          money.Amount
	StartDate       civil.Date
	RecurrenceType  RecurrenceType
	OccurrenceCount *int
	IntervalWeeks   *int
	DayOfMonth      *int
	Active          bool
	Notes           *string
	// CategoryID does double duty: for a label-mode item it's applied to a
	// matched operation that doesn't already have one (see applyItemCategory);
	// for a category-mode item it's also the match key itself.
	CategoryID *uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type PlannedOccurrence struct {
	ID                    uuid.UUID
	PlannedItemID         uuid.UUID
	DueDate               civil.Date
	ExpectedAmount        money.Amount
	Status                OccurrenceStatus
	ReconciledOperationID *uuid.UUID
	ReconciledAmount      *money.Amount
}

// Category: UserID == nil marks a standard (shared, seeded) category;
// otherwise it's one a user created for themselves. ParentID is a
// self-referencing FK with no enforced depth.
type Category struct {
	ID        uuid.UUID
	UserID    *uuid.UUID
	ParentID  *uuid.UUID
	Name      string
	CreatedAt time.Time
}

// BalanceCheckpoint anchors PrevisionsUseCase's running balance so it
// doesn't have to sum every operation since the account's first import.
type BalanceCheckpoint struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	AsOfDate  civil.Date
	Balance   money.Amount
	Note      *string
	CreatedAt time.Time
}

type CollaboratorRole string

const (
	// CollaboratorRoleCollaborator: read everything the owner can see on
	// their accounts, plus categorize/split operations, comment, reconcile,
	// and create recurring items — everything short of importing statements,
	// managing bank profiles/categories, or editing a recurring item.
	CollaboratorRoleCollaborator CollaboratorRole = "collaborator"
)

// Collaborator represents one share link an owner issued to give another
// user standing access to every account they own (present and future) — not
// a single account, since the whole point is "give my father access to my
// budget" rather than picking accounts one at a time. Row lifecycle: created
// pending (CollaboratorUserID nil) with a random Token; redeeming the invite
// link sets CollaboratorUserID and AcceptedAt; the owner can RevokedAt it at
// any time, which immediately drops the grant everywhere access is checked.
type Collaborator struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
	// InvitedEmail is the address this invite was issued for (normalized:
	// trimmed, lowercased) — only a user whose own account email matches it
	// may redeem the invite, so a leaked or forwarded link can't grant
	// access to the wrong person.
	InvitedEmail string
	// CollaboratorUserID is nil until the invite is redeemed.
	CollaboratorUserID *uuid.UUID
	Role               CollaboratorRole
	Token              string
	CreatedAt          time.Time
	AcceptedAt         *time.Time
	RevokedAt          *time.Time
}
