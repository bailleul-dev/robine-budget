package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/core"
	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/httpx"
)

type AuthHandler struct {
	auth     *services.AuthUseCase
	settings core.Settings
	tokens   security.SessionTokens
	// apple verifies "Sign in with Apple" identity tokens; nil-safe, and
	// unconfigured (no APPLE_BUNDLE_IDS) when the feature is off.
	apple *security.AppleVerifier

	// robineDiscoveryMu/robineDiscovery cache the OIDC discovery document
	// (see discoverRobine) for the life of the process — RobineIssuer is
	// static config, so there's no reason to re-fetch it on every login.
	// Caches only a success, never a failure, so a transient outage during
	// the very first login attempt doesn't wedge every later one until a
	// restart.
	robineDiscoveryMu sync.Mutex
	robineDiscovery   *oidcDiscoveryDocument

	// loginAttempts budgets failed password logins per (IP, email): 5 at
	// once, then one every 30 s.
	loginAttempts *rateLimiter
	// mailLimiter budgets emails triggered from public endpoints per address
	// (and per IP), so they can't be used to flood someone's inbox.
	mailLimiter *rateLimiter
}

func NewAuthHandler(auth *services.AuthUseCase, settings core.Settings) *AuthHandler {
	return &AuthHandler{
		auth:     auth,
		settings: settings,
		tokens:   security.SessionTokens{Secret: settings.JWTSecret, ExpireMinutes: settings.JWTExpireMinutes},
		apple:    security.NewAppleVerifier(settings.AppleBundleIDs),

		loginAttempts: newRateLimiter(30*time.Second, 5),
		mailLimiter:   newRateLimiter(20*time.Minute, 3),
	}
}

type userResponse struct {
	ID            string  `json:"id"`
	Email         *string `json:"email"`
	DisplayName   string  `json:"display_name"`
	HasPassword   bool    `json:"has_password"`
	HasRobineLink bool    `json:"has_robine_link"`
	HasGoogleLink bool    `json:"has_google_link"`
	EmailVerified bool    `json:"email_verified"`
}

func toUserResponse(u *entities.User) userResponse {
	return userResponse{
		ID:            u.ID.String(),
		Email:         u.Email,
		DisplayName:   u.DisplayName,
		HasPassword:   u.PasswordHash != nil,
		HasRobineLink: u.RobineSub != nil,
		HasGoogleLink: u.GoogleSub != nil,
		EmailVerified: u.EmailVerified(),
	}
}

// authResponse is what Login/Register return: the user plus the raw
// session token — the web client ignores Token (it already got the
// cookie), a native client stores it and sends it back as
// "Authorization: Bearer <token>" on every request.
type authResponse struct {
	userResponse
	Token string `json:"token"`
}

// setSessionCookie is the web login path: an HttpOnly cookie the browser
// sends back automatically. It also returns the raw token so JSON
// responses (Login/Register) can additionally hand it to a native client,
// which has no cookie jar to rely on and instead stores it itself (see
// AuthMiddleware's Bearer-header fallback) — one token, two delivery
// mechanisms depending on who's asking.
func (h *AuthHandler) setSessionCookie(w http.ResponseWriter, user *entities.User) (string, error) {
	token, err := h.tokens.Create(user.ID, user.TokenVersion)
	if err != nil {
		httpx.WriteError(w, err)
		return "", err
	}
	writeSessionCookie(w, h.settings, token)
	return token, nil
}

func writeSessionCookie(w http.ResponseWriter, settings core.Settings, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     security.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   settings.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   settings.JWTExpireMinutes * 60,
	})
}

// signupClosedDetail is shown to the person, so it is French like the app.
const signupClosedDetail = "L'inscription est fermée pour le moment (bêta privée). Demandez une invitation."

// writeAuthError maps the errors that sign-in and registration can return on
// purpose; anything else stays an opaque 500.
func writeAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, services.ErrSignupClosed) {
		httpx.WriteError(w, httpx.Forbidden(signupClosedDetail))
		return
	}
	httpx.WriteError(w, err)
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var body registerRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	user, err := h.auth.Register(r.Context(), body.Email, body.Password, body.DisplayName)
	if err != nil {
		if errors.Is(err, services.ErrEmailAlreadyRegistered) {
			httpx.WriteError(w, httpx.BadRequest(err.Error()))
			return
		}
		var invalid *services.ValidationError
		if errors.As(err, &invalid) {
			httpx.WriteError(w, httpx.BadRequest(invalid.Error()))
			return
		}
		writeAuthError(w, err)
		return
	}
	token, err := h.setSessionCookie(w, user)
	if err != nil {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authResponse{userResponse: toUserResponse(user), Token: token})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	// Failed attempts are budgeted per (client IP, email): enough for a
	// forgetful person, a wall for guessing — and one attacker can't lock
	// the real owner out from another address.
	attemptKey := clientIP(r, h.settings.TrustedProxyHops) + "|" + strings.ToLower(strings.TrimSpace(body.Email))
	if ok, retry := h.loginAttempts.take(attemptKey); !ok {
		writeTooManyRequests(w, retry)
		return
	}
	user, err := h.auth.AuthenticateWithPassword(r.Context(), body.Email, body.Password)
	if err != nil {
		httpx.WriteError(w, httpx.Unauthorized("invalid credentials"))
		return
	}
	h.loginAttempts.refund(attemptKey)
	token, err := h.setSessionCookie(w, user)
	if err != nil {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authResponse{userResponse: toUserResponse(user), Token: token})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: security.SessionCookieName, Value: "", Path: "/", MaxAge: -1,
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// LogoutAll signs the user out of every device: all outstanding session
// tokens stop working, this one included.
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	if err := h.auth.LogoutAll(r.Context(), CurrentUser(r).ID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: security.SessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.settings.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DeleteMe permanently deletes the signed-in user's own account and all of
// its data, then clears the session cookie (a bearer token held by the
// mobile app stops working on its own: AuthMiddleware no longer finds the
// user).
func (h *AuthHandler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	if err := h.auth.DeleteUser(r.Context(), user.ID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: security.SessionCookieName, Value: "", Path: "/", MaxAge: -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	httpx.WriteJSON(w, http.StatusOK, toUserResponse(user))
}

type authConfigResponse struct {
	GoogleEnabled bool `json:"google_enabled"`
	RobineEnabled bool `json:"robine_enabled"`
	AppleEnabled  bool `json:"apple_enabled"`
}

func (h *AuthHandler) Config(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, authConfigResponse{
		GoogleEnabled: h.settings.GoogleClientID != "",
		RobineEnabled: h.settings.RobineClientID != "" && h.settings.RobineIssuer != "",
		AppleEnabled:  h.apple != nil && h.apple.Configured(),
	})
}

func (h *AuthHandler) getRedirectURI(r *http.Request) string {
	if h.settings.GoogleRedirectURI != "" {
		return h.settings.GoogleRedirectURI
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/auth/google/callback", scheme, r.Host)
}

// isSafeRelativeRedirect guards the post-login destination (e.g. an invite
// link's "/invite/{token}") against becoming an open redirect: only a
// same-origin path is accepted, never an absolute or protocol-relative URL.
func isSafeRelativeRedirect(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//")
}

func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	if h.settings.GoogleClientID == "" {
		httpx.WriteError(w, httpx.BadRequest("google auth is not configured"))
		return
	}

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := hex.EncodeToString(b)

	http.SetCookie(w, &http.Cookie{
		Name:     "google_oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.settings.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	// Carries where to send the browser after a successful login (e.g. back
	// to the invite page it came from) across the round trip to Google —
	// Google echoes back our own "state" param instead, so this rides in a
	// separate short-lived cookie.
	if redirect := r.URL.Query().Get("redirect"); redirect != "" && isSafeRelativeRedirect(redirect) {
		http.SetCookie(w, &http.Cookie{
			Name:     "google_oauth_redirect",
			Value:    redirect,
			Path:     "/",
			HttpOnly: true,
			Secure:   h.settings.CookieSecure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   600,
		})
	}

	redirectURI := h.getRedirectURI(r)
	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&response_type=code&scope=%s&redirect_uri=%s&state=%s",
		url.QueryEscape(h.settings.GoogleClientID),
		url.QueryEscape("openid email profile"),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	IDToken     string `json:"id_token"`
	Error       string `json:"error"`
}

type googleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	if h.settings.GoogleClientID == "" {
		httpx.WriteError(w, httpx.BadRequest("google auth is not configured"))
		return
	}

	cookie, err := r.Cookie("google_oauth_state")
	if err != nil || cookie.Value == "" {
		httpx.WriteError(w, httpx.BadRequest("invalid state cookie"))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: "google_oauth_state", Value: "", Path: "/", MaxAge: -1,
	})

	stateParam := r.URL.Query().Get("state")
	if stateParam != cookie.Value {
		httpx.WriteError(w, httpx.BadRequest("state mismatch"))
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		httpx.WriteError(w, httpx.BadRequest("missing code parameter"))
		return
	}

	redirectURI := h.getRedirectURI(r)
	resp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {h.settings.GoogleClientID},
		"client_secret": {h.settings.GoogleClientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		httpx.WriteError(w, fmt.Errorf("failed token request: %w", err))
		return
	}
	defer resp.Body.Close()

	var tokenRes googleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		httpx.WriteError(w, fmt.Errorf("failed decoding token response: %w", err))
		return
	}
	if tokenRes.Error != "" || tokenRes.AccessToken == "" {
		httpx.WriteError(w, httpx.BadRequest("failed google authentication token exchange"))
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)

	userInfoResp, err := http.DefaultClient.Do(req)
	if err != nil {
		httpx.WriteError(w, fmt.Errorf("failed fetching user info: %w", err))
		return
	}
	defer userInfoResp.Body.Close()

	var userInfo googleUserInfo
	if err := json.NewDecoder(userInfoResp.Body).Decode(&userInfo); err != nil {
		httpx.WriteError(w, fmt.Errorf("failed decoding user info: %w", err))
		return
	}

	if userInfo.Sub == "" {
		httpx.WriteError(w, httpx.BadRequest("invalid user info from google"))
		return
	}

	user, err := h.auth.AuthenticateOrRegisterWithGoogle(r.Context(), userInfo.Sub, userInfo.Email, userInfo.EmailVerified, userInfo.Name)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	if _, err := h.setSessionCookie(w, user); err != nil {
		return
	}

	redirectPath := ""
	if cookie, err := r.Cookie("google_oauth_redirect"); err == nil && isSafeRelativeRedirect(cookie.Value) {
		redirectPath = cookie.Value
		http.SetCookie(w, &http.Cookie{
			Name: "google_oauth_redirect", Value: "", Path: "/", MaxAge: -1,
		})
	}
	// The OAuth consent pages (see OAuthHandler) are served by this server
	// itself, not the web app — stay on this origin for those.
	if strings.HasPrefix(redirectPath, "/oauth/") {
		http.Redirect(w, r, redirectPath, http.StatusFound)
		return
	}
	http.Redirect(w, r, h.settings.CORSOrigin+redirectPath, http.StatusFound)
}

// googleTokenInfo is Google's response from GET /tokeninfo?id_token=... — a
// 200 response already means the token's signature, issuer and expiry all
// checked out server-side (Google's own validation, not ours); a bad token
// gets a non-2xx "error_description" body instead. This lightweight
// verification (vs. fetching Google's JWKS and checking the signature
// ourselves) is the same approach Google's own docs suggest for a backend
// that doesn't already carry a JWT/JOSE library — matches this codebase's
// existing GoogleCallback, which also avoids one.
type googleTokenInfo struct {
	Aud           string `json:"aud"`
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Name          string `json:"name"`
	ErrorDesc     string `json:"error_description"`
}

type googleMobileLoginRequest struct {
	IDToken string `json:"id_token"`
}

// GoogleMobileLogin is the native counterpart to GoogleLogin/GoogleCallback:
// the mobile app runs the OAuth dance itself (expo-auth-session, one client
// ID per platform — see Settings.GoogleMobileClientIDs) and hands us the
// resulting ID token directly instead of a server-side redirect round trip,
// since a native app has no browser to redirect through in the first
// place. Public (no session required yet), like Login/Register.
func (h *AuthHandler) GoogleMobileLogin(w http.ResponseWriter, r *http.Request) {
	audiences := h.settings.GoogleAcceptedAudiences()
	if len(audiences) == 0 {
		httpx.WriteError(w, httpx.BadRequest("google auth is not configured"))
		return
	}

	var body googleMobileLoginRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if body.IDToken == "" {
		httpx.WriteError(w, httpx.BadRequest("id_token is required"))
		return
	}

	resp, err := http.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(body.IDToken))
	if err != nil {
		httpx.WriteError(w, fmt.Errorf("failed verifying google id token: %w", err))
		return
	}
	defer resp.Body.Close()

	var info googleTokenInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		httpx.WriteError(w, fmt.Errorf("failed decoding google tokeninfo response: %w", err))
		return
	}
	if resp.StatusCode != http.StatusOK {
		httpx.WriteError(w, httpx.Unauthorized("invalid google id token: "+info.ErrorDesc))
		return
	}
	if !slices.Contains(audiences, info.Aud) {
		httpx.WriteError(w, httpx.Unauthorized("google id token was not issued for this app"))
		return
	}
	if info.Sub == "" || info.EmailVerified != "true" {
		httpx.WriteError(w, httpx.BadRequest("invalid user info from google"))
		return
	}

	user, err := h.auth.AuthenticateOrRegisterWithGoogle(r.Context(), info.Sub, info.Email, info.EmailVerified == "true", info.Name)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	token, err := h.setSessionCookie(w, user)
	if err != nil {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authResponse{userResponse: toUserResponse(user), Token: token})
}

type appleMobileLoginRequest struct {
	IdentityToken string `json:"identity_token"`
	// FullName is only ever sent on the very first sign-in: Apple gives the
	// name to the app once, never again, and never inside the token.
	FullName string `json:"full_name"`
}

// AppleMobileLogin is the "Sign in with Apple" counterpart to
// GoogleMobileLogin: the iOS app runs Apple's own native sheet
// (expo-apple-authentication) and posts the resulting identity token here,
// which we verify against Apple's public keys (see
// adapters/security.AppleVerifier). Public, like the other login routes.
//
// App Store guideline 4.8 requires this as soon as the app offers a
// third-party login such as Google.
func (h *AuthHandler) AppleMobileLogin(w http.ResponseWriter, r *http.Request) {
	if h.apple == nil || !h.apple.Configured() {
		httpx.WriteError(w, httpx.BadRequest("sign in with apple is not configured"))
		return
	}

	var body appleMobileLoginRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if body.IdentityToken == "" {
		httpx.WriteError(w, httpx.BadRequest("identity_token is required"))
		return
	}

	identity, err := h.apple.Verify(r.Context(), body.IdentityToken)
	if err != nil {
		if errors.Is(err, security.ErrAppleTokenInvalid) {
			httpx.WriteError(w, httpx.Unauthorized("invalid apple identity token"))
			return
		}
		httpx.WriteError(w, err)
		return
	}

	user, err := h.auth.AuthenticateOrRegisterWithApple(r.Context(), identity.Sub, identity.Email, identity.EmailVerified, strings.TrimSpace(body.FullName))
	if err != nil {
		writeAuthError(w, err)
		return
	}

	token, err := h.setSessionCookie(w, user)
	if err != nil {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authResponse{userResponse: toUserResponse(user), Token: token})
}

// robineNativeCallbackURL is where the mobile app's in-app browser (see
// expo-web-browser's openAuthSessionAsync, called with this exact URL as
// its returnUrl on native) watches for a redirect — "mobile" is this Expo
// app's own url scheme (see mobile/app.json's "scheme"), not something
// Robine ID needs to know about: Robine only ever redirects back to
// RobineCallback below, which then issues *this* redirect itself once the
// login is done, same-origin, no extra client registered on Robine's side.
//
// The app never delivers its own session over a cookie for Robine (see
// isAllowedRobineCallback below) — its API client is bearer-token-only
// (see mobile/src/api/client.ts), on web too, so a cookie GoogleCallback-
// style would set something nothing ever reads. Both platforms instead get
// the token appended to their own callback URL's query string: this
// constant for native (intercepted by the OS before it's ever "loaded" as
// a page), or "{CORSOrigin}/auth-callback" for web (a real same-origin page
// in the Expo Router app, popped up via window.open — see
// mobile/src/app/auth-callback.tsx — that calls expo-web-browser's
// maybeCompleteAuthSession() to hand the URL back to the tab that opened
// it and closes itself; a custom scheme can't be to navigated to in a
// browser tab, which is why web needs its own real URL here rather than
// reusing the native one).
//
// The scheme is the app's own reverse-DNS bundle identifier (see
// mobile/app.json's "scheme"), not a generic word like "mobile": iOS and
// Android let any installed app claim any custom scheme, and a common one
// would let another app intercept this redirect — and the session token in
// it.
const robineNativeCallbackURL = "dev.bailleul.budget://auth-callback"

// isAllowedRobineCallback bounds RobineLogin's "callback" query param (see
// below) to exactly the two URLs above, plus this server's own OAuth
// consent flow (see OAuthHandler.Resume) — never an attacker-supplied one,
// which would otherwise let a crafted login link redirect a freshly minted
// session token anywhere.
func (h *AuthHandler) isAllowedRobineCallback(r *http.Request, callback string) bool {
	return callback == robineNativeCallbackURL || callback == h.settings.CORSOrigin+"/auth-callback" ||
		callback == requestOrigin(r)+"/oauth/resume"
}

// oidcDiscoveryDocument is the subset of a standard
// /.well-known/openid-configuration response this handler needs. Robine ID
// is a generic self-hosted OIDC provider (github.com/laibulle/robine-id,
// see ../silo01/archived/roles/robine_id) without Google-style fixed,
// documented endpoints, so — unlike GoogleLogin/GoogleCallback above, which
// hardcode accounts.google.com URLs — every endpoint here is discovered.
type oidcDiscoveryDocument struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

func (h *AuthHandler) discoverRobine() (*oidcDiscoveryDocument, error) {
	h.robineDiscoveryMu.Lock()
	defer h.robineDiscoveryMu.Unlock()
	if h.robineDiscovery != nil {
		return h.robineDiscovery, nil
	}

	resp, err := http.Get(h.settings.RobineIssuer + "/.well-known/openid-configuration")
	if err != nil {
		return nil, fmt.Errorf("fetch robine discovery document: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("robine discovery document returned status %d", resp.StatusCode)
	}

	var doc oidcDiscoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode robine discovery document: %w", err)
	}
	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" || doc.UserinfoEndpoint == "" {
		return nil, fmt.Errorf("robine discovery document is missing required endpoints")
	}
	h.robineDiscovery = &doc
	return h.robineDiscovery, nil
}

func (h *AuthHandler) getRobineRedirectURI(r *http.Request) string {
	if h.settings.RobineRedirectURI != "" {
		return h.settings.RobineRedirectURI
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/auth/oidc/robine/callback", scheme, r.Host)
}

// RobineLogin starts the browser-redirect OIDC dance against Robine ID —
// the only flow this provider has (unlike Google, there's no separate
// native SDK/ID-token shortcut). The caller (see mobile/src/auth/robine.ts)
// must pass a "callback" query param naming exactly where it wants the
// session token delivered afterwards (see isAllowedRobineCallback) —
// carried across the round trip to RobineCallback via its own short-lived
// cookie, since Robine only ever echoes back the "state" param we give it,
// never anything else of ours.
func (h *AuthHandler) RobineLogin(w http.ResponseWriter, r *http.Request) {
	if h.settings.RobineClientID == "" {
		httpx.WriteError(w, httpx.BadRequest("robine auth is not configured"))
		return
	}
	callback := r.URL.Query().Get("callback")
	if !h.isAllowedRobineCallback(r, callback) {
		httpx.WriteError(w, httpx.BadRequest("invalid or missing callback"))
		return
	}
	discovery, err := h.discoverRobine()
	if err != nil {
		httpx.WriteError(w, fmt.Errorf("robine discovery failed: %w", err))
		return
	}

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := hex.EncodeToString(b)

	http.SetCookie(w, &http.Cookie{
		Name:     "robine_oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.settings.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "robine_oauth_callback",
		Value:    callback,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.settings.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	redirectURI := h.getRobineRedirectURI(r)
	authURL := fmt.Sprintf(
		"%s?client_id=%s&response_type=code&scope=%s&redirect_uri=%s&state=%s",
		discovery.AuthorizationEndpoint,
		url.QueryEscape(h.settings.RobineClientID),
		url.QueryEscape("openid email profile"),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

type robineTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
}

type robineUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// RobineCallback exchanges the authorization code for an access token,
// fetches the user's claims, then redirects to whichever callback URL
// RobineLogin stashed in the "robine_oauth_callback" cookie, with the
// resulting session token appended as a query param — see
// robineNativeCallbackURL's comment for why this is a redirect-with-token
// rather than GoogleCallback's HttpOnly-cookie approach.
func (h *AuthHandler) RobineCallback(w http.ResponseWriter, r *http.Request) {
	if h.settings.RobineClientID == "" {
		httpx.WriteError(w, httpx.BadRequest("robine auth is not configured"))
		return
	}

	callbackCookie, err := r.Cookie("robine_oauth_callback")
	if err != nil || !h.isAllowedRobineCallback(r, callbackCookie.Value) {
		httpx.WriteError(w, httpx.BadRequest("invalid or missing callback cookie"))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "robine_oauth_callback", Value: "", Path: "/", MaxAge: -1})

	stateCookie, err := r.Cookie("robine_oauth_state")
	if err != nil || stateCookie.Value == "" {
		httpx.WriteError(w, httpx.BadRequest("invalid state cookie"))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "robine_oauth_state", Value: "", Path: "/", MaxAge: -1})

	if r.URL.Query().Get("state") != stateCookie.Value {
		httpx.WriteError(w, httpx.BadRequest("state mismatch"))
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		httpx.WriteError(w, httpx.BadRequest("missing code parameter"))
		return
	}

	discovery, err := h.discoverRobine()
	if err != nil {
		httpx.WriteError(w, fmt.Errorf("robine discovery failed: %w", err))
		return
	}

	redirectURI := h.getRobineRedirectURI(r)
	resp, err := http.PostForm(discovery.TokenEndpoint, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {h.settings.RobineClientID},
		"client_secret": {h.settings.RobineClientSecret},
		"redirect_uri":  {redirectURI},
	})
	if err != nil {
		httpx.WriteError(w, fmt.Errorf("failed robine token request: %w", err))
		return
	}
	defer resp.Body.Close()

	var tokenRes robineTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		httpx.WriteError(w, fmt.Errorf("failed decoding robine token response: %w", err))
		return
	}
	if tokenRes.Error != "" || tokenRes.AccessToken == "" {
		httpx.WriteError(w, httpx.BadRequest("failed robine authentication token exchange"))
		return
	}

	userInfoReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, discovery.UserinfoEndpoint, nil)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	userInfoReq.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)

	userInfoResp, err := http.DefaultClient.Do(userInfoReq)
	if err != nil {
		httpx.WriteError(w, fmt.Errorf("failed fetching robine user info: %w", err))
		return
	}
	defer userInfoResp.Body.Close()

	var userInfo robineUserInfo
	if err := json.NewDecoder(userInfoResp.Body).Decode(&userInfo); err != nil {
		httpx.WriteError(w, fmt.Errorf("failed decoding robine user info: %w", err))
		return
	}
	if userInfo.Sub == "" {
		httpx.WriteError(w, httpx.BadRequest("invalid user info from robine"))
		return
	}

	user, err := h.auth.AuthenticateOrRegisterWithRobine(r.Context(), userInfo.Sub, userInfo.Email, userInfo.EmailVerified, userInfo.Name)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	token, err := h.tokens.Create(user.ID, user.TokenVersion)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	http.Redirect(w, r, callbackCookie.Value+"?token="+url.QueryEscape(token), http.StatusFound)
}
