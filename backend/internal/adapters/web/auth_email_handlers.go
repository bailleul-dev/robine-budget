package web

import (
	"context"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/httpx"
)

// The emailed flows land on small server-rendered pages, not in the mobile
// app: a link in an email has to work on any device, installed app or not.
// They use no JavaScript and no cookie. Two design points:
//
//   - The GET only *shows* a form; consuming the single-use token happens on
//     the POST. Mail providers and antivirus gateways pre-fetch every link in
//     a message, and a GET that consumed the token would burn it before the
//     person ever clicked.
//   - The token in the form body is itself the secret that authorizes the
//     POST, so no separate CSRF token is needed: a cross-site attacker has
//     no way to know it.

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

// ForgotPassword always answers the same 200, whether or not the address has
// an account, and does the work after responding so that the response time
// doesn't reveal it either.
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body forgotPasswordRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if ok, retry := h.mailLimiter.take("reset|" + email); !ok {
		// Same body as success: don't tell an attacker they hit the cap for
		// a *real* address. The mail simply isn't sent again.
		_ = retry
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.auth.RequestPasswordReset(ctx, email); err != nil {
			log.Printf("auth: password reset request failed: %v", err)
		}
	}()
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ResendVerification sends the signed-in user a fresh verification link.
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	if user.EmailVerified() {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if ok, retry := h.mailLimiter.take("verify|" + user.ID.String()); !ok {
		writeTooManyRequests(w, retry)
		return
	}
	if err := h.auth.ResendVerification(r.Context(), user); err != nil {
		log.Printf("auth: resend verification for %s: %v", user.ID, err)
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="fr"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Title}} — Lagune</title>
<style>
:root{color-scheme:light dark;--bg:#f4f7f7;--card:#fff;--ink:#12302f;--muted:#5b7473;--accent:#0f766e;--err:#b42318;--line:#d7e3e2}
@media (prefers-color-scheme:dark){:root{--bg:#0d1b1b;--card:#132727;--ink:#e6f2f1;--muted:#93b0ae;--accent:#2dd4bf;--err:#fda29b;--line:#254140}}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:grid;place-items:center;padding:24px 16px;background:var(--bg);color:var(--ink);font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
main{width:100%;max-width:420px;background:var(--card);border:1px solid var(--line);border-radius:24px;padding:32px 28px;box-shadow:0 12px 40px rgba(15,118,110,.10)}
h1{margin:0 0 8px;font-size:1.4rem}
p{margin:0 0 16px;color:var(--muted)}
label{display:block;margin:0 0 6px;font-weight:600;font-size:.9rem}
input[type=password]{width:100%;padding:12px 14px;margin-bottom:16px;border:1px solid var(--line);border-radius:14px;background:transparent;color:var(--ink);font:inherit}
button{width:100%;padding:13px;border:0;border-radius:14px;background:var(--accent);color:#fff;font:inherit;font-weight:700;cursor:pointer}
@media (prefers-color-scheme:dark){button{color:#052423}}
.error{color:var(--err);font-weight:600}
.ok{color:var(--accent);font-weight:600}
</style></head>
<body><main>
<h1>{{.Title}}</h1>
{{if .Message}}<p class="{{.Class}}">{{.Message}}</p>{{end}}
{{if eq .Form "verify"}}
<form method="post" action="/verify-email"><input type="hidden" name="token" value="{{.Token}}"><button type="submit">Confirmer mon adresse</button></form>
{{end}}
{{if eq .Form "reset"}}
<form method="post" action="/reset-password" autocomplete="off">
<input type="hidden" name="token" value="{{.Token}}">
<label for="password">Nouveau mot de passe</label>
<input id="password" name="password" type="password" minlength="10" maxlength="72" autocomplete="new-password" required>
<label for="confirm">Confirmer le mot de passe</label>
<input id="confirm" name="confirm" type="password" minlength="10" maxlength="72" autocomplete="new-password" required>
<p>10 caractères minimum.</p>
<button type="submit">Changer mon mot de passe</button>
</form>
{{end}}
</main></body></html>`))

type pageData struct {
	Title, Message, Class, Form, Token string
}

func renderPage(w http.ResponseWriter, status int, d pageData) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.WriteHeader(status)
	if err := pageTemplate.Execute(w, d); err != nil {
		log.Printf("auth pages: render: %v", err)
	}
}

func (h *AuthHandler) VerifyEmailPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		renderPage(w, http.StatusBadRequest, pageData{Title: "Lien invalide", Message: "Ce lien de confirmation est incomplet.", Class: "error"})
		return
	}
	renderPage(w, http.StatusOK, pageData{Title: "Confirmer votre adresse", Message: "Un dernier clic pour activer votre compte Lagune.", Form: "verify", Token: token})
}

func (h *AuthHandler) VerifyEmailSubmit(w http.ResponseWriter, r *http.Request) {
	err := h.auth.VerifyEmail(r.Context(), r.PostFormValue("token"))
	switch {
	case err == nil:
		renderPage(w, http.StatusOK, pageData{Title: "Adresse confirmée", Message: "Merci ! Votre adresse est confirmée, vous pouvez retourner dans l'application.", Class: "ok"})
	case errors.Is(err, services.ErrInvalidToken):
		renderPage(w, http.StatusBadRequest, pageData{Title: "Lien expiré", Message: "Ce lien n'est plus valable. Demandez-en un nouveau depuis l'application.", Class: "error"})
	default:
		log.Printf("auth pages: verify email: %v", err)
		renderPage(w, http.StatusInternalServerError, pageData{Title: "Erreur", Message: "Une erreur est survenue. Veuillez réessayer.", Class: "error"})
	}
}

func (h *AuthHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		renderPage(w, http.StatusBadRequest, pageData{Title: "Lien invalide", Message: "Ce lien de réinitialisation est incomplet.", Class: "error"})
		return
	}
	renderPage(w, http.StatusOK, pageData{Title: "Nouveau mot de passe", Form: "reset", Token: token})
}

func (h *AuthHandler) ResetPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	token := r.PostFormValue("token")
	password := r.PostFormValue("password")
	if password != r.PostFormValue("confirm") {
		renderPage(w, http.StatusBadRequest, pageData{Title: "Nouveau mot de passe", Message: "Les deux mots de passe ne correspondent pas.", Class: "error", Form: "reset", Token: token})
		return
	}
	_, err := h.auth.ResetPassword(r.Context(), token, password)
	var invalid *services.ValidationError
	switch {
	case err == nil:
		renderPage(w, http.StatusOK, pageData{Title: "Mot de passe modifié", Message: "C'est fait. Vous avez été déconnecté de tous vos appareils : reconnectez-vous avec votre nouveau mot de passe.", Class: "ok"})
	case errors.As(err, &invalid):
		renderPage(w, http.StatusBadRequest, pageData{Title: "Nouveau mot de passe", Message: "Mot de passe refusé : 10 caractères minimum (72 octets maximum).", Class: "error", Form: "reset", Token: token})
	case errors.Is(err, services.ErrInvalidToken):
		renderPage(w, http.StatusBadRequest, pageData{Title: "Lien expiré", Message: "Ce lien n'est plus valable. Refaites une demande depuis l'écran de connexion.", Class: "error"})
	default:
		log.Printf("auth pages: reset password: %v", err)
		renderPage(w, http.StatusInternalServerError, pageData{Title: "Erreur", Message: "Une erreur est survenue. Veuillez réessayer.", Class: "error"})
	}
}
