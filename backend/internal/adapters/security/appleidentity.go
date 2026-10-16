package security

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AppleIssuer is the "iss" every Sign in with Apple identity token carries.
const AppleIssuer = "https://appleid.apple.com"

// appleKeysURL serves Apple's public signing keys (JWKS). Apple rotates
// them, so they're fetched on demand and cached, and a token signed with an
// unknown key id forces one refetch.
const appleKeysURL = "https://appleid.apple.com/auth/keys"

// ErrAppleTokenInvalid is returned for any identity token this app should
// refuse: bad signature, wrong audience or issuer, expired.
var ErrAppleTokenInvalid = errors.New("invalid apple identity token")

// AppleIdentity is what a verified identity token tells us about the user.
// Email is empty when the token doesn't carry one (Apple only includes it
// when the user agreed to share it, possibly behind a private relay).
type AppleIdentity struct {
	Sub   string
	Email string
	// EmailVerified is Apple's own claim that it verified Email (it does so
	// for every address it shares, private-relay ones included).
	EmailVerified bool
}

// AppleVerifier checks the identity tokens expo-apple-authentication's
// signInAsync returns, against Apple's published keys. Audiences are the
// app's bundle identifiers (see Settings.AppleBundleIDs); zero means Sign
// in with Apple isn't configured.
type AppleVerifier struct {
	audiences []string
	client    *http.Client
	// keysURL is appleKeysURL; only tests point it elsewhere.
	keysURL string

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewAppleVerifier(audiences []string) *AppleVerifier {
	return &AppleVerifier{
		audiences: audiences,
		client:    &http.Client{Timeout: 10 * time.Second},
		keysURL:   appleKeysURL,
		keys:      map[string]*rsa.PublicKey{},
	}
}

func (v *AppleVerifier) Configured() bool { return len(v.audiences) > 0 }

// Verify returns the identity the token vouches for, or an error wrapping
// ErrAppleTokenInvalid when it can't be trusted.
func (v *AppleVerifier) Verify(ctx context.Context, identityToken string) (*AppleIdentity, error) {
	if !v.Configured() {
		return nil, errors.New("sign in with apple is not configured")
	}

	var claims struct {
		jwt.RegisteredClaims
		Email string `json:"email"`
		// Apple sends this as a JSON bool or as the string "true".
		EmailVerified any `json:"email_verified"`
	}
	// The audience is every bundle id this app ships under, so it is
	// checked below rather than through jwt.WithAudience (one value only).
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(AppleIssuer),
		jwt.WithExpirationRequired(),
	)

	_, err := parser.ParseWithClaims(identityToken, &claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token has no key id")
		}
		return v.publicKey(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAppleTokenInvalid, err)
	}
	if !containsAudience(claims.Audience, v.audiences) {
		return nil, fmt.Errorf("%w: audience %v is not this app", ErrAppleTokenInvalid, claims.Audience)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: no subject", ErrAppleTokenInvalid)
	}
	verified := claims.EmailVerified == true || claims.EmailVerified == "true"
	return &AppleIdentity{Sub: claims.Subject, Email: claims.Email, EmailVerified: verified}, nil
}

func containsAudience(tokenAudiences jwt.ClaimStrings, accepted []string) bool {
	for _, a := range tokenAudiences {
		for _, want := range accepted {
			if a == want {
				return true
			}
		}
	}
	return false
}

// publicKey returns Apple's key for this key id, refetching the key set
// once when the id is unknown (a rotation) and at most every minute.
func (v *AppleVerifier) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	key, ok := v.keys[kid]
	stale := time.Since(v.fetchedAt) > time.Minute
	v.mu.Unlock()
	if ok {
		return key, nil
	}
	if !stale {
		return nil, fmt.Errorf("unknown apple key id %q", kid)
	}

	keys, err := v.fetchKeys(ctx)
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	key, ok = keys[kid]
	v.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown apple key id %q", kid)
	}
	return key, nil
}

func (v *AppleVerifier) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.keysURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch apple keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch apple keys: %s", resp.Status)
	}

	var body struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode apple keys: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(body.Keys))
	for _, k := range body.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return nil, errors.New("apple returned no usable keys")
	}
	return keys, nil
}
