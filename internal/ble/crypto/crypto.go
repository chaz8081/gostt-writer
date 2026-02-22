// Package crypto provides cryptographic primitives for the ToothPaste BLE protocol:
// ECDH P-256 key exchange, compressed public key serialization, HKDF-SHA256 key
// derivation, and AES-256-GCM encryption with separate IV and tag fields.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math/big"

	"golang.org/x/crypto/hkdf"
)

// GenerateKeyPair creates a new ECDH P-256 key pair for BLE pairing.
func GenerateKeyPair() (*ecdh.PrivateKey, *ecdh.PublicKey, error) {
	curve := ecdh.P256()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("ble/crypto: generate key: %w", err)
	}
	return priv, priv.PublicKey(), nil
}

// CompressPublicKey returns the 33-byte SEC1 compressed form of a P-256 public key.
// The crypto/ecdh package's Bytes() returns the uncompressed form (65 bytes: 0x04 || x || y).
// We compress it to 33 bytes (0x02/0x03 || x) to match ToothPaste's format.
func CompressPublicKey(pub *ecdh.PublicKey) []byte {
	raw := pub.Bytes() // 65 bytes: 0x04 || x(32) || y(32)
	x := raw[1:33]
	y := new(big.Int).SetBytes(raw[33:65])

	compressed := make([]byte, 33)
	if y.Bit(0) == 0 {
		compressed[0] = 0x02
	} else {
		compressed[0] = 0x03
	}
	copy(compressed[1:], x)
	return compressed
}

// ParseCompressedPublicKey parses a 33-byte SEC1 compressed P-256 public key.
func ParseCompressedPublicKey(data []byte) (*ecdh.PublicKey, error) {
	if len(data) != 33 {
		return nil, fmt.Errorf("ble/crypto: compressed key must be 33 bytes, got %d", len(data))
	}
	if data[0] != 0x02 && data[0] != 0x03 {
		return nil, fmt.Errorf("ble/crypto: invalid compression prefix: 0x%02x", data[0])
	}

	// Decompress using elliptic package
	x := new(big.Int).SetBytes(data[1:33])
	y := decompressP256(x, data[0] == 0x03)
	if y == nil {
		return nil, errors.New("ble/crypto: point decompression failed")
	}

	// Build uncompressed form for ecdh
	uncompressed := make([]byte, 65)
	uncompressed[0] = 0x04
	xBytes := x.Bytes()
	copy(uncompressed[1+32-len(xBytes):33], xBytes)
	yBytes := y.Bytes()
	copy(uncompressed[33+32-len(yBytes):65], yBytes)

	pub, err := ecdh.P256().NewPublicKey(uncompressed)
	if err != nil {
		return nil, fmt.Errorf("ble/crypto: parse public key: %w", err)
	}
	return pub, nil
}

// decompressP256 recovers the y coordinate from x on the P-256 curve.
// oddY indicates whether y should be odd.
func decompressP256(x *big.Int, oddY bool) *big.Int {
	curve := elliptic.P256()
	params := curve.Params()
	p := params.P

	// y^2 = x^3 - 3x + b (mod p)
	x3 := new(big.Int).Mul(x, x)
	x3.Mul(x3, x)
	x3.Mod(x3, p)

	threeX := new(big.Int).Mul(big.NewInt(3), x)
	threeX.Mod(threeX, p)

	y2 := new(big.Int).Sub(x3, threeX)
	y2.Add(y2, params.B)
	y2.Mod(y2, p)

	// y = y2^((p+1)/4) mod p (works because p ≡ 3 mod 4 for P-256)
	exp := new(big.Int).Add(p, big.NewInt(1))
	exp.Rsh(exp, 2)
	y := new(big.Int).Exp(y2, exp, p)

	// Verify
	check := new(big.Int).Mul(y, y)
	check.Mod(check, p)
	if check.Cmp(y2) != 0 {
		return nil
	}

	// Adjust parity
	if oddY != (y.Bit(0) == 1) {
		y.Sub(p, y)
	}
	return y
}

// DeriveSharedSecret performs ECDH and returns the raw shared secret.
func DeriveSharedSecret(priv *ecdh.PrivateKey, peerPub *ecdh.PublicKey) ([]byte, error) {
	secret, err := priv.ECDH(peerPub)
	if err != nil {
		return nil, fmt.Errorf("ble/crypto: ECDH: %w", err)
	}
	return secret, nil
}

// DeriveEncryptionKey uses HKDF-SHA256 to derive a 32-byte AES key from the shared secret.
// Matches ToothPaste: HKDF(secret, salt=nil, info="toothpaste", length=32).
func DeriveEncryptionKey(sharedSecret []byte) ([]byte, error) {
	hkdfReader := hkdf.New(sha256.New, sharedSecret, nil, []byte("toothpaste"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("ble/crypto: HKDF: %w", err)
	}
	return key, nil
}

// Encrypt encrypts plaintext with AES-256-GCM, returning iv (12 bytes),
// ciphertext, and tag (16 bytes) separately (as ToothPaste expects them in
// separate protobuf fields).
func Encrypt(key, plaintext []byte) (iv, ciphertext, tag []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ble/crypto: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ble/crypto: new GCM: %w", err)
	}

	iv = make([]byte, aead.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, nil, nil, fmt.Errorf("ble/crypto: random IV: %w", err)
	}

	// Go's GCM Seal appends the tag to the ciphertext
	sealed := aead.Seal(nil, iv, plaintext, nil)

	// Split: ciphertext is sealed[:len-tagSize], tag is sealed[len-tagSize:]
	tagSize := aead.Overhead() // 16
	ciphertext = sealed[:len(sealed)-tagSize]
	tag = sealed[len(sealed)-tagSize:]

	return iv, ciphertext, tag, nil
}

// Decrypt decrypts ciphertext with AES-256-GCM using separate iv, ciphertext, and tag.
func Decrypt(key, iv, ciphertext, tag []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("ble/crypto: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("ble/crypto: new GCM: %w", err)
	}

	// Reassemble: ciphertext || tag (as Go's GCM expects).
	// Use explicit allocation to avoid mutating the caller's ciphertext slice.
	sealed := make([]byte, len(ciphertext)+len(tag))
	copy(sealed, ciphertext)
	copy(sealed[len(ciphertext):], tag)
	plaintext, err := aead.Open(nil, iv, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("ble/crypto: decrypt: %w", err)
	}
	return plaintext, nil
}
