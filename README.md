# Budget

A self-hosted budget app: import bank CSV exports, deduplicate them, track
recurring planned charges/incomes, reconcile plans against actual
operations, and see forward-looking budget previsions.

- **Backend** (`backend/`): Go + PostgreSQL, built as ports & adapters
  (hexagonal architecture) — `backend/internal/domain` holds framework-free
  entities and use-cases; `backend/internal/adapters` holds the Postgres
  repositories and the HTTP API, all implementing interfaces declared in
  `backend/internal/domain/ports`. This keeps the door open for a future
  CLI or other interface to reuse the same domain logic.
- **Frontend** (`frontend/`): React + TypeScript (Vite), talking to the API
  over a cookie-based session.
- **Auth**: local email/password for now. robine-id (OpenID Connect) login
  is a planned addition — see `backend/internal/domain/services/auth.go`.

## Prerequisites

- Go 1.27+
- Node.js 20+
- Docker (for Postgres)

## First-time setup

```bash
# 1. Start Postgres
docker compose up -d

# 2. Backend env
cp .env.example backend/.env   # defaults already match docker-compose.yml

# 3. Frontend
cd frontend
npm install
```

## Running locally

```bash
# Terminal 1 — backend (hot reload via air)
make backend-run

# Terminal 2 — frontend
cd frontend
npm run dev
```

Or run both together: `make dev`.

Open http://localhost:5173. The API is at http://localhost:8000.

## Tests

```bash
make backend-test   # Go unit tests
make backend-vet    # static analysis

cd frontend
npm run build       # typecheck + build
npm run lint         # oxlint
```

## Golden path

1. Register or sign in.
2. Create an account (e.g. "Checking").
3. On that account's **Import** tab, create a bank profile describing your
   bank's CSV columns (date/label/amount, or debit+credit), then upload a
   CSV export.
4. Re-importing the same file is a no-op — matching rows are deduplicated
   by reference (or by date+amount+label hash when the bank doesn't
   provide a stable reference).
5. On **Recurring items**, add recurring charges/incomes (perpetual, a
   fixed number of months, or every N weeks) and generate their occurrences.
6. On **Reconciliation**, confirm suggested matches between planned
   occurrences and actual operations (or do it manually).
7. On **Previsions**, see the combined actual + forecast running balance
   for any date range.
8. On **Categories**, organize operations under the standard categories or
   your own subcategories; assign them from the **Operations** tab.

## MCP server

The backend serves a [Model Context Protocol](https://modelcontextprotocol.io)
endpoint (Streamable HTTP) at `/api/mcp`, so an AI assistant can work with
your budget:

| Tool | What it does |
| --- | --- |
| `list_accounts` | Your accounts, plus the ones shared with you |
| `list_categories` | Every category you can assign (standard + custom) |
| `list_operations` | An account's operations, newest first, paginated (`uncategorized_only` to see what's left to categorize) |
| `categorize_operations` | Assign (or clear) categories on one or more operations |
| `auto_categorize_account` | Run the app's own auto-categorization on an account |
| `sync_bank` | Fetch fresh operations from your connected bank |

Clients sign in with OAuth: add `https://<your-host>/api/mcp` as a custom
connector (claude.ai, Claude Desktop) or with Claude Code:

```bash
claude mcp add --transport http lagune https://<your-host>/api/mcp
```

then run `/mcp` to authenticate. The browser opens the server's own login
and consent page (password, Google or Robine ID), and the client gets an
access token scoped to the MCP endpoint (1 h, refreshed automatically for
90 days). Connected apps are listed, and can be disconnected, in the mobile
app's **Réglages → Applications connectées**. Tools apply the same
owner/collaborator access rules as the app.

A session token also works as a static `Authorization: Bearer <token>`
header, for clients without OAuth support.
