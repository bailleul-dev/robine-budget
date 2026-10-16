package security

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func writeKey(t *testing.T, block *pem.Block) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private-key.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRSASigner_SignsVerifiably(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)

	for name, block := range map[string]*pem.Block{
		"pkcs8": {Type: "PRIVATE KEY", Bytes: pkcs8},
		"pkcs1": {Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)},
	} {
		t.Run(name, func(t *testing.T) {
			signer, err := LoadRSASigner(writeKey(t, block), "main")
			if err != nil {
				t.Fatalf("LoadRSASigner: %v", err)
			}
			if signer.KeyID() != "main" {
				t.Errorf("KeyID = %q", signer.KeyID())
			}
			data := []byte(`{"id":"x"}`)
			sigB64, err := signer.Sign(data)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			sig, err := base64.StdEncoding.DecodeString(sigB64)
			if err != nil {
				t.Fatalf("signature is not base64: %v", err)
			}
			digest := sha256.Sum256(data)
			if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
				t.Fatalf("signature does not verify: %v", err)
			}
		})
	}
}

func TestLoadRSASigner_Errors(t *testing.T) {
	if _, err := LoadRSASigner(filepath.Join(t.TempDir(), "missing.pem"), "main"); err == nil {
		t.Error("missing file should fail")
	}
	if _, err := LoadRSASigner(writeKey(t, &pem.Block{Type: "PRIVATE KEY", Bytes: []byte("garbage")}), "main"); err == nil {
		t.Error("garbage key should fail")
	}
	notPEM := filepath.Join(t.TempDir(), "k.pem")
	_ = os.WriteFile(notPEM, []byte("hello"), 0o600)
	if _, err := LoadRSASigner(notPEM, "main"); err == nil {
		t.Error("non-PEM file should fail")
	}
}
