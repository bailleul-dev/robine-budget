package web

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/core"
	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/httpx"
)

// The OAuth authorization server that lets MCP clients connect (see
// services.OAuthUseCase for the rules). Its HTTP surface, per the MCP
// authorization spec:
//
//   - /.well-known/oauth-protected-resource[/api/mcp] and
//     /.well-known/oauth-authorization-server: discovery metadata, which a
//     client finds from the WWW-Authenticate header MCPAuthMiddleware
//     sends back on a 401.
//   - POST /oauth/register: dynamic client registration.
//   - /oauth/authorize: a server-rendered login + consent page (the mobile
//     app is the only real client UI, and it can't host a page a third-party
//     client redirects a browser to). Login reuses the app's own session
//     cookie: password right on the page, Google and Robine ID through their
//     existing flows, which come back to /oauth/resume.
//   - POST /oauth/token: authorization_code and refresh_token grants.
//
// The pending authorization request survives the login detour in a short
// cookie scoped to /oauth, so nothing about it has to round-trip through
// Google or Robine.

//go:embed templates/oauth.html
var oauthTemplateFS embed.FS

var oauthTemplate = template.Must(template.ParseFS(oauthTemplateFS, "templates/oauth.html"))

const (
	oauthPendingCookie = "oauth_pending"
	oauthCSRFCookie    = "oauth_csrf"
	mcpResourcePath    = "/api/mcp"
)

type OAuthHandler struct {
	oauth    *services.OAuthUseCase
	auth     *services.AuthUseCase
	users    ports.UserRepository
	settings core.Settings
	tokens   security.SessionTokens
}

func NewOAuthHandler(oauth *services.OAuthUseCase, auth *services.AuthUseCase, users ports.UserRepository, settings core.Settings) *OAuthHandler {
	return &OAuthHandler{
		oauth: oauth, auth: auth, users: users, settings: settings,
		tokens: security.SessionTokens{Secret: settings.JWTSecret, ExpireMinutes: settings.JWTExpireMinutes},
	}
}

// --- Discovery -------------------------------------------------------------

func mcpResourceURL(r *http.Request) string { return requestOrigin(r) + mcpResourcePath }

func mcpResourceMetadataURL(r *http.Request) string {
	return requestOrigin(r) + "/.well-known/oauth-protected-resource" + mcpResourcePath
}

// ProtectedResourceMetadata is RFC 9728: which authorization server issues
// tokens for the MCP endpoint.
func (h *OAuthHandler) ProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"resource":                 mcpResourceURL(r),
		"authorization_servers":    []string{requestOrigin(r)},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "lagune",
	})
}

// AuthorizationServerMetadata is RFC 8414.
func (h *OAuthHandler) AuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	origin := requestOrigin(r)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"issuer":                                origin,
		"authorization_endpoint":                origin + "/oauth/authorize",
		"token_endpoint":                        origin + "/oauth/token",
		"registration_endpoint":                 origin + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post", "client_secret_basic"},
	})
}

// --- Registration ----------------------------------------------------------

type registerClientRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type registerClientResponse struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientSecretExpiresAt   *int64   `json:"client_secret_expires_at,omitempty"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

func (h *OAuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var body registerClientRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeOAuthError(w, &services.OAuthError{Code: "invalid_client_metadata", Description: "invalid JSON body"})
		return
	}
	method := body.TokenEndpointAuthMethod
	switch method {
	case "":
		method = "none"
	case "none", "client_secret_post", "client_secret_basic":
	default:
		writeOAuthError(w, &services.OAuthError{Code: "invalid_client_metadata", Description: "unsupported token_endpoint_auth_method"})
		return
	}
	client, secret, err := h.oauth.RegisterClient(r.Context(), body.ClientName, body.RedirectURIs, method != "none")
	if err != nil {
		writeOAuthError(w, err)
		return
	}
	resp := registerClientResponse{
		ClientID: client.ID, ClientSecret: secret, ClientIDIssuedAt: client.CreatedAt.Unix(),
		ClientName: client.Name, RedirectURIs: client.RedirectURIs, TokenEndpointAuthMethod: method,
		GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"},
	}
	if secret != "" {
		never := int64(0)
		resp.ClientSecretExpiresAt = &never
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

// --- Authorization (login + consent pages) --------------------------------

type oauthPageData struct {
	Mode          string // "error", "login" or "consent"
	Title         string
	Error         string
	ClientName    string
	ClientInitial string
	RedirectHost  string
	UserLabel     string
	UserInitial   string
	Email         string
	CSRF          string
	GoogleURL     string
	RobineURL     string
}

// pendingAuthorization is an authorization request that passed validation.
type pendingAuthorization struct {
	req    services.AuthorizeRequest
	state  string
	client *entities.OAuthClient
}

// Authorize is the authorization endpoint (GET): validates the client's
// request, remembers it, then shows the login or the consent page.
func (h *OAuthHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	pending, ok := h.validatePending(w, r, r.URL.Query())
	if !ok {
		return
	}
	h.setCookie(w, oauthPendingCookie, base64.RawURLEncoding.EncodeToString([]byte(r.URL.RawQuery)), 15*time.Minute)
	h.renderForUser(w, r, pending, "", "")
}

// Decide handles the consent form (POST /oauth/authorize).
func (h *OAuthHandler) Decide(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(r) {
		h.renderError(w, http.StatusForbidden, "La demande a expiré. Veuillez réessayer.")
		return
	}
	pending, ok := h.loadPending(w, r)
	if !ok {
		return
	}
	user := h.sessionUser(r)
	if user == nil {
		h.renderForUser(w, r, pending, "", "")
		return
	}
	h.clearCookie(w, oauthPendingCookie)
	if r.PostFormValue("decision") != "approve" {
		redirectWithParams(w, r, pending.req.RedirectURI, url.Values{"error": {"access_denied"}, "state": {pending.state}, "iss": {requestOrigin(r)}})
		return
	}
	code, err := h.oauth.IssueCode(r.Context(), user.ID, pending.req)
	if err != nil {
		log.Printf("oauth: issue code: %v", err)
		h.renderError(w, http.StatusInternalServerError, "Une erreur est survenue. Veuillez réessayer.")
		return
	}
	redirectWithParams(w, r, pending.req.RedirectURI, url.Values{"code": {code}, "state": {pending.state}, "iss": {requestOrigin(r)}})
}

// Login handles the password form on the login page.
func (h *OAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(r) {
		h.renderError(w, http.StatusForbidden, "La demande a expiré. Veuillez réessayer.")
		return
	}
	pending, ok := h.loadPending(w, r)
	if !ok {
		return
	}
	email := strings.TrimSpace(r.PostFormValue("email"))
	user, err := h.auth.AuthenticateWithPassword(r.Context(), email, r.PostFormValue("password"))
	if err != nil {
		if !errors.Is(err, services.ErrInvalidCredentials) {
			log.Printf("oauth: password login: %v", err)
		}
		h.renderLogin(w, r, pending, "E-mail ou mot de passe incorrect.", email)
		return
	}
	if !h.startSession(w, user) {
		return
	}
	http.Redirect(w, r, "/oauth/resume", http.StatusSeeOther)
}

// Logout ("Changer" on the consent page) signs out so another account can
// be used, then goes back to the pending request's login page.
func (h *OAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(r) {
		h.renderError(w, http.StatusForbidden, "La demande a expiré. Veuillez réessayer.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: security.SessionCookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/oauth/resume", http.StatusSeeOther)
}

// Resume brings the browser back to the pending authorization request —
// after the password form, or at the end of the Google or Robine login
// flows. Robine hands its session token over as ?token= (see
// AuthHandler.RobineCallback); Google already set the session cookie.
func (h *OAuthHandler) Resume(w http.ResponseWriter, r *http.Request) {
	raw, ok := h.pendingQuery(r)
	if !ok {
		h.renderError(w, http.StatusBadRequest, "La demande d'autorisation a expiré.")
		return
	}
	if token := r.URL.Query().Get("token"); token != "" {
		userID, version, err := h.tokens.Decode(token)
		var user *entities.User
		if err == nil {
			user, err = h.users.GetByID(r.Context(), userID)
		}
		if err != nil || user == nil || user.TokenVersion != version {
			h.renderError(w, http.StatusBadRequest, "La connexion a échoué. Veuillez réessayer.")
			return
		}
		if !h.startSession(w, user) {
			return
		}
	}
	http.Redirect(w, r, "/oauth/authorize?"+raw, http.StatusSeeOther)
}

func (h *OAuthHandler) validatePending(w http.ResponseWriter, r *http.Request, q url.Values) (*pendingAuthorization, bool) {
	req := services.AuthorizeRequest{
		ClientID: q.Get("client_id"), RedirectURI: q.Get("redirect_uri"), ResponseType: q.Get("response_type"),
		CodeChallenge: q.Get("code_challenge"), CodeChallengeMethod: q.Get("code_challenge_method"),
	}
	client, err := h.oauth.ValidateAuthorizeRequest(r.Context(), req)
	if errors.Is(err, services.ErrOAuthUntrustedRedirect) {
		h.renderError(w, http.StatusBadRequest, "Cette application n'est pas reconnue, ou son adresse de retour n'est pas enregistrée.")
		return nil, false
	}
	var oerr *services.OAuthError
	if errors.As(err, &oerr) {
		redirectWithParams(w, r, req.RedirectURI, url.Values{"error": {oerr.Code}, "error_description": {oerr.Description}, "state": {q.Get("state")}})
		return nil, false
	}
	if err != nil {
		log.Printf("oauth: validate authorize request: %v", err)
		h.renderError(w, http.StatusInternalServerError, "Une erreur est survenue. Veuillez réessayer.")
		return nil, false
	}
	// The MCP endpoint is the only resource there is; RFC 8707 clients name
	// it explicitly.
	if resource := q.Get("resource"); resource != "" && resource != mcpResourceURL(r) && resource != requestOrigin(r) {
		redirectWithParams(w, r, req.RedirectURI, url.Values{"error": {"invalid_target"}, "state": {q.Get("state")}})
		return nil, false
	}
	return &pendingAuthorization{req: req, state: q.Get("state"), client: client}, true
}

func (h *OAuthHandler) pendingQuery(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(oauthPendingCookie)
	if err != nil {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// loadPending re-reads and re-validates the request stashed by Authorize.
func (h *OAuthHandler) loadPending(w http.ResponseWriter, r *http.Request) (*pendingAuthorization, bool) {
	raw, ok := h.pendingQuery(r)
	if !ok {
		h.renderError(w, http.StatusBadRequest, "La demande d'autorisation a expiré.")
		return nil, false
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		h.renderError(w, http.StatusBadRequest, "La demande d'autorisation a expiré.")
		return nil, false
	}
	return h.validatePending(w, r, q)
}

func (h *OAuthHandler) sessionUser(r *http.Request) *entities.User {
	cookie, err := r.Cookie(security.SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	userID, version, err := h.tokens.Decode(cookie.Value)
	if err != nil {
		return nil
	}
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil || user.TokenVersion != version {
		return nil
	}
	return user
}

func (h *OAuthHandler) startSession(w http.ResponseWriter, user *entities.User) bool {
	token, err := h.tokens.Create(user.ID, user.TokenVersion)
	if err != nil {
		log.Printf("oauth: create session: %v", err)
		h.renderError(w, http.StatusInternalServerError, "Une erreur est survenue. Veuillez réessayer.")
		return false
	}
	writeSessionCookie(w, h.settings, token)
	return true
}

func (h *OAuthHandler) renderForUser(w http.ResponseWriter, r *http.Request, pending *pendingAuthorization, loginError, email string) {
	user := h.sessionUser(r)
	if user == nil {
		h.renderLogin(w, r, pending, loginError, email)
		return
	}
	label := user.DisplayName
	if user.Email != nil && *user.Email != "" {
		label = *user.Email
	}
	h.render(w, http.StatusOK, oauthPageData{
		Mode: "consent", Title: "Autoriser " + pending.client.Name, ClientName: pending.client.Name,
		ClientInitial: initial(pending.client.Name), RedirectHost: redirectHost(pending.req.RedirectURI),
		UserLabel: label, UserInitial: initial(user.DisplayName + label), CSRF: h.ensureCSRF(w, r),
	})
}

func (h *OAuthHandler) renderLogin(w http.ResponseWriter, r *http.Request, pending *pendingAuthorization, loginError, email string) {
	data := oauthPageData{
		Mode: "login", Title: "Connexion", Error: loginError, Email: email, ClientName: pending.client.Name,
		ClientInitial: initial(pending.client.Name), CSRF: h.ensureCSRF(w, r),
	}
	if h.settings.GoogleClientID != "" {
		data.GoogleURL = "/api/auth/google/login?redirect=" + url.QueryEscape("/oauth/resume")
	}
	if h.settings.RobineClientID != "" {
		data.RobineURL = "/api/auth/oidc/robine/login?callback=" + url.QueryEscape(requestOrigin(r)+"/oauth/resume")
	}
	status := http.StatusOK
	if loginError != "" {
		status = http.StatusUnauthorized
	}
	h.render(w, status, data)
}

func (h *OAuthHandler) renderError(w http.ResponseWriter, status int, message string) {
	h.render(w, status, oauthPageData{Mode: "error", Title: "Erreur", Error: message})
}

func (h *OAuthHandler) render(w http.ResponseWriter, status int, data oauthPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// Never framed: a consent button inside someone else's page could be
	// clickjacked.
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.WriteHeader(status)
	if err := oauthTemplate.Execute(w, data); err != nil {
		log.Printf("oauth: render page: %v", err)
	}
}

// ensureCSRF returns the double-submit token the page's forms echo back,
// minting its cookie on first use.
func (h *OAuthHandler) ensureCSRF(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(oauthCSRFCookie); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	h.setCookie(w, oauthCSRFCookie, token, time.Hour)
	return token
}

func (h *OAuthHandler) checkCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(oauthCSRFCookie)
	form := r.PostFormValue("csrf")
	return err == nil && cookie.Value != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(form)) == 1
}

func (h *OAuthHandler) setCookie(w http.ResponseWriter, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/oauth", HttpOnly: true, Secure: h.settings.CookieSecure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(maxAge.Seconds()),
	})
}

func (h *OAuthHandler) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/oauth", MaxAge: -1})
}

func redirectWithParams(w http.ResponseWriter, r *http.Request, redirectURI string, params url.Values) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	q := u.Query()
	for k, v := range params {
		if len(v) > 0 && v[0] != "" {
			q.Set(k, v[0])
		}
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func redirectHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

func initial(s string) string {
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			return strings.ToUpper(string(c))
		}
	}
	return "?"
}

// --- Token endpoint ------------------------------------------------------------

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func (h *OAuthHandler) Token(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, &services.OAuthError{Code: "invalid_request", Description: "invalid form body"})
		return
	}
	clientID, clientSecret := r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	if id, secret, ok := r.BasicAuth(); ok {
		// client_secret_basic form-encodes both before base64 (RFC 6749 §2.3.1).
		clientID, _ = url.QueryUnescape(id)
		clientSecret, _ = url.QueryUnescape(secret)
	}

	var tokens *services.OAuthTokens
	var err error
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		tokens, err = h.oauth.ExchangeCode(r.Context(), clientID, clientSecret,
			r.PostForm.Get("code"), r.PostForm.Get("redirect_uri"), r.PostForm.Get("code_verifier"))
	case "refresh_token":
		tokens, err = h.oauth.Refresh(r.Context(), clientID, clientSecret, r.PostForm.Get("refresh_token"))
	default:
		err = &services.OAuthError{Code: "unsupported_grant_type", Description: "grant_type must be authorization_code or refresh_token"}
	}
	if err != nil {
		writeOAuthError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, oauthTokenResponse{
		AccessToken: tokens.AccessToken, TokenType: "Bearer", ExpiresIn: int(tokens.ExpiresIn.Seconds()), RefreshToken: tokens.RefreshToken,
	})
}

func writeOAuthError(w http.ResponseWriter, err error) {
	var oerr *services.OAuthError
	if !errors.As(err, &oerr) {
		log.Printf("oauth: %v", err)
		oerr = &services.OAuthError{Code: "server_error", Description: "internal server error"}
	}
	status := http.StatusBadRequest
	switch oerr.Code {
	case "invalid_client":
		status = http.StatusUnauthorized
	case "server_error":
		status = http.StatusInternalServerError
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, status, map[string]string{"error": oerr.Code, "error_description": oerr.Description})
}

// --- Connected apps (for the mobile app's settings) ---------------------------

type oauthGrantResponse struct {
	ID          string    `json:"id"`
	ClientName  string    `json:"client_name"`
	CreatedAt   time.Time `json:"created_at"`
	RefreshedAt time.Time `json:"last_used_at"`
}

func (h *OAuthHandler) ListGrants(w http.ResponseWriter, r *http.Request) {
	grants, err := h.oauth.ListGrants(r.Context(), CurrentUser(r).ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]oauthGrantResponse, len(grants))
	for i, g := range grants {
		out[i] = oauthGrantResponse{ID: g.ID.String(), ClientName: g.ClientName, CreatedAt: g.CreatedAt, RefreshedAt: g.RefreshedAt}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *OAuthHandler) RevokeGrant(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "grantID")
	if err != nil {
		httpx.WriteError(w, httpx.NotFound("connected app not found"))
		return
	}
	ok, err := h.oauth.RevokeGrant(r.Context(), CurrentUser(r).ID, id)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if !ok {
		httpx.WriteError(w, httpx.NotFound("connected app not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- MCP endpoint auth ---------------------------------------------------------

// MCPAuthMiddleware guards the MCP endpoint. It accepts an OAuth access
// token, or — for clients configured by hand with a static header — a
// session token, both only as "Authorization: Bearer" (never the browser
// cookie). A 401 points the client at the protected resource metadata, which
// is how an MCP client discovers it should start the OAuth flow.
func MCPAuthMiddleware(oauth *services.OAuthUseCase, users ports.UserRepository, tokens security.SessionTokens) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			unauthorized := func(challengeError string) {
				challenge := `Bearer resource_metadata="` + mcpResourceMetadataURL(r) + `"`
				if challengeError != "" {
					challenge += `, error="` + challengeError + `"`
				}
				w.Header().Set("WWW-Authenticate", challenge)
				http.Error(w, `{"detail":"not authenticated"}`, http.StatusUnauthorized)
			}
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				unauthorized("")
				return
			}
			token := strings.TrimPrefix(auth, "Bearer ")
			var userID uuid.UUID
			var err error
			// sessionVersion is -1 for an OAuth access token: those have their
			// own revocation (grants), not the session token version.
			sessionVersion := -1
			if strings.HasPrefix(token, services.OAuthAccessTokenPrefix) {
				userID, err = oauth.AuthenticateAccessToken(r.Context(), token)
			} else {
				userID, sessionVersion, err = tokens.Decode(token)
			}
			if err != nil {
				if !errors.Is(err, services.ErrOAuthInvalidToken) && strings.HasPrefix(token, services.OAuthAccessTokenPrefix) {
					log.Printf("mcp auth: %v", err)
				}
				unauthorized("invalid_token")
				return
			}
			user, err := users.GetByID(r.Context(), userID)
			if err != nil || user == nil || (sessionVersion >= 0 && user.TokenVersion != sessionVersion) {
				unauthorized("invalid_token")
				return
			}
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
		})
	}
}

// PublicCORSMiddleware opens the discovery, registration and token endpoints
// to any origin — browser-based MCP clients call them directly, and none of
// them rely on cookies, so there's no ambient credential to protect.
func PublicCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
