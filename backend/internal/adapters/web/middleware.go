// Package web holds the HTTP layer — net/http handlers and middleware.
// Mirrors backend/app/adapters/web/routes/*.py and
// backend/app/core/dependencies.py's auth/ownership dependencies.
package web

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/core"
	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
)

type ctxKey int

const userCtxKey ctxKey = iota

// sessionTokenFrom reads the session token from wherever the caller put it:
// the HttpOnly cookie the web app relies on, or an "Authorization: Bearer
// <token>" header — the only option for a native (Expo) client, which has
// no browser cookie jar and stores the token itself (see authResponse).
func sessionTokenFrom(r *http.Request) (string, bool) {
	if cookie, err := r.Cookie(security.SessionCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, true
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer "), true
	}
	return "", false
}

// AuthMiddleware decodes the session token, loads the user, and stores it
// on the request context — mirrors get_current_user in dependencies.py.
// Unauthenticated requests get a 401 before reaching any handler.
func AuthMiddleware(users ports.UserRepository, tokens security.SessionTokens) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString, ok := sessionTokenFrom(r)
			if !ok {
				http.Error(w, `{"detail":"not authenticated"}`, http.StatusUnauthorized)
				return
			}
			userID, version, err := tokens.Decode(tokenString)
			if err != nil {
				http.Error(w, `{"detail":"not authenticated"}`, http.StatusUnauthorized)
				return
			}
			user, err := users.GetByID(r.Context(), userID)
			if err != nil || user == nil || user.TokenVersion != version {
				http.Error(w, `{"detail":"not authenticated"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
		})
	}
}

func withUser(ctx context.Context, user *entities.User) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

func CurrentUser(r *http.Request) *entities.User {
	user, _ := r.Context().Value(userCtxKey).(*entities.User)
	return user
}

func CORSMiddleware(settings core.Settings) func(http.Handler) http.Handler {
	allowed := settings.CORSAllowOrigins()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if slices.Contains(allowed, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				// A literal "*" is invalid here once Allow-Credentials is
				// true (the spec forbids the wildcard on any of these three
				// headers alongside credentialed requests, and browsers
				// enforce it — this was silently failing every preflight
				// for a header as ordinary as Content-Type). Echo back
				// whatever the browser is actually asking to send instead.
				if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
					w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
				} else {
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(r.PathValue(name))
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
