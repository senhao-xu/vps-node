package secrets

import (
	"bytes"
	"errors"
	"testing"
)

func testKey() []byte {
	return bytes.Repeat([]byte{0x2a}, 32)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey()
	plaintext := []byte(`{"password":"hunter2"}`)

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext must not contain plaintext")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("round trip mismatch: %q != %q", decrypted, plaintext)
	}
}

func TestEncryptNoncesDiffer(t *testing.T) {
	key := testKey()
	a, err := Encrypt(key, []byte("same"))
	if err != nil {
		t.Fatalf("encrypt a: %v", err)
	}
	b, err := Encrypt(key, []byte("same"))
	if err != nil {
		t.Fatalf("encrypt b: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("expected different ciphertexts for identical plaintext")
	}
}

func TestDecryptRejectsTamperedData(t *testing.T) {
	key := testKey()
	ciphertext, err := Encrypt(key, []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	ciphertext[len(ciphertext)-1] ^= 0xff

	if _, err := Decrypt(key, ciphertext); !errors.Is(err, ErrCiphertext) {
		t.Fatalf("expected ErrCiphertext, got %v", err)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	ciphertext, err := Encrypt(testKey(), []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	otherKey := bytes.Repeat([]byte{0x11}, 32)
	if _, err := Decrypt(otherKey, ciphertext); !errors.Is(err, ErrCiphertext) {
		t.Fatalf("expected ErrCiphertext, got %v", err)
	}
}
