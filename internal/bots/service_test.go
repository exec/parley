// internal/bots/service_test.go
package bots

import (
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	svc := &Service{keySecret: key}

	plaintext := "sk-test-api-key-12345"
	enc, err := svc.encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plaintext {
		t.Fatal("encrypt should not return plaintext unchanged")
	}

	dec, err := svc.decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != plaintext {
		t.Fatalf("want %q, got %q", plaintext, dec)
	}
}

func TestEncryptProducesUniqueValues(t *testing.T) {
	key := make([]byte, 32)
	svc := &Service{keySecret: key}

	a, err := svc.encrypt("same-input")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.encrypt("same-input")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("encrypt should produce unique ciphertext each call (random nonce)")
	}
}
