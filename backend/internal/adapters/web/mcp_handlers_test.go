package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

type mcpFixture struct {
	*operationHandlerFixture
	user    *entities.User
	session *mcp.ClientSession
}

// newMCPFixture serves NewMCPHandler over a real HTTP server, signed in as a
// fresh user the same way AuthMiddleware would, and connects the SDK's own
// client to it — so these tests cover the actual wire protocol, not just
// the tool functions.
func newMCPFixture(t *testing.T) *mcpFixture {
	t.Helper()
	f := newOperationHandlerFixture()
	user := newTestUser()
	bankSync, _, _, _ := newTestBankSyncHandler(&fakeBankConnector{})
	handler := NewMCPHandler(f.accountHandler, NewCategoryHandler(f.categories, f.accountHandler), f.handler, bankSync)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, asUser(r, user))
	}))
	t.Cleanup(server.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return &mcpFixture{operationHandlerFixture: f, user: user, session: session}
}

func (f *mcpFixture) call(t *testing.T, name string, args any, dest any) *mcp.CallToolResult {
	t.Helper()
	res, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	if dest != nil && !res.IsError {
		raw, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(raw, dest); err != nil {
			t.Fatalf("decode %s output: %v", name, err)
		}
	}
	return res
}

func toolErrorText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

func (f *mcpFixture) addAccount(ownerID uuid.UUID) *entities.Account {
	account := &entities.Account{ID: uuid.New(), UserID: ownerID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	return account
}

func TestMCP_ListsTools(t *testing.T) {
	f := newMCPFixture(t)
	res, err := f.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	want := "list_accounts,list_categories,list_operations,categorize_operations,auto_categorize_account,sync_bank"
	for _, name := range strings.Split(want, ",") {
		if !strings.Contains(strings.Join(names, ","), name) {
			t.Errorf("missing tool %q, got %v", name, names)
		}
	}
}

func TestMCP_ListAccountsAndOperations(t *testing.T) {
	f := newMCPFixture(t)
	account := f.addAccount(f.user.ID)
	f.addAccount(uuid.New()) // a stranger's, must not show up
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "CARREFOUR", Amount: mustAmount(t, "-42.10")}
	f.operations.byID[op.ID] = op

	var accounts mcpAccountsOutput
	f.call(t, "list_accounts", map[string]any{}, &accounts)
	if len(accounts.Accounts) != 1 || accounts.Accounts[0].ID != account.ID.String() {
		t.Fatalf("accounts = %+v, want only the user's own", accounts.Accounts)
	}

	var page operationsPageResponse
	f.call(t, "list_operations", map[string]any{"account_id": account.ID.String()}, &page)
	if len(page.Operations) != 1 || page.Operations[0].Amount != "-42.10" {
		t.Fatalf("operations = %+v", page.Operations)
	}
}

func TestMCP_ListOperations_StrangersAccountNotFound(t *testing.T) {
	f := newMCPFixture(t)
	other := f.addAccount(uuid.New())

	res := f.call(t, "list_operations", map[string]any{"account_id": other.ID.String()}, nil)
	if !res.IsError || toolErrorText(res) != "account not found" {
		t.Fatalf("IsError = %v, text = %q, want account not found", res.IsError, toolErrorText(res))
	}
}

func TestMCP_CategorizeOperations(t *testing.T) {
	f := newMCPFixture(t)
	account := f.addAccount(f.user.ID)
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "CARREFOUR", Amount: mustAmount(t, "-42.10")}
	f.operations.byID[op.ID] = op
	strangerOp := &entities.Operation{ID: uuid.New(), AccountID: f.addAccount(uuid.New()).ID, Date: civil.Today(), Label: "X", Amount: mustAmount(t, "-1.00")}
	f.operations.byID[strangerOp.ID] = strangerOp
	category := &entities.Category{ID: uuid.New(), Name: "Groceries"}
	f.categories.byID[category.ID] = category
	categoryID := category.ID.String()

	var out mcpCategorizeOutput
	f.call(t, "categorize_operations", map[string]any{"assignments": []map[string]any{
		{"operation_id": op.ID.String(), "category_id": categoryID},
		{"operation_id": strangerOp.ID.String(), "category_id": categoryID},
	}}, &out)

	if len(out.Updated) != 1 || out.Updated[0].ID != op.ID.String() {
		t.Fatalf("updated = %+v, want just the user's operation", out.Updated)
	}
	if len(out.Failed) != 1 || out.Failed[0].Error != "operation not found" {
		t.Fatalf("failed = %+v, want the stranger's operation rejected", out.Failed)
	}
	if got := f.operations.byID[op.ID].CategoryID; got == nil || *got != category.ID {
		t.Errorf("stored CategoryID = %v, want %s", got, category.ID)
	}
	if strangerOp.CategoryID != nil {
		t.Error("stranger's operation must stay uncategorized")
	}
}

func TestMCP_SyncBank_NotConnected(t *testing.T) {
	f := newMCPFixture(t)

	res := f.call(t, "sync_bank", map[string]any{}, nil)
	if !res.IsError || !strings.Contains(toolErrorText(res), "no bank connection") {
		t.Fatalf("IsError = %v, text = %q, want a no-connection error", res.IsError, toolErrorText(res))
	}
}

func TestMCP_ListOperations_UncategorizedOnly(t *testing.T) {
	f := newMCPFixture(t)
	account := f.addAccount(f.user.ID)
	categoryID := uuid.New()
	todo := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "UNKNOWN", Amount: mustAmount(t, "-5.00")}
	done := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "CARREFOUR", Amount: mustAmount(t, "-42.10"), CategoryID: &categoryID}
	f.operations.byID[todo.ID] = todo
	f.operations.byID[done.ID] = done

	var page operationsPageResponse
	f.call(t, "list_operations", map[string]any{"account_id": account.ID.String(), "uncategorized_only": true}, &page)
	if len(page.Operations) != 1 || page.Operations[0].ID != todo.ID.String() {
		t.Fatalf("operations = %+v, want only the uncategorized one", page.Operations)
	}
}
