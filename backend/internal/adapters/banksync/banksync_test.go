package banksync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bailleul.dev/budget-server/pkg/money"
)

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	amt, err := money.New(s)
	if err != nil {
		t.Fatalf("money.New(%q): %v", s, err)
	}
	return amt.Decimal
}

// newFakeScript writes a small shell script standing in for the real
// `python -m bank_sync accounts` subprocess — CLIConnector only cares about
// an executable at PythonBin that reads stdin and writes one JSON line to
// stdout, so a shell script exercises the exact same contract without any
// Python/woob dependency in the test.
func newFakeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake_bank_sync.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}
	return path
}

func TestCLIConnector_FetchAccounts_Success(t *testing.T) {
	script := newFakeScript(t, `cat <<'EOF'
{"accounts":[{"id":"acc1","label":"Compte Courant","balance":"1234.56","currency":"EUR","operations":[{"id":"tx1","date":"2026-01-15","label":"Some label","amount":"-10.00"}]}],"session":"new-session-data"}
EOF`)
	c := NewCLIConnector(script, t.TempDir())

	accounts, session, err := c.FetchAccounts(context.Background(), "user@example.com", "hunter2", []byte("old-session"))
	if err != nil {
		t.Fatalf("FetchAccounts: %v", err)
	}
	if string(session) != "new-session-data" {
		t.Errorf("session = %q, want new-session-data", session)
	}
	if len(accounts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(accounts))
	}
	acc := accounts[0]
	if acc.ID != "acc1" || acc.Label != "Compte Courant" || acc.Currency != "EUR" {
		t.Errorf("account = %+v, unexpected", acc)
	}
	if !acc.Balance.Decimal.Equal(mustDecimal(t, "1234.56")) {
		t.Errorf("balance = %s, want 1234.56", acc.Balance.Decimal)
	}
	if len(acc.Operations) != 1 {
		t.Fatalf("got %d operations, want 1", len(acc.Operations))
	}
	op := acc.Operations[0]
	if op.ID != "tx1" || op.Label != "Some label" {
		t.Errorf("operation = %+v, unexpected", op)
	}
	if op.Date.String() != "2026-01-15" {
		t.Errorf("Date = %s, want 2026-01-15", op.Date)
	}
	if !op.Amount.Decimal.Equal(mustDecimal(t, "-10.00")) {
		t.Errorf("Amount = %s, want -10.00", op.Amount.Decimal)
	}
}

// The subprocess's credentials come in through its environment, not
// arguments or stdin — assert the exact env vars the caller passed in are
// what actually reaches the process, by having the script echo them back.
func TestCLIConnector_FetchAccounts_PassesCredentialsViaEnv(t *testing.T) {
	script := newFakeScript(t, `cat <<EOF
{"accounts":[{"id":"acc1","label":"$CCM_IDENTIFIER:$CCM_PASSWORD","balance":"0.00","currency":"EUR","operations":[]}],"session":""}
EOF`)
	c := NewCLIConnector(script, t.TempDir())

	accounts, _, err := c.FetchAccounts(context.Background(), "alice", "s3cret", nil)
	if err != nil {
		t.Fatalf("FetchAccounts: %v", err)
	}
	if got, want := accounts[0].Label, "alice:s3cret"; got != want {
		t.Errorf("label (echoing env) = %q, want %q", got, want)
	}
}

func TestCLIConnector_FetchAccounts_ErrorField(t *testing.T) {
	script := newFakeScript(t, `cat <<'EOF'
{"error":"auth_failed","detail":"bad credentials","session":"partial-session"}
EOF`)
	c := NewCLIConnector(script, t.TempDir())

	accounts, session, err := c.FetchAccounts(context.Background(), "user", "pass", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "auth_failed") || !strings.Contains(err.Error(), "bad credentials") {
		t.Errorf("error = %v, want it to mention auth_failed and bad credentials", err)
	}
	if accounts != nil {
		t.Errorf("accounts = %v, want nil on error", accounts)
	}
	// Even on failure, whatever session state woob left behind is still
	// reported back — see bank_sync/sync.py:fail referenced in the source.
	if string(session) != "partial-session" {
		t.Errorf("session = %q, want partial-session even on failure", session)
	}
}

func TestCLIConnector_FetchAccounts_InvalidJSONOutput(t *testing.T) {
	script := newFakeScript(t, `echo 'not valid json'`)
	c := NewCLIConnector(script, t.TempDir())

	_, _, err := c.FetchAccounts(context.Background(), "user", "pass", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid output") {
		t.Errorf("error = %v, want it to mention invalid output", err)
	}
}

func TestCLIConnector_FetchAccounts_NonZeroExitNoOutput(t *testing.T) {
	script := newFakeScript(t, `exit 7`)
	c := NewCLIConnector(script, t.TempDir())

	_, _, err := c.FetchAccounts(context.Background(), "user", "pass", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "bank_sync") {
		t.Errorf("error = %v, want it wrapped with the bank_sync: prefix", err)
	}
}

func TestCLIConnector_FetchAccounts_InvalidBalance(t *testing.T) {
	script := newFakeScript(t, `cat <<'EOF'
{"accounts":[{"id":"acc1","label":"X","balance":"not-a-number","currency":"EUR","operations":[]}]}
EOF`)
	c := NewCLIConnector(script, t.TempDir())

	_, _, err := c.FetchAccounts(context.Background(), "user", "pass", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "acc1") {
		t.Errorf("error = %v, want it to name the offending account", err)
	}
}

func TestCLIConnector_FetchAccounts_InvalidOperationDate(t *testing.T) {
	script := newFakeScript(t, `cat <<'EOF'
{"accounts":[{"id":"acc1","label":"X","balance":"10.00","currency":"EUR","operations":[{"id":"tx1","date":"not-a-date","label":"L","amount":"1.00"}]}]}
EOF`)
	c := NewCLIConnector(script, t.TempDir())

	_, _, err := c.FetchAccounts(context.Background(), "user", "pass", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "tx1") {
		t.Errorf("error = %v, want it to name the offending transaction", err)
	}
}

func TestCLIConnector_FetchAccounts_ContextCancelled(t *testing.T) {
	// A script that outlives the deadline it's given — the subprocess must
	// actually be killed rather than the call hanging forever.
	script := newFakeScript(t, `sleep 5`)
	c := NewCLIConnector(script, t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, _, err := c.FetchAccounts(ctx, "user", "pass", nil)
	if err == nil {
		t.Fatal("expected an error from a cancelled context, got nil")
	}
}

func TestCLIConnector_FetchAccounts_EmptyAccountList(t *testing.T) {
	script := newFakeScript(t, `cat <<'EOF'
{"accounts":[],"session":"sess"}
EOF`)
	c := NewCLIConnector(script, t.TempDir())

	accounts, _, err := c.FetchAccounts(context.Background(), "user", "pass", nil)
	if err != nil {
		t.Fatalf("FetchAccounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("got %d accounts, want 0", len(accounts))
	}
}
