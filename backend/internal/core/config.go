// Package core holds cross-cutting composition-root concerns: config and
// the system clock.
package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// loadDotEnv loads backend/.env, regardless of which directory the process
// is started from — the repo root (`make dev`) or backend/ itself (running
// air directly). Never overrides an env var the shell/CI already set, and
// silently no-ops if neither path exists.
func loadDotEnv() {
	for _, path := range []string{"backend/.env", ".env"} {
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Load(path)
			return
		}
	}
}

type Settings struct {
	// AppEnv is "production" (the default, so a forgotten variable can never
	// silently weaken a deployment) or "development". Only production runs
	// the strict checks in Validate, and only development tolerates the
	// placeholder secrets below.
	AppEnv string

	Port        string
	DatabaseURL string

	// PublicURL is the absolute origin users reach the app on (e.g.
	// "https://lagune.example.com"): the base of the links in verification
	// and password-reset emails. Empty derives nothing — links then can't be
	// built and those emails are not sent.
	PublicURL string

	// SMTP* configure outgoing mail (verification, password reset). With no
	// SMTPHost, mails are only written to the server log (development).
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	JWTSecret        string
	JWTExpireMinutes int

	// TrustedProxyHops is how many reverse proxies sit in front of the server
	// (Traefik = 1, Cloudflare + Traefik = 2). It decides which
	// X-Forwarded-For entry is the real client for rate limiting; 0 ignores
	// the header, which is the only safe value when the server is exposed
	// directly.
	TrustedProxyHops int

	// SignupAllowedEmails is who may CREATE an account (existing accounts always
	// sign in): "*" opens registration to everybody, otherwise a comma-separated
	// list of addresses (case-insensitive). Unset means closed in production and
	// open in development. It gates every way of creating an account — password,
	// Google, Apple, Robine ID — which is what makes a private beta private.
	SignupAllowedEmails []string

	CORSOrigin       string
	ExtraCORSOrigins string
	CookieSecure     bool

	SeedDevUser         bool
	SeedDevUserEmail    string
	SeedDevUserPassword string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
	// GoogleMobileClientIDs are additional OAuth client IDs the mobile app's
	// native Google sign-in (expo-auth-session) may present an ID token
	// for — Google issues a separate client ID per platform (Expo web, iOS,
	// Android), each producing tokens whose "aud" claim is that platform's
	// own client ID, never GoogleClientID (the web redirect flow's own
	// client). See AuthHandler.GoogleMobileLogin.
	GoogleMobileClientIDs []string

	// RobineIssuer/ClientID/ClientSecret/RedirectURI configure "Sign in with
	// Robine ID" — the self-hosted OIDC provider at id.base59.dev (see
	// ../silo01/archived/roles/robine_id and roles/budget's k8s manifest,
	// which already sets these four env vars once budget_oidc_enabled is
	// true). Unlike Google, Robine's endpoints aren't hardcoded here: they're
	// discovered from RobineIssuer + "/.well-known/openid-configuration" at
	// first use (see AuthHandler's oidcDiscovery), since it's a generic OIDC
	// server rather than a well-known one with documented fixed endpoints.
	RobineIssuer       string
	RobineClientID     string
	RobineClientSecret string
	RobineRedirectURI  string

	// AppleBundleIDs lists the app bundle identifiers a "Sign in with
	// Apple" identity token may be issued for (its "aud" claim) — normally
	// just the iOS app's own, comma-separated if it ever ships under more.
	// Empty disables Apple sign-in: POST /api/auth/apple/mobile then
	// answers 400 and the app hides the button (see GET /api/auth/config).
	AppleBundleIDs []string

	// BankSyncPythonBin/BankSyncProjectDir must be absolute paths: the
	// bank_sync CLI is invoked as `<PythonBin> -m bank_sync ...` with Dir
	// set to ProjectDir (the directory containing bank_sync/), independent
	// of the server's own working directory (which differs between `make
	// dev`, running the built binary, and the Docker image). The
	// subprocess is a port (stdin/stdout only, see adapters/banksync) - no
	// storage directory to configure. In the Docker image these are baked
	// in as ENV; for local dev set them in backend/.env to point at the
	// repo's own .venv (see .env.example).
	BankSyncPythonBin  string
	BankSyncProjectDir string

	// BankSyncEncryptionKey is a 64-character hex string (32 bytes, for
	// AES-256-GCM) used to encrypt bank credentials and session state at
	// rest (see adapters/security.SecretBox) - change this for any real
	// deployment, same as JWTSecret above.
	BankSyncEncryptionKey string
	// BankSyncAllowedEmails lists the (verified) accounts allowed to use bank
	// sync. Empty — the default — closes it to everyone: it stores online
	// banking credentials and scrapes the bank, which is only acceptable for
	// the operator's own account, not for the public.
	BankSyncAllowedEmails []string

	// OTAUpdatesDir is where `make ota-publish` stores the mobile app's
	// over-the-air updates (see adapters/otaupdates for the layout). Set
	// either this or OTAS3Bucket (not both); with neither, the
	// /api/updates routes are disabled (they answer 404). In the Docker
	// image, point it at a mounted volume so updates survive a redeploy.
	OTAUpdatesDir string
	// OTAS3* store the same updates in an S3-compatible bucket instead
	// (Garage in production, see ../silo01/roles/garage). The server only
	// needs read access. OTAS3Region defaults to "garage", Garage's own
	// default; OTAS3Prefix is optional.
	OTAS3Endpoint        string
	OTAS3Region          string
	OTAS3Bucket          string
	OTAS3AccessKeyID     string
	OTAS3SecretAccessKey string
	OTAS3Prefix          string
	// OTACodeSigningKeyPath is the PEM private key from `make ota-keys`.
	// Optional, but builds whose app config embeds the matching certificate
	// refuse any update that isn't signed with it — set it in production.
	OTACodeSigningKeyPath string
	// OTAPublicURL is the absolute origin asset download URLs are built on
	// (e.g. "https://budget.example.com"). Empty derives it from each
	// request's Host/X-Forwarded-* headers.
	OTAPublicURL string
}

func LoadSettings() Settings {
	loadDotEnv()

	appEnv := strings.ToLower(strings.TrimSpace(getEnv("APP_ENV", envProduction)))
	if appEnv == "" {
		appEnv = envProduction
	}

	s := Settings{
		AppEnv:              appEnv,
		PublicURL:           strings.TrimRight(getEnv("PUBLIC_URL", ""), "/"),
		SMTPHost:            getEnv("SMTP_HOST", ""),
		SMTPPort:            getEnvInt("SMTP_PORT", 587),
		SMTPUsername:        getEnv("SMTP_USERNAME", ""),
		SMTPPassword:        getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:            getEnv("SMTP_FROM", ""),
		Port:                getEnv("PORT", "8000"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://budget:budget@localhost:5432/budget"),
		JWTSecret:           getEnv("JWT_SECRET", "change-me-to-a-long-random-string"),
		JWTExpireMinutes:    getEnvInt("JWT_EXPIRE_MINUTES", 10080),
		SignupAllowedEmails: signupAllowed(appEnv),
		TrustedProxyHops:    getEnvInt("TRUSTED_PROXY_HOPS", 0),
		CORSOrigin:          getEnv("CORS_ORIGIN", "http://localhost:5173"),
		ExtraCORSOrigins:    getEnv("EXTRA_CORS_ORIGINS", ""),
		CookieSecure:        getEnvBool("COOKIE_SECURE", appEnv == "production"),

		SeedDevUser:         getEnvBool("SEED_DEV_USER", false),
		SeedDevUserEmail:    getEnv("SEED_DEV_USER_EMAIL", "dev@example.com"),
		SeedDevUserPassword: getEnv("SEED_DEV_USER_PASSWORD", "devpass123"),

		GoogleClientID:        getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:    getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURI:     getEnv("GOOGLE_REDIRECT_URI", ""),
		GoogleMobileClientIDs: splitNonEmpty(getEnv("GOOGLE_MOBILE_CLIENT_IDS", "")),

		RobineIssuer:       strings.TrimRight(getEnv("ROBINE_ISSUER", ""), "/"),
		RobineClientID:     getEnv("ROBINE_CLIENT_ID", ""),
		RobineClientSecret: getEnv("ROBINE_CLIENT_SECRET", ""),
		RobineRedirectURI:  getEnv("ROBINE_REDIRECT_URI", ""),

		AppleBundleIDs: splitNonEmpty(getEnv("APPLE_BUNDLE_IDS", "")),

		BankSyncPythonBin:  getEnv("BANK_SYNC_PYTHON_BIN", "python3"),
		BankSyncProjectDir: getEnv("BANK_SYNC_PROJECT_DIR", "."),

		BankSyncAllowedEmails: splitNonEmpty(getEnv("BANK_SYNC_ALLOWED_EMAILS", "")),
		BankSyncEncryptionKey: getEnv("BANK_SYNC_ENCRYPTION_KEY", strings.Repeat("0", 64)),

		OTAUpdatesDir:         getEnv("OTA_UPDATES_DIR", ""),
		OTAS3Endpoint:         getEnv("OTA_S3_ENDPOINT", ""),
		OTAS3Region:           getEnv("OTA_S3_REGION", "garage"),
		OTAS3Bucket:           getEnv("OTA_S3_BUCKET", ""),
		OTAS3AccessKeyID:      getEnv("OTA_S3_ACCESS_KEY_ID", ""),
		OTAS3SecretAccessKey:  getEnv("OTA_S3_SECRET_ACCESS_KEY", ""),
		OTAS3Prefix:           getEnv("OTA_S3_PREFIX", ""),
		OTACodeSigningKeyPath: getEnv("OTA_CODE_SIGNING_KEY_PATH", ""),
		OTAPublicURL:          strings.TrimRight(getEnv("OTA_PUBLIC_URL", ""), "/"),
	}
	// Accept SQLAlchemy's "postgresql+asyncpg://" scheme too (a leftover
	// from .env.example's original format) — pgx wants a plain "postgres://" DSN.
	s.DatabaseURL = strings.Replace(s.DatabaseURL, "postgresql+asyncpg://", "postgres://", 1)
	s.DatabaseURL = strings.Replace(s.DatabaseURL, "postgresql://", "postgres://", 1)
	return s
}

const (
	defaultJWTSecret   = "change-me-to-a-long-random-string"
	minJWTSecretLength = 32
	envProduction      = "production"
	envDevelopment     = "development"
)

// IsProduction reports whether strict security checks apply.
func (s Settings) IsProduction() bool { return s.AppEnv != envDevelopment }

// Validate refuses configurations that must never reach a real deployment:
// placeholder or short secrets, insecure cookies, the dev seed account. In
// development it only rejects an unknown APP_ENV. Call it once at startup,
// before anything listens.
func (s Settings) Validate() error {
	if s.AppEnv != envProduction && s.AppEnv != envDevelopment {
		return fmt.Errorf("APP_ENV must be %q or %q, got %q", envProduction, envDevelopment, s.AppEnv)
	}
	if !s.IsProduction() {
		return nil
	}
	var problems []string
	if s.JWTSecret == defaultJWTSecret || len(s.JWTSecret) < minJWTSecretLength {
		problems = append(problems, fmt.Sprintf("JWT_SECRET must be set to a random string of at least %d characters", minJWTSecretLength))
	}
	if strings.Trim(s.BankSyncEncryptionKey, "0") == "" {
		problems = append(problems, "BANK_SYNC_ENCRYPTION_KEY must be set to a random 64-character hex string (not the all-zero placeholder)")
	}
	if !s.CookieSecure {
		problems = append(problems, "COOKIE_SECURE must not be false in production")
	}
	if s.SeedDevUser {
		problems = append(problems, "SEED_DEV_USER must not be enabled in production")
	}
	if len(problems) > 0 {
		return fmt.Errorf("refusing to start in production: %s (set APP_ENV=development for local work)", strings.Join(problems, "; "))
	}
	return nil
}

// signupAllowed reads SIGNUP_ALLOWED_EMAILS; unset defaults to closed in
// production and open ("*") in development.
func signupAllowed(appEnv string) []string {
	raw, set := os.LookupEnv("SIGNUP_ALLOWED_EMAILS")
	if !set {
		if appEnv == envDevelopment {
			return []string{"*"}
		}
		return nil
	}
	return splitNonEmpty(raw)
}

// SignupAllowed reports whether registering with this address is permitted.
// An empty address (Apple can hide it) is only allowed when registration is
// fully open, since it can't be matched against the list.
func (s Settings) SignupAllowed(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, allowed := range s.SignupAllowedEmails {
		if allowed == "*" || (email != "" && strings.EqualFold(allowed, email)) {
			return true
		}
	}
	return false
}

// GoogleAcceptedAudiences lists every OAuth client ID an incoming Google ID
// token's "aud" claim may match: the web redirect flow's own client, plus
// any mobile-specific ones (see GoogleMobileClientIDs) — empty entries
// dropped, so an unconfigured GoogleClientID doesn't become a valid ""
// audience.
func (s Settings) GoogleAcceptedAudiences() []string {
	audiences := make([]string, 0, len(s.GoogleMobileClientIDs)+1)
	if s.GoogleClientID != "" {
		audiences = append(audiences, s.GoogleClientID)
	}
	audiences = append(audiences, s.GoogleMobileClientIDs...)
	return audiences
}

func splitNonEmpty(csv string) []string {
	var out []string
	for _, v := range strings.Split(csv, ",") {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (s Settings) CORSAllowOrigins() []string {
	origins := []string{s.CORSOrigin}
	for _, o := range strings.Split(s.ExtraCORSOrigins, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
