package web

import (
	"net/http"
	"strings"
	"time"

	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/core"
	"bailleul.dev/budget-server/internal/domain/ports"
)

// Handlers bundles every resource handler the router wires up — the
// composition root (cmd/server/main.go) builds one of these.
type Handlers struct {
	Auth               *AuthHandler
	Accounts           *AccountHandler
	Categories         *CategoryHandler
	BalanceCheckpoints *BalanceCheckpointHandler
	Operations         *OperationHandler
	PlannedItems       *PlannedItemHandler
	Reconciliation     *ReconciliationHandler
	Previsions         *PrevisionsHandler
	BankProfiles       *BankProfileHandler
	Imports            *ImportHandler
	Collaborators      *CollaboratorHandler
	Export             *ExportHandler
	BankSync           *BankSyncHandler
	// MCP is the Model Context Protocol endpoint (see NewMCPHandler) and
	// OAuth the authorization server MCP clients sign in through; both may
	// be nil (in tests), their routes are then not registered.
	MCP   http.Handler
	OAuth *OAuthHandler
	// OTAUpdates may be nil (no OTA_UPDATES_DIR configured): its routes
	// then answer 404.
	OTAUpdates *OTAUpdatesHandler
}

func NewRouter(h Handlers, users ports.UserRepository, settings core.Settings) http.Handler {
	mux := http.NewServeMux()

	// One shared budget per client IP for every endpoint that checks a
	// secret or sends an email: 10 requests at once, then one every 6 s.
	// Generous for a person, useless for a brute-force script.
	authIPLimiter := newRateLimiter(6*time.Second, 10)
	authLimit := func(next http.HandlerFunc) http.HandlerFunc {
		return rateLimited(authIPLimiter, settings.TrustedProxyHops, next)
	}

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// --- Version (public) --------------------------------------------------
	mux.HandleFunc("GET /api/version", VersionHandler)

	// --- Auth (public) ---------------------------------------------------
	mux.HandleFunc("GET /api/auth/config", h.Auth.Config)
	mux.HandleFunc("POST /api/auth/register", authLimit(h.Auth.Register))
	mux.HandleFunc("POST /api/auth/login", authLimit(h.Auth.Login))
	mux.HandleFunc("POST /api/auth/logout", h.Auth.Logout)
	mux.HandleFunc("POST /api/auth/forgot-password", authLimit(h.Auth.ForgotPassword))

	// --- Pages behind the links in verification / reset emails (public) ---
	mux.HandleFunc("GET /verify-email", h.Auth.VerifyEmailPage)
	mux.HandleFunc("POST /verify-email", authLimit(h.Auth.VerifyEmailSubmit))
	mux.HandleFunc("GET /reset-password", h.Auth.ResetPasswordPage)
	mux.HandleFunc("POST /reset-password", authLimit(h.Auth.ResetPasswordSubmit))
	mux.HandleFunc("GET /api/auth/google/login", h.Auth.GoogleLogin)
	mux.HandleFunc("GET /api/auth/google/callback", h.Auth.GoogleCallback)
	mux.HandleFunc("POST /api/auth/google/mobile", authLimit(h.Auth.GoogleMobileLogin))
	mux.HandleFunc("POST /api/auth/apple/mobile", authLimit(h.Auth.AppleMobileLogin))
	mux.HandleFunc("GET /api/auth/oidc/robine/login", h.Auth.RobineLogin)
	mux.HandleFunc("GET /api/auth/oidc/robine/callback", h.Auth.RobineCallback)

	// --- Mobile over-the-air updates (public: checked before sign-in) ----
	// More specific than "/api/" below, so ServeMux routes these here
	// instead of through AuthMiddleware.
	mux.HandleFunc("GET /api/updates/manifest", h.OTAUpdates.Manifest)
	mux.HandleFunc("GET /api/updates/assets/{runtimeVersion}/{updateName}/{path...}", h.OTAUpdates.Asset)

	// --- Share invites (public landing info, redeeming needs a session) --
	mux.HandleFunc("GET /api/invites/{token}", h.Collaborators.GetInvite)

	// --- Everything below requires a session -----------------------------
	protected := http.NewServeMux()

	protected.HandleFunc("GET /api/auth/me", h.Auth.Me)
	protected.HandleFunc("DELETE /api/auth/me", h.Auth.DeleteMe)
	protected.HandleFunc("POST /api/auth/logout-all", h.Auth.LogoutAll)
	protected.HandleFunc("GET /api/auth/me/export", h.Export.Export)
	protected.HandleFunc("POST /api/auth/resend-verification", h.Auth.ResendVerification)

	protected.HandleFunc("GET /api/accounts", h.Accounts.List)
	protected.HandleFunc("POST /api/accounts", h.Accounts.Create)
	protected.HandleFunc("GET /api/accounts/{accountID}", h.Accounts.Get)
	protected.HandleFunc("DELETE /api/accounts/{accountID}", h.Accounts.Delete)

	protected.HandleFunc("GET /api/categories", h.Categories.List)
	protected.HandleFunc("POST /api/categories", h.Categories.Create)
	protected.HandleFunc("PUT /api/categories/{categoryID}", h.Categories.Update)
	protected.HandleFunc("DELETE /api/categories/{categoryID}", h.Categories.Delete)

	protected.HandleFunc("GET /api/accounts/{accountID}/balance-checkpoints", h.BalanceCheckpoints.List)
	protected.HandleFunc("POST /api/accounts/{accountID}/balance-checkpoints", h.BalanceCheckpoints.Create)
	protected.HandleFunc("POST /api/accounts/{accountID}/balance-checkpoints/initial", h.BalanceCheckpoints.ComputeInitial)
	protected.HandleFunc("DELETE /api/balance-checkpoints/{checkpointID}", h.BalanceCheckpoints.Delete)

	protected.HandleFunc("GET /api/accounts/{accountID}/operations", h.Operations.List)
	protected.HandleFunc("POST /api/accounts/{accountID}/operations/auto-categorize", h.Operations.AutoCategorize)
	protected.HandleFunc("PUT /api/operations/{operationID}/category", h.Operations.SetCategory)
	protected.HandleFunc("PUT /api/operations/{operationID}/splits", h.Operations.SetSplits)
	protected.HandleFunc("GET /api/operations/{operationID}/comments", h.Operations.ListComments)
	protected.HandleFunc("POST /api/operations/{operationID}/comments", h.Operations.AddComment)

	protected.HandleFunc("GET /api/planned-items", h.PlannedItems.ListAll)
	protected.HandleFunc("POST /api/planned-items", h.PlannedItems.Create)
	protected.HandleFunc("PUT /api/planned-items/{itemID}", h.PlannedItems.Update)
	protected.HandleFunc("GET /api/accounts/{accountID}/planned-items", h.PlannedItems.ListForAccount)
	protected.HandleFunc("GET /api/accounts/{accountID}/recurring-suggestions", h.PlannedItems.RecurringSuggestions)
	protected.HandleFunc("GET /api/accounts/{accountID}/planned-occurrences", h.PlannedItems.ListOccurrencesForAccount)
	protected.HandleFunc("POST /api/planned-items/{itemID}/generate-occurrences", h.PlannedItems.GenerateOccurrences)
	protected.HandleFunc("GET /api/planned-items/{itemID}/occurrences", h.PlannedItems.ListOccurrences)

	protected.HandleFunc("GET /api/accounts/{accountID}/reconciliation/unreconciled-operations", h.Reconciliation.ListUnreconciledOperations)
	protected.HandleFunc("GET /api/accounts/{accountID}/reconciliation/suggestions", h.Reconciliation.Suggestions)
	protected.HandleFunc("POST /api/reconciliation", h.Reconciliation.Reconcile)
	protected.HandleFunc("DELETE /api/reconciliation/{occurrenceID}", h.Reconciliation.Unreconcile)

	protected.HandleFunc("GET /api/accounts/{accountID}/previsions", h.Previsions.Get)

	protected.HandleFunc("GET /api/bank-profiles", h.BankProfiles.List)
	protected.HandleFunc("POST /api/bank-profiles", h.BankProfiles.Create)
	protected.HandleFunc("PUT /api/bank-profiles/{profileID}", h.BankProfiles.Update)

	protected.HandleFunc("POST /api/accounts/{accountID}/imports", h.Imports.Import)

	protected.HandleFunc("GET /api/bank-sync/status", h.BankSync.Status)
	protected.HandleFunc("POST /api/bank-sync/connect", h.BankSync.Connect)
	protected.HandleFunc("POST /api/bank-sync/sync", h.BankSync.Sync)

	protected.HandleFunc("GET /api/collaborators", h.Collaborators.List)
	protected.HandleFunc("POST /api/collaborators", h.Collaborators.Create)
	protected.HandleFunc("DELETE /api/collaborators/{collaboratorID}", h.Collaborators.Revoke)
	protected.HandleFunc("POST /api/invites/{token}/redeem", h.Collaborators.RedeemInvite)

	tokens := security.SessionTokens{Secret: settings.JWTSecret, ExpireMinutes: settings.JWTExpireMinutes}

	// --- MCP + the OAuth server its clients sign in through ---------------
	// Discovery, registration and token endpoints are open to any origin
	// (PublicCORSMiddleware, applied below), the MCP endpoint has its own
	// Bearer-only auth that answers 401s the way MCP clients expect.
	if h.OAuth != nil {
		mux.HandleFunc("GET /.well-known/oauth-protected-resource", h.OAuth.ProtectedResourceMetadata)
		mux.HandleFunc("GET /.well-known/oauth-protected-resource/api/mcp", h.OAuth.ProtectedResourceMetadata)
		mux.HandleFunc("GET /.well-known/oauth-authorization-server", h.OAuth.AuthorizationServerMetadata)
		mux.HandleFunc("POST /oauth/register", authLimit(h.OAuth.Register))
		mux.HandleFunc("POST /oauth/token", authLimit(h.OAuth.Token))
		mux.HandleFunc("GET /oauth/authorize", h.OAuth.Authorize)
		mux.HandleFunc("POST /oauth/authorize", authLimit(h.OAuth.Decide))
		mux.HandleFunc("POST /oauth/login", authLimit(h.OAuth.Login))
		mux.HandleFunc("POST /oauth/logout", h.OAuth.Logout)
		mux.HandleFunc("GET /oauth/resume", h.OAuth.Resume)

		protected.HandleFunc("GET /api/oauth/grants", h.OAuth.ListGrants)
		protected.HandleFunc("DELETE /api/oauth/grants/{grantID}", h.OAuth.RevokeGrant)

		if h.MCP != nil {
			mux.Handle("/api/mcp", MCPAuthMiddleware(h.OAuth.oauth, users, tokens)(h.MCP))
		}
	}

	mux.Handle("/api/", AuthMiddleware(users, tokens)(protected))

	// Everything else: the built frontend (a no-op in local dev, where the
	// embedded dist/ is just a placeholder — the frontend runs separately
	// via `npm run dev` there instead).
	mux.Handle("/", NewStaticHandler())

	credentialed := CORSMiddleware(settings)(mux)
	public := PublicCORSMiddleware(mux)
	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicOAuthPath(r.URL.Path) {
			public.ServeHTTP(w, r)
			return
		}
		credentialed.ServeHTTP(w, r)
	})
	return SecurityHeadersMiddleware(settings)(LimitBodiesMiddleware(dispatch))
}

func isPublicOAuthPath(path string) bool {
	return strings.HasPrefix(path, "/.well-known/oauth-") || path == "/oauth/register" || path == "/oauth/token"
}
