package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testBundleID = "dev.bailleul.budget"

// appleStub stands in for Apple's key endpoint, serving one RSA key.
func appleStub(t *testing.T, kid string) (*rsa.PrivateKey, *AppleVerifier) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": kid,
			"kty": "RSA",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(server.Close)

	verifier := NewAppleVerifier([]string{testBundleID})
	verifier.keysURL = server.URL
	return key, verifier
}

func appleToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":   AppleIssuer,
		"aud":   testBundleID,
		"sub":   "001234.abcdef",
		"email": "camille@privaterelay.appleid.com",
		"exp":   time.Now().Add(10 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	}
}

func TestAppleVerifier_Verify(t *testing.T) {
	key, verifier := appleStub(t, "key-1")

	identity, err := verifier.Verify(context.Background(), appleToken(t, key, "key-1", validClaims()))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.Sub != "001234.abcdef" || identity.Email != "camille@privaterelay.appleid.com" {
		t.Errorf("identity = %+v", identity)
	}
}

func TestAppleVerifier_Verify_Rejects(t *testing.T) {
	key, verifier := appleStub(t, "key-1")
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	tokens := map[string]string{
		"another app's audience": appleToken(t, key, "key-1", mutate(validClaims(), "aud", "com.someone.else")),
		"another issuer":         appleToken(t, key, "key-1", mutate(validClaims(), "iss", "https://evil.example")),
		"expired":                appleToken(t, key, "key-1", mutate(validClaims(), "exp", time.Now().Add(-time.Minute).Unix())),
		"no subject":             appleToken(t, key, "key-1", mutate(validClaims(), "sub", "")),
		"unknown key id":         appleToken(t, key, "other-key", validClaims()),
		"signed by someone else": appleToken(t, other, "key-1", validClaims()),
	}
	for name, token := range tokens {
		if _, err := verifier.Verify(context.Background(), token); !errors.Is(err, ErrAppleTokenInvalid) {
			t.Errorf("%s: err = %v, want ErrAppleTokenInvalid", name, err)
		}
	}
}

func TestAppleVerifier_Verify_Unconfigured(t *testing.T) {
	verifier := NewAppleVerifier(nil)
	if verifier.Configured() {
		t.Fatal("no bundle id should mean unconfigured")
	}
	if _, err := verifier.Verify(context.Background(), "whatever"); err == nil {
		t.Fatal("expected an error")
	}
}

func mutate(claims jwt.MapClaims, key string, value any) jwt.MapClaims {
	claims[key] = value
	return claims
}
