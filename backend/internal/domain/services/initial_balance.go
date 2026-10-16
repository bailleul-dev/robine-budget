// Use-case: bootstrap an account's very first balance checkpoint from a
// balance the user knows to be true right now (e.g. "270€ today"), instead
// of asking them to figure out what their opening balance must have been.
// The checkpoint gets dated the day before the account's earliest imported
// operation and its amount walked backward across every operation since —
// so once it exists, PrevisionsUseCase needs no further retroactive
// correction, it's just an ordinary anchor at the true start of history.
package services

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

// ErrInitialBalanceAlreadySet: this is a one-time bootstrap for an account
// with no checkpoint yet — once one exists, further corrections go through
// the plain "add a checkpoint dated X" flow instead.
var ErrInitialBalanceAlreadySet = errors.New("this account already has a balance checkpoint")

type ComputeInitialBalanceUseCase struct {
	operations  ports.OperationRepository
	checkpoints ports.BalanceCheckpointRepository
}

func NewComputeInitialBalanceUseCase(
	operations ports.OperationRepository,
	checkpoints ports.BalanceCheckpointRepository,
) *ComputeInitialBalanceUseCase {
	return &ComputeInitialBalanceUseCase{operations: operations, checkpoints: checkpoints}
}

func (uc *ComputeInitialBalanceUseCase) Execute(
	ctx context.Context, accountID uuid.UUID, asOf civil.Date, knownBalance money.Amount, note *string,
) (*entities.BalanceCheckpoint, error) {
	existing, err := uc.checkpoints.ListForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return nil, ErrInitialBalanceAlreadySet
	}

	ops, err := uc.operations.ListForAccount(ctx, accountID, nil, nil)
	if err != nil {
		return nil, err
	}

	checkpointDate := asOf
	openingBalance := knownBalance
	if len(ops) > 0 {
		firstDate := ops[0].Date
		for _, op := range ops[1:] {
			if op.Date.Before(firstDate) {
				firstDate = op.Date
			}
		}
		// Undo every operation from the account's first one through the
		// date the known balance applies to, landing on what the balance
		// must have been the day before history starts. Operations after
		// asOf (a known balance dated in the past, say) are left alone —
		// they're none of this checkpoint's business.
		for _, op := range ops {
			if !op.Date.Before(firstDate) && !op.Date.After(asOf) {
				openingBalance = openingBalance.Sub(op.Amount)
			}
		}
		checkpointDate = firstDate.AddDays(-1)
	}

	return uc.checkpoints.Add(ctx, &entities.BalanceCheckpoint{
		ID: uuid.New(), AccountID: accountID, AsOfDate: checkpointDate, Balance: openingBalance, Note: note,
	})
}
