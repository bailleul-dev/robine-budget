// Use-case: import a bank CSV export into Operations, deduplicating as we
// go. Mirrors backend/app/domain/services/csv_import.py.
package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

// decodeCSVBytes mirrors Python's `file_bytes.decode(bank_profile.encoding)`:
// some banks (e.g. Crédit Mutuel) export CSV as ISO-8859-1/Windows-1252
// rather than UTF-8, and encoding/csv requires valid UTF-8 input.
func decodeCSVBytes(b []byte, encodingName string) (string, error) {
	var enc encoding.Encoding
	switch strings.ToLower(strings.TrimSpace(encodingName)) {
	case "", "utf-8", "utf8":
		return string(b), nil
	case "iso-8859-1", "iso8859-1", "latin-1", "latin1":
		enc = charmap.ISO8859_1
	case "windows-1252", "cp1252":
		enc = charmap.Windows1252
	default:
		return string(b), nil
	}
	decoded, err := enc.NewDecoder().Bytes(b)
	if err != nil {
		return "", fmt.Errorf("decode csv as %s: %w", encodingName, err)
	}
	return string(decoded), nil
}

type rowParseError struct{ reason string }

func (e rowParseError) Error() string { return e.reason }

func parseAmount(raw string, decimalSeparator string) (money.Amount, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.ReplaceAll(cleaned, "€", "")
	cleaned = strings.ReplaceAll(cleaned, "$", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "") // non-breaking space
	if cleaned == "" {
		return money.Amount{}, rowParseError{"empty amount"}
	}
	if decimalSeparator == "," {
		cleaned = strings.ReplaceAll(cleaned, ".", "")
		cleaned = strings.ReplaceAll(cleaned, ",", ".")
	} else {
		cleaned = strings.ReplaceAll(cleaned, ",", "")
	}
	amount, err := money.New(cleaned)
	if err != nil {
		return money.Amount{}, rowParseError{fmt.Sprintf("invalid amount %q", raw)}
	}
	return amount, nil
}

// goDateLayout converts a handful of common Python strftime tokens (as used
// by BankProfile.date_format, e.g. "%d/%m/%Y") to Go's reference-time layout.
func goDateLayout(pyFormat string) string {
	replacer := strings.NewReplacer(
		"%Y", "2006", "%y", "06",
		"%m", "01", "%d", "02",
		"%H", "15", "%M", "04", "%S", "05",
	)
	return replacer.Replace(pyFormat)
}

type ImportCsvUseCase struct {
	operations    ports.OperationRepository
	importBatches ports.ImportBatchRepository
	clock         ports.Clock
}

func NewImportCsvUseCase(operations ports.OperationRepository, importBatches ports.ImportBatchRepository, clock ports.Clock) *ImportCsvUseCase {
	return &ImportCsvUseCase{operations: operations, importBatches: importBatches, clock: clock}
}

func (uc *ImportCsvUseCase) Execute(
	ctx context.Context,
	accountID uuid.UUID,
	profile entities.BankProfile,
	filename string,
	fileBytes []byte,
) (*entities.ImportBatch, error) {
	text, err := decodeCSVBytes(fileBytes, profile.Encoding)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(text))
	if profile.Delimiter != "" {
		reader.Comma = rune(profile.Delimiter[0])
	}
	reader.FieldsPerRecord = -1 // tolerate ragged rows the same way csv.DictReader does per-row

	header, err := reader.Read()
	if err != nil {
		// Empty file: nothing to import, not a hard failure.
		batch, addErr := uc.saveBatch(ctx, accountID, profile.ID, filename, 0, 0, nil, nil)
		return batch, addErr
	}
	colIndex := map[string]int{}
	for i, name := range header {
		colIndex[strings.TrimSpace(name)] = i
	}
	dateLayout := goDateLayout(profile.DateFormat)

	rowCount := 0
	duplicateCount := 0
	var errs []string
	var newOperations []entities.Operation
	batchID := uuid.New()
	seenReferences := map[string]bool{}
	seenDedupHashes := map[string]bool{}

	get := func(row []string, col string) (string, bool) {
		idx, ok := colIndex[col]
		if !ok || idx >= len(row) {
			return "", false
		}
		return row[idx], true
	}

	for {
		row, readErr := reader.Read()
		if readErr != nil {
			break
		}
		rowCount++
		rowNumber := rowCount

		dateRaw, ok := get(row, profile.ColumnMapping.Date)
		if !ok {
			errs = append(errs, fmt.Sprintf("row %d: missing column %q", rowNumber, profile.ColumnMapping.Date))
			continue
		}
		parsedDate, err := time.Parse(dateLayout, strings.TrimSpace(dateRaw))
		if err != nil {
			errs = append(errs, fmt.Sprintf("row %d: invalid date %q", rowNumber, dateRaw))
			continue
		}
		operationDate := civil.FromTime(parsedDate)

		labelRaw, ok := get(row, profile.ColumnMapping.Label)
		if !ok {
			errs = append(errs, fmt.Sprintf("row %d: missing column %q", rowNumber, profile.ColumnMapping.Label))
			continue
		}
		label := strings.TrimSpace(labelRaw)

		var amount money.Amount
		var amountErr error
		switch profile.AmountMode {
		case entities.AmountModeDebitCredit, entities.AmountModeDebitCreditSigned:
			debitRaw := ""
			if profile.ColumnMapping.Debit != nil {
				debitRaw, _ = get(row, *profile.ColumnMapping.Debit)
			}
			creditRaw := ""
			if profile.ColumnMapping.Credit != nil {
				creditRaw, _ = get(row, *profile.ColumnMapping.Credit)
			}
			debit := money.Zero()
			credit := money.Zero()
			if strings.TrimSpace(debitRaw) != "" {
				debit, amountErr = parseAmount(debitRaw, profile.DecimalSeparator)
			}
			if amountErr == nil && strings.TrimSpace(creditRaw) != "" {
				credit, amountErr = parseAmount(creditRaw, profile.DecimalSeparator)
			}
			if amountErr == nil {
				if profile.AmountMode == entities.AmountModeDebitCredit {
					amount = credit.Sub(debit) // debit is a positive magnitude (money out)
				} else {
					amount = debit.Add(credit) // both already signed; only one populated per row
				}
			}
		case entities.AmountModeSingle:
			amountRaw := ""
			if profile.ColumnMapping.Amount != nil {
				amountRaw, _ = get(row, *profile.ColumnMapping.Amount)
			}
			amount, amountErr = parseAmount(amountRaw, profile.DecimalSeparator)
		default:
			amountErr = fmt.Errorf("unknown amount mode %q", profile.AmountMode)
		}
		if amountErr != nil {
			errs = append(errs, fmt.Sprintf("row %d: %s", rowNumber, amountErr))
			continue
		}

		var rawReference *string
		if profile.ColumnMapping.Reference != nil {
			if ref, ok := get(row, *profile.ColumnMapping.Reference); ok {
				ref = strings.TrimSpace(ref)
				if ref != "" {
					rawReference = &ref
				}
			}
		}

		if rawReference != nil {
			if seenReferences[*rawReference] {
				duplicateCount++
				continue
			}
			existing, err := uc.operations.FindByReference(ctx, accountID, *rawReference)
			if err != nil {
				return nil, err
			}
			if existing != nil {
				duplicateCount++
				continue
			}
		}

		dedupHash := ComputeDedupHash(accountID, operationDate, amount, label)
		if seenDedupHashes[dedupHash] {
			duplicateCount++
			continue
		}
		existing, err := uc.operations.FindByDedupHash(ctx, accountID, dedupHash)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			duplicateCount++
			continue
		}

		if rawReference != nil {
			seenReferences[*rawReference] = true
		}
		seenDedupHashes[dedupHash] = true

		batchIDCopy := batchID
		newOperations = append(newOperations, entities.Operation{
			ID:            uuid.New(),
			AccountID:     accountID,
			ImportBatchID: &batchIDCopy,
			Date:          operationDate,
			Label:         label,
			Amount:        amount,
			RawReference:  rawReference,
			DedupHash:     dedupHash,
			CreatedAt:     uc.clock.Now(),
		})
	}

	return uc.saveBatch(ctx, accountID, profile.ID, filename, rowCount, duplicateCount, errs, newOperations, batchID)
}

func (uc *ImportCsvUseCase) saveBatch(
	ctx context.Context,
	accountID, bankProfileID uuid.UUID,
	filename string,
	rowCount, duplicateCount int,
	errs []string,
	newOperations []entities.Operation,
	batchID ...uuid.UUID,
) (*entities.ImportBatch, error) {
	id := uuid.New()
	if len(batchID) > 0 {
		id = batchID[0]
	}
	// Batch row must exist before operations that FK-reference it.
	batch := &entities.ImportBatch{
		ID:             id,
		AccountID:      accountID,
		BankProfileID:  bankProfileID,
		Filename:       filename,
		ImportedAt:     uc.clock.Now(),
		RowCount:       rowCount,
		ImportedCount:  len(newOperations),
		DuplicateCount: duplicateCount,
		Errors:         errs,
	}
	saved, err := uc.importBatches.Add(ctx, batch)
	if err != nil {
		return nil, err
	}
	if len(newOperations) > 0 {
		if _, err := uc.operations.AddMany(ctx, newOperations); err != nil {
			return nil, err
		}
	}
	return saved, nil
}
