package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// SecretBox is AES-256-GCM encryption for small secrets at rest (bank
// credentials, session cookies) the app needs to read back in plaintext -
// unlike BcryptPasswordHasher above, which is one-way. Implements
// ports.Encryptor.
type SecretBox struct {
	key []byte // 32 bytes
}

// NewSecretBox builds a SecretBox from a 64-character hex string (32 bytes,
// AES-256). Fails fast on a malformed key rather than at first use.
func NewSecretBox(hexKey string) (SecretBox, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return SecretBox{}, fmt.Errorf("encryption key: invalid hex: %w", err)
	}
	if len(key) != 32 {
		return SecretBox{}, fmt.Errorf("encryption key: want 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	return SecretBox{key: key}, nil
}

func (b SecretBox) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (b SecretBox) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, data := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, data, nil)
}
