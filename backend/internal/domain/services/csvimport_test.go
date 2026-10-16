package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time    { return c.now }
func (c fixedClock) Today() civil.Date { return civil.FromTime(c.now) }

func TestDecodeCSVBytes_ISO88591(t *testing.T) {
	// 0xE9 is 'é' in both ISO-8859-1 and UTF-8's "Libell\xe9" mis-encoding.
	raw := []byte("Libell\xe9;D\xe9bit;Cr\xe9dit")
	got, err := decodeCSVBytes(raw, "iso-8859-1")
	if err != nil {
		t.Fatalf("decodeCSVBytes: %v", err)
	}
	want := "Libellé;Débit;Crédit"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestImportCsv_CreditMutuelProfile_DebitCreditSignedWithLatin1Encoding(t *testing.T) {
	operations := newFakeOperationRepo()
	batches := newFakeImportBatchRepo()
	uc := NewImportCsvUseCase(operations, batches, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})

	debit := "Débit"
	credit := "Crédit"
	profile := entities.BankProfile{
		ID:               uuid.New(),
		Name:             "Crédit Mutuel",
		Delimiter:        ";",
		Encoding:         "iso-8859-1",
		DateFormat:       "%d/%m/%Y",
		DecimalSeparator: ",",
		AmountMode:       entities.AmountModeDebitCreditSigned,
		ColumnMapping: entities.ColumnMapping{
			Date:   "Date",
			Label:  "Libellé",
			Debit:  &debit,
			Credit: &credit,
		},
	}
	accountID := uuid.New()
	// Real Crédit Mutuel export shape: semicolon-delimited, Latin-1 encoded,
	// already-signed debit/credit columns (only one populated per row).
	csvBytes := []byte(
		"Date;Libell\xe9;D\xe9bit;Cr\xe9dit\r\n" +
			"11/09/2026;VIR SFEIR SAS;;181,64\r\n" +
			"10/09/2026;ASSURANCE HABITATION;-51,18;\r\n",
	)

	batch, err := uc.Execute(context.Background(), accountID, profile, "export.csv", csvBytes)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if batch.ImportedCount != 2 {
		t.Fatalf("ImportedCount = %d, want 2", batch.ImportedCount)
	}

	ops, err := operations.ListForAccount(context.Background(), accountID, nil, nil)
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("len(ops) = %d, want 2", len(ops))
	}
	byLabel := map[string]entities.Operation{}
	for _, op := range ops {
		byLabel[op.Label] = op
	}
	credited, ok := byLabel["VIR SFEIR SAS"]
	if !ok {
		t.Fatalf("missing operation with decoded label %q", "VIR SFEIR SAS")
	}
	if credited.Amount.String() != "181.64" {
		t.Fatalf("credited amount = %s, want 181.64", credited.Amount.String())
	}
	debited, ok := byLabel["ASSURANCE HABITATION"]
	if !ok {
		t.Fatalf("missing operation with decoded label %q", "ASSURANCE HABITATION")
	}
	if debited.Amount.String() != "-51.18" {
		t.Fatalf("debited amount = %s, want -51.18", debited.Amount.String())
	}
}
