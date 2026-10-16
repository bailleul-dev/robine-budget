// Use-cases: connecting a user's bank (woob-backed, Credit Mutuel for now)
// and syncing it. Split in two because both the first-ever sync (right
// after connecting) and every later sync run the exact same reconciliation
// logic - ConnectBankUseCase just adds the "save credentials" step in front
// of it and delegates the rest to SyncBankAccountsUseCase.
package services

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
)

// BankSyncError wraps a failure that originated in the bank connector
// itself (wrong credentials, app-validation refused/expired, already
// connected) - safe to show to the end user. Any other error returned by
// these use-cases is an infrastructure failure (DB down, etc.) that callers
// should treat as opaque.
type BankSyncError struct{ err error }

func (e *BankSyncError) Error() string { return e.err.Error() }
func (e *BankSyncError) Unwrap() error { return e.err }

// redactSecrets replaces any occurrence of the given secrets in msg with a
// placeholder. Second line of defense: bank_sync (see bank_sync/config.py
// redact()) already redacts credentials out of anything it emits, but a
// plaintext login/password must never reach an HTTP response even if that
// redaction were ever bypassed, changed, or a future caller forgot it -
// this runs again, independently, right where a *BankSyncError (the only
// error type this package puts into an HTTP response body) gets built from
// connector output.
func redactSecrets(msg string, secrets ...string) string {
	for _, s := range secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "***REDACTED***")
		}
	}
	return msg
}

// SyncedAccount is one account a sync reconciled: the persisted Account row
// (created or pre-existing), the balance just recorded for it, and how many
// new transactions were saved as Operations.
type SyncedAccount struct {
	Account         entities.Account
	Balance         entities.BankSyncAccount
	Created         bool
	OperationsAdded int
}

// SyncBankAccountsUseCase pulls a live snapshot from the bank connector for
// a user's existing connection and reconciles it: creates any account the
// connector reports that isn't linked to one yet, records each account's
// balance as of today, saves any new transaction as an Operation
// (deduplicated exactly like ImportCsvUseCase - woob already normalizes
// every bank to the same Transaction shape, so there's no per-bank
// format/mapping to apply here, unlike CSV import), and persists the
// connector's updated session state (cookies / 2FA validation) if it
// changed - so the next sync doesn't re-trigger an app-validation prompt.
type SyncBankAccountsUseCase struct {
	connections    ports.BankConnectionRepository
	accounts       ports.AccountRepository
	checkpoints    ports.BalanceCheckpointRepository
	operations     ports.OperationRepository
	connector      ports.BankSyncConnector
	encryptor      ports.Encryptor
	clock          ports.Clock
	autoCategorize *AutoCategorizeUseCase
}

func NewSyncBankAccountsUseCase(
	connections ports.BankConnectionRepository,
	accounts ports.AccountRepository,
	checkpoints ports.BalanceCheckpointRepository,
	operations ports.OperationRepository,
	connector ports.BankSyncConnector,
	encryptor ports.Encryptor,
	clock ports.Clock,
	autoCategorize *AutoCategorizeUseCase,
) *SyncBankAccountsUseCase {
	return &SyncBankAccountsUseCase{
		connections:    connections,
		accounts:       accounts,
		checkpoints:    checkpoints,
		operations:     operations,
		connector:      connector,
		encryptor:      encryptor,
		clock:          clock,
		autoCategorize: autoCategorize,
	}
}

func (uc *SyncBankAccountsUseCase) Sync(ctx context.Context, userID uuid.UUID) ([]SyncedAccount, error) {
	conn, err := uc.connections.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, &BankSyncError{fmt.Errorf("no bank connection for this user")}
	}

	login, err := uc.encryptor.Decrypt(conn.LoginEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt login: %w", err)
	}
	password, err := uc.encryptor.Decrypt(conn.PasswordEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt password: %w", err)
	}
	var sessionBefore []byte
	if conn.SessionEncrypted != nil {
		sessionBefore, err = uc.encryptor.Decrypt(conn.SessionEncrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt session: %w", err)
		}
	}

	snapshots, sessionAfter, fetchErr := uc.connector.FetchAccounts(ctx, string(login), string(password), sessionBefore)

	// Persist whatever session state the connector returned even if the
	// fetch itself failed (e.g. app-validation resolved but a later step
	// errored) - it's still progress worth keeping for the next attempt.
	if len(sessionAfter) > 0 && !bytes.Equal(sessionAfter, sessionBefore) {
		if encrypted, encErr := uc.encryptor.Encrypt(sessionAfter); encErr == nil {
			_ = uc.connections.UpdateSession(ctx, conn.ID, encrypted)
		}
	}

	if fetchErr != nil {
		return nil, &BankSyncError{fmt.Errorf("%s", redactSecrets(fetchErr.Error(), string(login), string(password)))}
	}

	today := uc.clock.Today()
	out := make([]SyncedAccount, 0, len(snapshots))
	for _, snap := range snapshots {
		account, created, err := uc.findOrCreateAccount(ctx, userID, snap)
		if err != nil {
			return nil, err
		}
		note := "Synced automatically via bank_sync"
		if _, err := uc.checkpoints.Upsert(ctx, &entities.BalanceCheckpoint{
			ID:        uuid.New(),
			AccountID: account.ID,
			AsOfDate:  today,
			Balance:   snap.Balance,
			Note:      &note,
		}); err != nil {
			return nil, err
		}
		operationsAdded, err := uc.syncOperations(ctx, account.ID, snap.Operations)
		if err != nil {
			return nil, err
		}
		// Best-effort: every newly-synced operation that isn't already
		// categorized (via a matching recurring item, handled elsewhere)
		// gets a shot at the merchant dictionary / exact-label memory — see
		// AutoCategorizeUseCase. Runs here rather than only from the manual
		// "Sync" HTTP handler so the daily cron (core.RunDailyBankSync)
		// benefits the same way.
		if operationsAdded > 0 {
			if _, err := uc.autoCategorize.CategorizeAccount(ctx, account.ID, userID); err != nil {
				return nil, err
			}
		}
		out = append(out, SyncedAccount{Account: *account, Balance: snap, Created: created, OperationsAdded: operationsAdded})
	}
	return out, nil
}

// syncOperations saves each transaction woob returned that isn't already
// recorded for accountID, and reports how many were new. Deduplicates the
// same way ImportCsvUseCase does: by RawReference when woob gave one (its
// transaction id - generally more reliable than a CSV reference column),
// and always by dedup hash (account + date + amount + normalized label) as
// a fallback/second check, including against duplicates within this same
// batch (woob returning the same transaction twice in one page, or two
// consecutive syncs overlapping their date range).
func (uc *SyncBankAccountsUseCase) syncOperations(ctx context.Context, accountID uuid.UUID, transactions []entities.BankSyncTransaction) (int, error) {
	seenReferences := map[string]bool{}
	seenDedupHashes := map[string]bool{}
	var newOperations []entities.Operation

	for _, txn := range transactions {
		if txn.ID != "" {
			if seenReferences[txn.ID] {
				continue
			}
			existing, err := uc.operations.FindByReference(ctx, accountID, txn.ID)
			if err != nil {
				return 0, err
			}
			if existing != nil {
				continue
			}
		}

		dedupHash := ComputeDedupHash(accountID, txn.Date, txn.Amount, txn.Label)
		if seenDedupHashes[dedupHash] {
			continue
		}
		existing, err := uc.operations.FindByDedupHash(ctx, accountID, dedupHash)
		if err != nil {
			return 0, err
		}
		if existing != nil {
			continue
		}

		var rawReference *string
		if txn.ID != "" {
			id := txn.ID
			rawReference = &id
			seenReferences[txn.ID] = true
		}
		seenDedupHashes[dedupHash] = true

		newOperations = append(newOperations, entities.Operation{
			ID:           uuid.New(),
			AccountID:    accountID,
			Date:         txn.Date,
			Label:        txn.Label,
			Amount:       txn.Amount,
			RawReference: rawReference,
			DedupHash:    dedupHash,
			CreatedAt:    uc.clock.Now(),
		})
	}

	if len(newOperations) == 0 {
		return 0, nil
	}
	if _, err := uc.operations.AddMany(ctx, newOperations); err != nil {
		return 0, err
	}
	return len(newOperations), nil
}

func (uc *SyncBankAccountsUseCase) findOrCreateAccount(ctx context.Context, userID uuid.UUID, snap entities.BankSyncAccount) (*entities.Account, bool, error) {
	existing, err := uc.accounts.GetByBankSyncAccountID(ctx, userID, snap.ID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}
	bankSyncAccountID := snap.ID
	created, err := uc.accounts.Add(ctx, &entities.Account{
		ID:                uuid.New(),
		UserID:            userID,
		Name:              snap.Label,
		Currency:          snap.Currency,
		BankSyncAccountID: &bankSyncAccountID,
	})
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// SyncAllResult is one user's outcome from SyncAllBankConnectionsUseCase -
// Err is set instead of aborting the run so one user's bad credentials or
// an expired 2FA session doesn't stop every other user's daily sync.
type SyncAllResult struct {
	UserID   uuid.UUID
	Accounts []SyncedAccount
	Err      error
}

// SyncAllBankConnectionsUseCase re-syncs every connected user's bank in
// turn - the daily cron's entry point (see core.RunDailyBankSync). Reuses
// SyncBankAccountsUseCase.Sync exactly as the manual "Sync" button does, so
// results are always consistent between an automatic and a user-triggered
// sync.
type SyncAllBankConnectionsUseCase struct {
	connections ports.BankConnectionRepository
	sync        *SyncBankAccountsUseCase
}

func NewSyncAllBankConnectionsUseCase(connections ports.BankConnectionRepository, sync *SyncBankAccountsUseCase) *SyncAllBankConnectionsUseCase {
	return &SyncAllBankConnectionsUseCase{connections: connections, sync: sync}
}

func (uc *SyncAllBankConnectionsUseCase) SyncAll(ctx context.Context) ([]SyncAllResult, error) {
	userIDs, err := uc.connections.ListUserIDs(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]SyncAllResult, 0, len(userIDs))
	for _, userID := range userIDs {
		accounts, err := uc.sync.Sync(ctx, userID)
		results = append(results, SyncAllResult{UserID: userID, Accounts: accounts, Err: err})
	}
	return results, nil
}

// BankConnectionStatus reports whether the current user has already linked
// a bank, and which one — lets a client decide whether to show the
// "connect" form (login/password) or a plain "sync" action, instead of
// having to infer it from a failed Connect/Sync call's error message.
type BankConnectionStatus struct {
	Connected bool
	Bank      entities.BankSyncBank
}

type BankConnectionStatusUseCase struct {
	connections ports.BankConnectionRepository
}

func NewBankConnectionStatusUseCase(connections ports.BankConnectionRepository) *BankConnectionStatusUseCase {
	return &BankConnectionStatusUseCase{connections: connections}
}

func (uc *BankConnectionStatusUseCase) Status(ctx context.Context, userID uuid.UUID) (BankConnectionStatus, error) {
	conn, err := uc.connections.GetByUserID(ctx, userID)
	if err != nil {
		return BankConnectionStatus{}, err
	}
	if conn == nil {
		return BankConnectionStatus{}, nil
	}
	return BankConnectionStatus{Connected: true, Bank: conn.Bank}, nil
}

// ConnectBankUseCase is the one-time "link my bank account" flow: encrypts
// and stores the given credentials, then runs the exact same sync
// SyncBankAccountsUseCase runs afterwards - which is also where the initial
// app-validation (2FA) happens (the connector blocks on it) and the
// resulting session gets saved for next time.
type ConnectBankUseCase struct {
	connections ports.BankConnectionRepository
	encryptor   ports.Encryptor
	sync        *SyncBankAccountsUseCase
}

func NewConnectBankUseCase(connections ports.BankConnectionRepository, encryptor ports.Encryptor, sync *SyncBankAccountsUseCase) *ConnectBankUseCase {
	return &ConnectBankUseCase{connections: connections, encryptor: encryptor, sync: sync}
}

func (uc *ConnectBankUseCase) Connect(ctx context.Context, userID uuid.UUID, bank entities.BankSyncBank, login, password string) ([]SyncedAccount, error) {
	// Only Credit Mutuel today - the field exists on the request/entity
	// from the start so adding a second bank later doesn't need a schema
	// or API change, just a new case here.
	if bank != entities.BankSyncBankCreditMutuel {
		return nil, &BankSyncError{fmt.Errorf("unsupported bank %q (only %q is supported for now)", bank, entities.BankSyncBankCreditMutuel)}
	}

	existing, err := uc.connections.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, &BankSyncError{fmt.Errorf("bank already connected for this user")}
	}

	loginEnc, err := uc.encryptor.Encrypt([]byte(login))
	if err != nil {
		return nil, fmt.Errorf("encrypt login: %w", err)
	}
	passwordEnc, err := uc.encryptor.Encrypt([]byte(password))
	if err != nil {
		return nil, fmt.Errorf("encrypt password: %w", err)
	}

	if _, err := uc.connections.Add(ctx, &entities.BankConnection{
		ID:                uuid.New(),
		UserID:            userID,
		Bank:              bank,
		LoginEncrypted:    loginEnc,
		PasswordEncrypted: passwordEnc,
	}); err != nil {
		return nil, err
	}

	// If this first sync fails (e.g. app-validation not confirmed in time),
	// the connection row stays - the user retries with Sync rather than
	// having to hand over their credentials again.
	return uc.sync.Sync(ctx, userID)
}
