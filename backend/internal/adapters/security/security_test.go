package security

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestBcryptPasswordHasher_RoundTrip(t *testing.T) {
	h := BcryptPasswordHasher{}
	hash, err := h.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !h.Verify("correct horse battery staple", hash) {
		t.Fatal("Verify should accept the correct password")
	}
	if h.Verify("wrong password", hash) {
		t.Fatal("Verify should reject a wrong password")
	}
}

func TestBcryptPasswordHasher_SaltedDifferently(t *testing.T) {
	h := BcryptPasswordHasher{}
	hash1, err := h.Hash("same password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	hash2, err := h.Hash("same password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash1 == hash2 {
		t.Fatal("two hashes of the same password should differ (bcrypt salts per call)")
	}
	if !h.Verify("same password", hash1) || !h.Verify("same password", hash2) {
		t.Fatal("both hashes should still verify the original password")
	}
}

// bcrypt only ever processes the first 72 bytes of its input - past that it
// returns an error rather than silently truncating, so a very long
// passphrase must surface as a Hash error instead of hashing a value the
// caller didn't expect.
func TestBcryptPasswordHasher_Hash_RejectsPasswordOver72Bytes(t *testing.T) {
	h := BcryptPasswordHasher{}
	tooLong := strings.Repeat("a", 73)
	if _, err := h.Hash(tooLong); err == nil {
		t.Fatal("expected an error hashing a password longer than 72 bytes")
	}
}

func TestBcryptPasswordHasher_VerifyRejectsGarbageHash(t *testing.T) {
	h := BcryptPasswordHasher{}
	if h.Verify("anything", "not-a-real-bcrypt-hash") {
		t.Fatal("Verify should reject a malformed hash, not panic or accept it")
	}
}

func TestSessionTokens_RoundTrip(t *testing.T) {
	tokens := SessionTokens{Secret: "test-secret", ExpireMinutes: 60}
	userID := uuid.New()

	tokenString, err := tokens.Create(userID, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _, err := tokens.Decode(tokenString)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != userID {
		t.Fatalf("got %s, want %s", got, userID)
	}
}

func TestSessionTokens_Decode_Expired(t *testing.T) {
	tokens := SessionTokens{Secret: "test-secret", ExpireMinutes: -1} // already expired
	tokenString, err := tokens.Create(uuid.New(), 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, _, err := tokens.Decode(tokenString); err == nil {
		t.Fatal("expected an error decoding an expired token")
	}
}

func TestSessionTokens_Decode_WrongSecret(t *testing.T) {
	created := SessionTokens{Secret: "secret-a", ExpireMinutes: 60}
	tokenString, err := created.Create(uuid.New(), 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	decoded := SessionTokens{Secret: "secret-b", ExpireMinutes: 60}
	if _, _, err := decoded.Decode(tokenString); err == nil {
		t.Fatal("expected an error decoding a token signed with a different secret")
	}
}

func TestSessionTokens_Decode_MalformedToken(t *testing.T) {
	tokens := SessionTokens{Secret: "test-secret", ExpireMinutes: 60}
	if _, _, err := tokens.Decode("not.a.jwt"); err == nil {
		t.Fatal("expected an error decoding a malformed token")
	}
	if _, _, err := tokens.Decode(""); err == nil {
		t.Fatal("expected an error decoding an empty string")
	}
}

// The classic JWT "alg" downgrade attack: a token self-signed with
// SigningMethodNone (no server secret involved at all) must be rejected,
// not accepted because its claims parse fine.
func TestSessionTokens_Decode_RejectsNoneAlgorithm(t *testing.T) {
	tokens := SessionTokens{Secret: "test-secret", ExpireMinutes: 60}
	claims := jwt.MapClaims{
		"sub": uuid.New().String(),
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("building an alg=none token: %v", err)
	}
	if _, _, err := tokens.Decode(tokenString); err == nil {
		t.Fatal("expected an error decoding an alg=none token")
	}
}

func TestSessionTokens_Decode_RejectsNonUUIDSubject(t *testing.T) {
	tokens := SessionTokens{Secret: "test-secret", ExpireMinutes: 60}
	claims := jwt.MapClaims{
		"sub": "not-a-uuid",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tokens.Secret))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if _, _, err := tokens.Decode(tokenString); err == nil {
		t.Fatal("expected an error decoding a token whose subject isn't a UUID")
	}
}
