package core

import (
	"reflect"
	"testing"
)

func TestGetEnv(t *testing.T) {
	t.Setenv("BUDGET_TEST_STRING", "hello")
	if got := getEnv("BUDGET_TEST_STRING", "fallback"); got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
	if got := getEnv("BUDGET_TEST_STRING_UNSET", "fallback"); got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("BUDGET_TEST_INT", "42")
	if got := getEnvInt("BUDGET_TEST_INT", 0); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
	if got := getEnvInt("BUDGET_TEST_INT_UNSET", 7); got != 7 {
		t.Errorf("got %d, want 7 (fallback)", got)
	}

	t.Setenv("BUDGET_TEST_INT_BAD", "not-a-number")
	if got := getEnvInt("BUDGET_TEST_INT_BAD", 7); got != 7 {
		t.Errorf("got %d, want 7 (fallback for an unparsable value)", got)
	}
}

func TestGetEnvBool(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
	}
	for _, tc := range cases {
		t.Setenv("BUDGET_TEST_BOOL", tc.value)
		if got := getEnvBool("BUDGET_TEST_BOOL", !tc.want); got != tc.want {
			t.Errorf("getEnvBool(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}

	if got := getEnvBool("BUDGET_TEST_BOOL_UNSET", true); got != true {
		t.Errorf("got %v, want true (fallback)", got)
	}

	t.Setenv("BUDGET_TEST_BOOL_BAD", "not-a-bool")
	if got := getEnvBool("BUDGET_TEST_BOOL_BAD", true); got != true {
		t.Errorf("got %v, want true (fallback for an unparsable value)", got)
	}
}

func TestSplitNonEmpty(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", nil},
		{"single value", "a", []string{"a"}},
		{"multiple values", "a,b,c", []string{"a", "b", "c"}},
		{"trims whitespace", " a , b ,c ", []string{"a", "b", "c"}},
		{"drops empty entries", "a,,b,", []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitNonEmpty(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitNonEmpty(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestSettings_CORSAllowOrigins(t *testing.T) {
	s := Settings{CORSOrigin: "https://lagune.bailleul.dev", ExtraCORSOrigins: "http://localhost:5173, http://localhost:8081"}
	got := s.CORSAllowOrigins()
	want := []string{"https://lagune.bailleul.dev", "http://localhost:5173", "http://localhost:8081"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSettings_CORSAllowOrigins_NoExtras(t *testing.T) {
	s := Settings{CORSOrigin: "https://lagune.bailleul.dev"}
	got := s.CORSAllowOrigins()
	want := []string{"https://lagune.bailleul.dev"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSettings_GoogleAcceptedAudiences(t *testing.T) {
	s := Settings{GoogleClientID: "web-client-id", GoogleMobileClientIDs: []string{"ios-client-id", "android-client-id"}}
	got := s.GoogleAcceptedAudiences()
	want := []string{"web-client-id", "ios-client-id", "android-client-id"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// An unconfigured GoogleClientID ("") must never itself become a valid
// audience an ID token could match against.
func TestSettings_GoogleAcceptedAudiences_EmptyClientIDExcluded(t *testing.T) {
	s := Settings{GoogleClientID: "", GoogleMobileClientIDs: []string{"ios-client-id"}}
	got := s.GoogleAcceptedAudiences()
	want := []string{"ios-client-id"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSettings_GoogleAcceptedAudiences_NothingConfigured(t *testing.T) {
	s := Settings{}
	got := s.GoogleAcceptedAudiences()
	if len(got) != 0 {
		t.Errorf("got %v, want an empty slice", got)
	}
}

// LoadSettings must accept SQLAlchemy's "postgresql[+asyncpg]://" schemes
// (a leftover from .env.example's original format) and normalize them to
// the plain "postgres://" pgx wants — set explicitly via t.Setenv, which
// LoadSettings' own loadDotEnv() never overrides (godotenv only fills in
// vars that aren't already set), so this doesn't depend on whatever the
// real backend/.env on this machine happens to contain.
func TestLoadSettings_NormalizesDatabaseURLScheme(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"asyncpg scheme", "postgresql+asyncpg://user:pass@host/db", "postgres://user:pass@host/db"},
		{"plain postgresql scheme", "postgresql://user:pass@host/db", "postgres://user:pass@host/db"},
		{"already postgres scheme", "postgres://user:pass@host/db", "postgres://user:pass@host/db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tc.input)
			got := LoadSettings().DatabaseURL
			if got != tc.want {
				t.Errorf("DatabaseURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoadSettings_RobineIssuerTrailingSlashTrimmed(t *testing.T) {
	t.Setenv("ROBINE_ISSUER", "https://id.bailleul.dev/default/")
	got := LoadSettings().RobineIssuer
	if got != "https://id.bailleul.dev/default" {
		t.Errorf("RobineIssuer = %q, want the trailing slash trimmed", got)
	}
}

func TestLoadSettings_Defaults(t *testing.T) {
	// Every var LoadSettings reads, pinned to something explicit so this
	// test's expectations don't depend on the real backend/.env.
	t.Setenv("PORT", "9999")
	t.Setenv("JWT_EXPIRE_MINUTES", "123")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("SEED_DEV_USER", "false")
	t.Setenv("GOOGLE_MOBILE_CLIENT_IDS", "")

	s := LoadSettings()
	if s.Port != "9999" {
		t.Errorf("Port = %q, want 9999", s.Port)
	}
	if s.JWTExpireMinutes != 123 {
		t.Errorf("JWTExpireMinutes = %d, want 123", s.JWTExpireMinutes)
	}
	if !s.CookieSecure {
		t.Error("CookieSecure = false, want true")
	}
	if s.SeedDevUser {
		t.Error("SeedDevUser = true, want false")
	}
	if len(s.GoogleMobileClientIDs) != 0 {
		t.Errorf("GoogleMobileClientIDs = %v, want empty", s.GoogleMobileClientIDs)
	}
}
