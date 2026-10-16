package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/httpx"
)

// NewMCPHandler exposes a Model Context Protocol server (Streamable HTTP)
// so an AI assistant can list accounts and operations, categorize them and
// trigger a bank sync on the signed-in user's behalf. It sits behind
// MCPAuthMiddleware — clients sign in through the OAuth flow in
// oauth_handlers.go — and every tool goes through the same access checks
// as its REST counterpart, so a collaborator gets exactly the scope they'd
// have in the app.
//
// Stateless: each POST builds a fresh server bound to that request's user,
// so there's no MCP session to leak across users or to expire separately
// from the auth token.
func NewMCPHandler(accounts *AccountHandler, categories *CategoryHandler, operations *OperationHandler, bankSync *BankSyncHandler) http.Handler {
	tools := &mcpTools{accounts: accounts, categories: categories, operations: operations, bankSync: bankSync}
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		user := CurrentUser(r)
		if user == nil {
			return nil
		}
		return tools.serverFor(user)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}

type mcpTools struct {
	accounts   *AccountHandler
	categories *CategoryHandler
	operations *OperationHandler
	bankSync   *BankSyncHandler
}

const mcpInstructions = `Budget app tools. Typical categorization flow: list_accounts, list_categories, ` +
	`then list_operations for an account with uncategorized_only set, ` +
	`then categorize_operations with category ids from list_categories. auto_categorize_account fills in ` +
	`what the app can guess on its own from past categorizations; sync_bank fetches fresh operations from the ` +
	`connected bank first. Amounts are signed decimal strings (negative = debit).`

func (t *mcpTools) serverFor(user *entities.User) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "budget", Version: "1.0.0"}, &mcp.ServerOptions{Instructions: mcpInstructions})
	viewerID := user.ID

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_accounts",
		Description: "List the bank accounts the user can access: their own, plus accounts shared with them (owner_display_name set).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, mcpAccountsOutput, error) {
		out, err := t.accounts.listAccessible(ctx, viewerID)
		if err != nil {
			return nil, mcpAccountsOutput{}, mcpError(err)
		}
		return nil, mcpAccountsOutput{Accounts: out}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_categories",
		Description: "List every category the user can assign: standard ones and custom ones. Categories form a tree via parent_id; prefer the most specific subcategory that fits.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, mcpCategoriesOutput, error) {
		out, err := t.categories.listVisible(ctx, viewerID)
		if err != nil {
			return nil, mcpCategoriesOutput{}, mcpError(err)
		}
		return nil, mcpCategoriesOutput{Categories: out}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_operations",
		Description: "List an account's operations, newest first, paginated. Pass next_cursor back as cursor to get the next page (null means no more).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpListOperationsInput) (*mcp.CallToolResult, operationsPageResponse, error) {
		page, err := t.listOperations(ctx, viewerID, in)
		if err != nil {
			return nil, operationsPageResponse{}, mcpError(err)
		}
		return nil, page, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "categorize_operations",
		Description: "Assign a category to one or more operations (replacing any previous category or split). Set category_id to null to clear it. Each assignment succeeds or fails independently.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpCategorizeInput) (*mcp.CallToolResult, mcpCategorizeOutput, error) {
		if len(in.Assignments) == 0 {
			return nil, mcpCategorizeOutput{}, errors.New("assignments must not be empty")
		}
		out := mcpCategorizeOutput{Updated: []operationResponse{}, Failed: []mcpCategorizeFailure{}}
		for _, a := range in.Assignments {
			saved, err := t.categorize(ctx, viewerID, a)
			if err != nil {
				out.Failed = append(out.Failed, mcpCategorizeFailure{OperationID: a.OperationID, Error: mcpError(err).Error()})
				continue
			}
			out.Updated = append(out.Updated, toOperationResponse(*saved, nil))
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "auto_categorize_account",
		Description: "Let the app categorize every still-uncategorized operation of an account it can confidently guess (from the user's and others' past choices, then a merchant dictionary). Never overwrites an existing category.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpAccountInput) (*mcp.CallToolResult, autoCategorizeResponse, error) {
		account, err := t.account(ctx, viewerID, in.AccountID)
		if err != nil {
			return nil, autoCategorizeResponse{}, mcpError(err)
		}
		count, err := t.operations.autoCategorize.CategorizeAccount(ctx, account.ID, account.UserID)
		if err != nil {
			return nil, autoCategorizeResponse{}, mcpError(err)
		}
		return nil, autoCategorizeResponse{CategorizedCount: count}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "sync_bank",
		Description: "Fetch the latest accounts, balances and operations from the user's connected bank (only works once a bank was connected in the app). " +
			"New operations are auto-categorized. May wait for the user to approve the login in their banking app.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false), OpenWorldHint: new(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, bankSyncResultResponse, error) {
		if !t.bankSync.availableForID(ctx, viewerID) {
			return nil, bankSyncResultResponse{}, mcpError(errBankSyncUnavailable)
		}
		accounts, err := t.bankSync.sync.Sync(ctx, viewerID)
		if err != nil {
			return nil, bankSyncResultResponse{}, mcpError(err)
		}
		return nil, toBankSyncResultResponse(accounts), nil
	})

	return server
}

type mcpAccountsOutput struct {
	Accounts []accountResponse `json:"accounts"`
}

type mcpCategoriesOutput struct {
	Categories []categoryResponse `json:"categories"`
}

type mcpAccountInput struct {
	AccountID string `json:"account_id" jsonschema:"id of the account, from list_accounts"`
}

type mcpListOperationsInput struct {
	AccountID         string `json:"account_id" jsonschema:"id of the account, from list_accounts"`
	DateFrom          string `json:"date_from,omitempty" jsonschema:"only operations on or after this date (YYYY-MM-DD)"`
	DateTo            string `json:"date_to,omitempty" jsonschema:"only operations on or before this date (YYYY-MM-DD)"`
	UncategorizedOnly bool   `json:"uncategorized_only,omitempty" jsonschema:"only operations with neither a category nor splits — the ones still to categorize"`
	Limit             int    `json:"limit,omitempty" jsonschema:"page size, 1-500, default 100"`
	Cursor            string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
}

type mcpCategoryAssignment struct {
	OperationID string  `json:"operation_id"`
	CategoryID  *string `json:"category_id" jsonschema:"category id from list_categories, or null to clear"`
}

type mcpCategorizeInput struct {
	Assignments []mcpCategoryAssignment `json:"assignments"`
}

type mcpCategorizeFailure struct {
	OperationID string `json:"operation_id"`
	Error       string `json:"error"`
}

type mcpCategorizeOutput struct {
	Updated []operationResponse    `json:"updated"`
	Failed  []mcpCategorizeFailure `json:"failed"`
}

func (t *mcpTools) account(ctx context.Context, viewerID uuid.UUID, rawID string) (*entities.Account, error) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return nil, httpx.NotFound("account not found")
	}
	return t.accounts.accessibleAccountByID(ctx, viewerID, id)
}

func (t *mcpTools) listOperations(ctx context.Context, viewerID uuid.UUID, in mcpListOperationsInput) (operationsPageResponse, error) {
	account, err := t.account(ctx, viewerID, in.AccountID)
	if err != nil {
		return operationsPageResponse{}, err
	}
	dateFrom, err := parseOptionalDate(in.DateFrom, "date_from")
	if err != nil {
		return operationsPageResponse{}, err
	}
	dateTo, err := parseOptionalDate(in.DateTo, "date_to")
	if err != nil {
		return operationsPageResponse{}, err
	}
	limit := in.Limit
	if limit == 0 {
		limit = defaultOperationsPageLimit
	}
	if limit < 1 || limit > maxOperationsPageLimit {
		return operationsPageResponse{}, httpx.BadRequest(fmt.Sprintf("limit must be between 1 and %d", maxOperationsPageLimit))
	}
	after, err := decodeOperationCursor(in.Cursor)
	if err != nil {
		return operationsPageResponse{}, err
	}
	return t.operations.listPage(ctx, account.ID, dateFrom, dateTo, in.UncategorizedOnly, after, limit)
}

func (t *mcpTools) categorize(ctx context.Context, viewerID uuid.UUID, a mcpCategoryAssignment) (*entities.Operation, error) {
	operationID, err := uuid.Parse(a.OperationID)
	if err != nil {
		return nil, httpx.NotFound("operation not found")
	}
	op, account, err := t.operations.accessibleOperationByID(ctx, viewerID, operationID)
	if err != nil {
		return nil, err
	}
	var categoryID *uuid.UUID
	if a.CategoryID != nil {
		id, err := uuid.Parse(*a.CategoryID)
		if err != nil {
			return nil, httpx.BadRequest("invalid category_id")
		}
		categoryID = &id
	}
	return t.operations.assignCategory(ctx, op, account, viewerID, categoryID)
}

func parseOptionalDate(raw, field string) (*civil.Date, error) {
	if raw == "" {
		return nil, nil
	}
	d, err := civil.Parse(raw)
	if err != nil {
		return nil, httpx.BadRequest("invalid " + field)
	}
	return &d, nil
}

// mcpError turns an error into what the assistant gets to see: the message
// of an expected, user-facing error (not found, validation, a bank sync
// refusal), or an opaque one for anything else — same policy as
// httpx.WriteError, so no SQL or infrastructure detail reaches the client.
func mcpError(err error) error {
	var apiErr *httpx.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	var syncErr *services.BankSyncError
	if errors.As(err, &syncErr) {
		return syncErr
	}
	log.Printf("mcp tool error: %v", err)
	return errors.New("internal server error")
}
