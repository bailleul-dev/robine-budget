// Package banksync adapts the domain's ports.BankSyncConnector to the
// `bank_sync` Python CLI (repo root /bank_sync, see its cli.py for the
// contract). Today this shells out to a subprocess; kept behind the port
// interface so a future HTTP-based bank-sync service can replace it without
// touching callers.
package banksync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

// CLIConnector invokes `<PythonBin> -m bank_sync accounts` with Dir set to
// ProjectDir (the directory containing the bank_sync/ package) so the `-m`
// import resolves regardless of the server's own working directory, and
// CCM_IDENTIFIER/CCM_PASSWORD set explicitly in the subprocess's
// environment (not inherited) - each call carries the one user's own
// decrypted credentials, not a single global identity.
//
// The subprocess is treated as a port, the way an Erlang/Elixir port works:
// the session goes in on stdin, and the result - accounts plus the
// connector's updated session - comes back as a single JSON line on
// stdout. Nothing is exchanged through the filesystem; there is no
// storage path for the caller to manage or clean up.
type CLIConnector struct {
	PythonBin  string
	ProjectDir string
}

func NewCLIConnector(pythonBin, projectDir string) *CLIConnector {
	return &CLIConnector{PythonBin: pythonBin, ProjectDir: projectDir}
}

type cliTransaction struct {
	ID     string `json:"id"`
	Date   string `json:"date"`
	Label  string `json:"label"`
	Amount string `json:"amount"`
}

type cliAccount struct {
	ID         string           `json:"id"`
	Label      string           `json:"label"`
	Balance    string           `json:"balance"`
	Currency   string           `json:"currency"`
	Operations []cliTransaction `json:"operations"`
}

type cliResponse struct {
	Accounts []cliAccount `json:"accounts"`
	// Session is set on both success and (best-effort) failure - see
	// bank_sync/sync.py:fail, which reports whatever session state woob
	// left behind even when the call itself raised.
	Session string `json:"session"`
	Error   string `json:"error"`
	Detail  string `json:"detail"`
}

func (c *CLIConnector) FetchAccounts(ctx context.Context, login, password string, session []byte) ([]entities.BankSyncAccount, []byte, error) {
	cmd := exec.CommandContext(ctx, c.PythonBin, "-m", "bank_sync", "accounts")
	cmd.Dir = c.ProjectDir
	// Appended after the inherited environment: os/exec keeps only the last
	// value for a duplicate key, so these two win over anything else named
	// CCM_IDENTIFIER/CCM_PASSWORD in the server's own environment.
	cmd.Env = append(os.Environ(), "CCM_IDENTIFIER="+login, "CCM_PASSWORD="+password)
	cmd.Stdin = bytes.NewReader(session)
	// Progress/2FA messages ("confirme sur ton appli mobile...") go to
	// stderr by convention (see bank_sync/cli.py) - forward them to the
	// server's own logs so whoever triggered the sync knows to check their
	// phone, instead of the caller just hanging with no explanation.
	cmd.Stderr = os.Stderr

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	runErr := cmd.Run()

	var resp cliResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		if runErr != nil {
			return nil, nil, fmt.Errorf("bank_sync: %w", runErr)
		}
		return nil, nil, fmt.Errorf("bank_sync: invalid output: %w", err)
	}

	var newSession []byte
	if resp.Session != "" {
		newSession = []byte(resp.Session)
	}

	if resp.Error != "" {
		return nil, newSession, fmt.Errorf("bank_sync: %s: %s", resp.Error, resp.Detail)
	}
	if runErr != nil {
		return nil, newSession, fmt.Errorf("bank_sync: %w", runErr)
	}

	out := make([]entities.BankSyncAccount, len(resp.Accounts))
	for i, a := range resp.Accounts {
		balance, err := money.New(a.Balance)
		if err != nil {
			return nil, newSession, fmt.Errorf("bank_sync: account %s: %w", a.ID, err)
		}
		currency, err := money.ParseCurrency(a.Currency)
		if err != nil {
			return nil, newSession, fmt.Errorf("bank_sync: account %s: %w", a.ID, err)
		}
		operations := make([]entities.BankSyncTransaction, len(a.Operations))
		for j, t := range a.Operations {
			date, err := civil.Parse(t.Date)
			if err != nil {
				return nil, newSession, fmt.Errorf("bank_sync: account %s transaction %s: %w", a.ID, t.ID, err)
			}
			amount, err := money.New(t.Amount)
			if err != nil {
				return nil, newSession, fmt.Errorf("bank_sync: account %s transaction %s: %w", a.ID, t.ID, err)
			}
			operations[j] = entities.BankSyncTransaction{ID: t.ID, Date: date, Label: t.Label, Amount: amount}
		}
		out[i] = entities.BankSyncAccount{ID: a.ID, Label: a.Label, Balance: balance, Currency: currency, Operations: operations}
	}
	return out, newSession, nil
}
