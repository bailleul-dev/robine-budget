package services

import (
	"context"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
)

// Use-case: a user's complete personal-data export (RGPD art. 15 and 20 —
// right of access and data portability). Everything the user *owns* is
// included; data belonging to other people who merely share an account with
// them is not. Secrets are never exported: no password hash, no stored bank
// credentials or session.

// AccountExport is one owned account with everything hanging off it.
type AccountExport struct {
	Account     entities.Account
	Checkpoints []entities.BalanceCheckpoint
	Operations  []entities.Operation
	Splits      map[uuid.UUID][]entities.OperationCategorySplit // by operation id
	Items       []ItemExport
	Imports     []entities.ImportBatch
}

// ItemExport is a recurring item with its occurrences.
type ItemExport struct {
	Item        entities.PlannedItem
	Occurrences []entities.PlannedOccurrence
}

// UserExport is the whole export.
type UserExport struct {
	User          entities.User
	Accounts      []AccountExport
	Categories    []entities.Category
	BankProfiles  []entities.BankProfile
	Comments      []entities.Comment
	Collaborators []entities.Collaborator
	BankConnected bool
	BankName      entities.BankSyncBank
}

type ExportUserDataUseCase struct {
	accounts      ports.AccountRepository
	checkpoints   ports.BalanceCheckpointRepository
	operations    ports.OperationRepository
	splits        ports.OperationCategorySplitRepository
	categories    ports.CategoryRepository
	plannedItems  ports.PlannedItemRepository
	occurrences   ports.PlannedOccurrenceRepository
	profiles      ports.BankProfileRepository
	comments      ports.CommentRepository
	collaborators ports.CollaboratorRepository
	imports       ports.ImportBatchRepository
	connections   ports.BankConnectionRepository
}

func NewExportUserDataUseCase(
	accounts ports.AccountRepository,
	checkpoints ports.BalanceCheckpointRepository,
	operations ports.OperationRepository,
	splits ports.OperationCategorySplitRepository,
	categories ports.CategoryRepository,
	plannedItems ports.PlannedItemRepository,
	occurrences ports.PlannedOccurrenceRepository,
	profiles ports.BankProfileRepository,
	comments ports.CommentRepository,
	collaborators ports.CollaboratorRepository,
	imports ports.ImportBatchRepository,
	connections ports.BankConnectionRepository,
) *ExportUserDataUseCase {
	return &ExportUserDataUseCase{accounts, checkpoints, operations, splits, categories, plannedItems, occurrences, profiles, comments, collaborators, imports, connections}
}

// splitChunk bounds how many operation ids go into one splits query.
const splitChunk = 5000

func (uc *ExportUserDataUseCase) Execute(ctx context.Context, user entities.User) (*UserExport, error) {
	out := &UserExport{User: user}

	accounts, err := uc.accounts.ListForUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		if account.UserID != user.ID {
			continue // shared with the user, not theirs
		}
		ae, err := uc.exportAccount(ctx, account)
		if err != nil {
			return nil, err
		}
		out.Accounts = append(out.Accounts, *ae)
	}

	visible, err := uc.categories.ListVisibleTo(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range visible {
		if c.UserID != nil && *c.UserID == user.ID {
			out.Categories = append(out.Categories, c)
		}
	}

	profiles, err := uc.profiles.ListForUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	for _, p := range profiles {
		if p.UserID != nil && *p.UserID == user.ID {
			out.BankProfiles = append(out.BankProfiles, p)
		}
	}

	if out.Comments, err = uc.comments.ListForAuthor(ctx, user.ID); err != nil {
		return nil, err
	}
	if out.Collaborators, err = uc.collaborators.ListForOwner(ctx, user.ID); err != nil {
		return nil, err
	}
	conn, err := uc.connections.GetByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if conn != nil {
		out.BankConnected = true
		out.BankName = conn.Bank
	}
	return out, nil
}

func (uc *ExportUserDataUseCase) exportAccount(ctx context.Context, account entities.Account) (*AccountExport, error) {
	ae := &AccountExport{Account: account, Splits: map[uuid.UUID][]entities.OperationCategorySplit{}}
	var err error
	if ae.Checkpoints, err = uc.checkpoints.ListForAccount(ctx, account.ID); err != nil {
		return nil, err
	}
	if ae.Operations, err = uc.operations.ListForAccount(ctx, account.ID, nil, nil); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(ae.Operations))
	for i, op := range ae.Operations {
		ids[i] = op.ID
	}
	for start := 0; start < len(ids); start += splitChunk {
		end := min(start+splitChunk, len(ids))
		splits, err := uc.splits.ListForOperationIDs(ctx, ids[start:end])
		if err != nil {
			return nil, err
		}
		for _, s := range splits {
			ae.Splits[s.OperationID] = append(ae.Splits[s.OperationID], s)
		}
	}
	items, err := uc.plannedItems.ListForAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		occurrences, err := uc.occurrences.ListForItem(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		ae.Items = append(ae.Items, ItemExport{Item: item, Occurrences: occurrences})
	}
	if ae.Imports, err = uc.imports.ListForAccount(ctx, account.ID); err != nil {
		return nil, err
	}
	return ae, nil
}
