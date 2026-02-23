// firmware/esp32/main/crypto.h
#ifndef GOSTT_KBD_CRYPTO_H
#define GOSTT_KBD_CRYPTO_H

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>
#include "config.h"

// Crypto context — holds ECDH keypair during pairing and AES key for normal operation
typedef struct {
    uint8_t aes_key[GOSTT_AES_KEY_LEN];
    bool    has_key;
    uint8_t peer_pubkey[GOSTT_COMPRESSED_PUBKEY_LEN]; // stored for re-pairing detection
} gostt_crypto_ctx_t;

// Initialize crypto context. Attempts to load AES key from NVS.
// Returns 0 on success (key may or may not be loaded — check ctx->has_key).
int gostt_crypto_init(gostt_crypto_ctx_t *ctx);

// Perform ECDH key exchange given the peer's 33-byte compressed public key.
// Generates our own keypair, derives shared secret, derives AES key via HKDF.
// Stores AES key in ctx and NVS.
// Writes our 33-byte compressed public key to own_pubkey_out.
// Returns 0 on success, -1 on error.
int gostt_crypto_pair(gostt_crypto_ctx_t *ctx,
                      const uint8_t *peer_compressed_pubkey,
                      uint8_t *own_pubkey_out);

// Decrypt ciphertext with AES-256-GCM.
// iv: 12 bytes, tag: 16 bytes, ciphertext: variable length.
// plaintext_out must be at least ciphertext_len bytes.
// Returns plaintext length on success, -1 on error.
int gostt_crypto_decrypt(const gostt_crypto_ctx_t *ctx,
                         const uint8_t *iv,
                         const uint8_t *tag,
                         const uint8_t *ciphertext, size_t ciphertext_len,
                         uint8_t *plaintext_out);

// Erase all stored keys from NVS and reset context.
int gostt_crypto_erase(gostt_crypto_ctx_t *ctx);

#endif // GOSTT_KBD_CRYPTO_H
