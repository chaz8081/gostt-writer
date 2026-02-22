package crypto

import (
	"bytes"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	if priv == nil || pub == nil {
		t.Fatal("GenerateKeyPair() returned nil key")
	}
	// Compressed P-256 public key is 33 bytes
	compressed := CompressPublicKey(pub)
	if len(compressed) != 33 {
		t.Errorf("compressed public key length = %d, want 33", len(compressed))
	}
}

func TestDeriveSharedSecret(t *testing.T) {
	// Generate two key pairs and derive shared secret from both sides
	priv1, pub1, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	priv2, pub2, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	secret1, err := DeriveSharedSecret(priv1, pub2)
	if err != nil {
		t.Fatalf("DeriveSharedSecret(priv1, pub2) error = %v", err)
	}
	secret2, err := DeriveSharedSecret(priv2, pub1)
	if err != nil {
		t.Fatalf("DeriveSharedSecret(priv2, pub1) error = %v", err)
	}

	if !bytes.Equal(secret1, secret2) {
		t.Error("shared secrets from both sides do not match")
	}
}

func TestDeriveEncryptionKey(t *testing.T) {
	sharedSecret := make([]byte, 32)
	sharedSecret[0] = 0x42

	key, err := DeriveEncryptionKey(sharedSecret)
	if err != nil {
		t.Fatalf("DeriveEncryptionKey() error = %v", err)
	}
	if len(key) != 32 {
		t.Errorf("encryption key length = %d, want 32", len(key))
	}

	// Same input should produce same output (deterministic)
	key2, err := DeriveEncryptionKey(sharedSecret)
	if err != nil {
		t.Fatalf("DeriveEncryptionKey() second call error = %v", err)
	}
	if !bytes.Equal(key, key2) {
		t.Error("DeriveEncryptionKey is not deterministic")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	key[0] = 0x01
	key[31] = 0xFF

	plaintext := []byte("hello from gostt-writer")

	iv, ciphertext, tag, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if len(iv) != 12 {
		t.Errorf("IV length = %d, want 12", len(iv))
	}
	if len(tag) != 16 {
		t.Errorf("tag length = %d, want 16", len(tag))
	}

	decrypted, err := Decrypt(key, iv, ciphertext, tag)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key := make([]byte, 32)
	plaintext := []byte("secret")

	iv, ciphertext, tag, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	wrongKey := make([]byte, 32)
	wrongKey[0] = 0xFF

	_, err = Decrypt(wrongKey, iv, ciphertext, tag)
	if err == nil {
		t.Error("Decrypt() with wrong key should fail")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	plaintext := []byte("secret")

	iv, ciphertext, tag, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	ciphertext[0] ^= 0xFF // tamper
	_, err = Decrypt(key, iv, ciphertext, tag)
	if err == nil {
		t.Error("Decrypt() with tampered ciphertext should fail")
	}
}

func TestParseCompressedPublicKey(t *testing.T) {
	_, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	compressed := CompressPublicKey(pub)
	parsed, err := ParseCompressedPublicKey(compressed)
	if err != nil {
		t.Fatalf("ParseCompressedPublicKey() error = %v", err)
	}

	if !bytes.Equal(pub.Bytes(), parsed.Bytes()) {
		t.Error("round-tripped public key does not match original")
	}
}
