package security

import (
	"strings"
	"testing"
)

const testHexKey = "0000000000000000000000000000000000000000000000000000000000000000000000000000"

// A real 64-hex-char (32-byte) key for tests that need one — the all-zeros
// key above is deliberately too long, used only by the "invalid length" case.
const validHexKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func TestNewSecretBox(t *testing.T) {
	if _, err := NewSecretBox(validHexKey); err != nil {
		t.Fatalf("NewSecretBox with a valid 32-byte key: %v", err)
	}
}

func TestNewSecretBox_InvalidHex(t *testing.T) {
	if _, err := NewSecretBox("not-hex-at-all"); err == nil {
		t.Fatal("expected an error for a non-hex key")
	}
}

func TestNewSecretBox_WrongLength(t *testing.T) {
	if _, err := NewSecretBox("aabbcc"); err == nil {
		t.Fatal("expected an error for a key shorter than 32 bytes")
	}
	if _, err := NewSecretBox(testHexKey); err == nil {
		t.Fatal("expected an error for a key longer than 32 bytes")
	}
}

func TestSecretBox_RoundTrip(t *testing.T) {
	box, err := NewSecretBox(validHexKey)
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}

	plaintext := []byte("super secret bank credentials")
	ciphertext, err := box.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if string(ciphertext) == string(plaintext) {
		t.Fatal("ciphertext should not equal the plaintext")
	}

	decrypted, err := box.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

// AES-GCM must use a fresh random nonce per call - encrypting the exact
// same plaintext twice should never produce identical ciphertexts (that
// would leak that two secrets are the same, and with a reused nonce would
// break GCM's authentication guarantee entirely).
func TestSecretBox_EncryptIsNotDeterministic(t *testing.T) {
	box, err := NewSecretBox(validHexKey)
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	plaintext := []byte("same secret both times")

	a, err := box.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	b, err := box.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if string(a) == string(b) {
		t.Fatal("two encryptions of the same plaintext should produce different ciphertexts")
	}
}

func TestSecretBox_DecryptWrongKeyFails(t *testing.T) {
	boxA, err := NewSecretBox(validHexKey)
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	otherKey := strings.Repeat("ab", 32)
	boxB, err := NewSecretBox(otherKey)
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}

	ciphertext, err := boxA.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := boxB.Decrypt(ciphertext); err == nil {
		t.Fatal("expected an error decrypting with the wrong key")
	}
}

func TestSecretBox_DecryptTamperedCiphertextFails(t *testing.T) {
	box, err := NewSecretBox(validHexKey)
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	ciphertext, err := box.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	tampered := append([]byte(nil), ciphertext...)
	tampered[len(tampered)-1] ^= 0xFF // flip the last byte

	if _, err := box.Decrypt(tampered); err == nil {
		t.Fatal("expected an error decrypting tampered ciphertext (GCM auth tag should fail)")
	}
}

func TestSecretBox_DecryptTooShortFails(t *testing.T) {
	box, err := NewSecretBox(validHexKey)
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	if _, err := box.Decrypt([]byte("short")); err == nil {
		t.Fatal("expected an error decrypting ciphertext shorter than the nonce")
	}
}
