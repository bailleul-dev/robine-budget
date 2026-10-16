package core

import (
	"os"
	"strings"
	"testing"
)

func validProduction() Settings {
	return Settings{
		AppEnv:                "production",
		JWTSecret:             strings.Repeat("a", 40),
		BankSyncEncryptionKey: strings.Repeat("ab", 32),
		CookieSecure:          true,
	}
}

func TestValidate_ProductionAcceptsRealSecrets(t *testing.T) {
	if err := validProduction().Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidate_ProductionRejects(t *testing.T) {
	cases := map[string]func(*Settings){
		"default JWT secret":    func(s *Settings) { s.JWTSecret = defaultJWTSecret },
		"short JWT secret":      func(s *Settings) { s.JWTSecret = "short" },
		"all-zero encryption":   func(s *Settings) { s.BankSyncEncryptionKey = strings.Repeat("0", 64) },
		"insecure cookie":       func(s *Settings) { s.CookieSecure = false },
		"dev seed user enabled": func(s *Settings) { s.SeedDevUser = true },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := validProduction()
			mutate(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
		})
	}
}

func TestValidate_DevelopmentToleratesPlaceholders(t *testing.T) {
	s := Settings{AppEnv: "development", JWTSecret: defaultJWTSecret, BankSyncEncryptionKey: strings.Repeat("0", 64)}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil in development", err)
	}
}

func TestValidate_UnknownEnvRejected(t *testing.T) {
	if err := (Settings{AppEnv: "staging"}).Validate(); err == nil {
		t.Fatal("Validate() = nil, want an error for an unknown APP_ENV")
	}
}

func TestLoadSettings_EmptyAppEnvMeansProductionWithSecureCookie(t *testing.T) {
	t.Setenv("APP_ENV", "")
	os.Unsetenv("COOKIE_SECURE")
	s := LoadSettings()
	if s.AppEnv != "production" || !s.IsProduction() {
		t.Errorf("AppEnv = %q, want production", s.AppEnv)
	}
	if !s.CookieSecure {
		t.Error("CookieSecure = false, want true by default in production")
	}
}

func TestSignupAllowed(t *testing.T) {
	s := Settings{SignupAllowedEmails: []string{"Friend@Example.com", "other@example.com"}}
	for email, want := range map[string]bool{
		"friend@example.com":   true,
		" FRIEND@example.com ": true,
		"other@example.com":    true,
		"stranger@example.com": false,
		"":                     false, // a hidden Apple address can't be matched
	} {
		if got := s.SignupAllowed(email); got != want {
			t.Errorf("SignupAllowed(%q) = %v, want %v", email, got, want)
		}
	}
	if (Settings{}).SignupAllowed("anyone@example.com") {
		t.Error("an empty list must mean closed")
	}
	open := Settings{SignupAllowedEmails: []string{"*"}}
	if !open.SignupAllowed("anyone@example.com") || !open.SignupAllowed("") {
		t.Error("\"*\" must open registration, including for hidden addresses")
	}
}

func TestLoadSettings_SignupDefaultsByEnvironment(t *testing.T) {
	os.Unsetenv("SIGNUP_ALLOWED_EMAILS")
	t.Setenv("APP_ENV", "production")
	if got := LoadSettings().SignupAllowedEmails; len(got) != 0 {
		t.Errorf("production default = %v, want closed", got)
	}
	t.Setenv("APP_ENV", "development")
	if got := LoadSettings().SignupAllowedEmails; len(got) != 1 || got[0] != "*" {
		t.Errorf("development default = %v, want open", got)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("SIGNUP_ALLOWED_EMAILS", "a@example.com, b@example.com")
	if got := LoadSettings().SignupAllowedEmails; len(got) != 2 {
		t.Errorf("explicit list = %v, want 2 addresses", got)
	}
	t.Setenv("SIGNUP_ALLOWED_EMAILS", "")
	if got := LoadSettings().SignupAllowedEmails; len(got) != 0 {
		t.Errorf("explicitly empty = %v, want closed", got)
	}
}
