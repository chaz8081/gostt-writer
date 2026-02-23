// firmware/esp32/main/crypto.c
//
// PSA Crypto API implementation for ESP-IDF v6.1 (mbedtls 4.x).
// Uses PSA for ECDH, HKDF-SHA256, AES-256-GCM.
// Uses ESP-IDF-ported mbedtls/ecp.h for compressed EC pubkey conversion
// (PSA only supports uncompressed format).

#include "crypto.h"
#include "config.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "nvs.h"

// IMPORTANT: mbedtls/ecp.h MUST come before psa/crypto.h.
// ESP-IDF's port wrapper defines MBEDTLS_DECLARE_PRIVATE_IDENTIFIERS before
// including the private header. If psa/crypto.h is included first, it
// transitively includes private/ecp.h WITHOUT that macro, setting the include
// guard but skipping all function declarations.
#include "mbedtls/ecp.h"
#include "mbedtls/bignum.h"
#include "psa/crypto.h"

#include <string.h>
#include <limits.h>

static const char *TAG = "gostt-crypto";

// Uncompressed EC P-256 pubkey: 04 || x(32) || y(32) = 65 bytes
#define UNCOMPRESSED_PUBKEY_LEN 65

// --- NVS helpers ---

static int load_key_from_nvs(gostt_crypto_ctx_t *ctx)
{
    nvs_handle_t handle;
    esp_err_t err = nvs_open(GOSTT_NVS_NAMESPACE, NVS_READONLY, &handle);
    if (err != ESP_OK) return -1;

    size_t key_len = GOSTT_AES_KEY_LEN;
    err = nvs_get_blob(handle, GOSTT_NVS_KEY_AES, ctx->aes_key, &key_len);
    if (err == ESP_OK && key_len == GOSTT_AES_KEY_LEN) {
        ctx->has_key = true;
        size_t pub_len = GOSTT_COMPRESSED_PUBKEY_LEN;
        nvs_get_blob(handle, GOSTT_NVS_KEY_PEER_PUB, ctx->peer_pubkey, &pub_len);
    }

    nvs_close(handle);
    return (ctx->has_key) ? 0 : -1;
}

static int save_key_to_nvs(const gostt_crypto_ctx_t *ctx, const uint8_t *peer_pubkey)
{
    nvs_handle_t handle;
    esp_err_t err = nvs_open(GOSTT_NVS_NAMESPACE, NVS_READWRITE, &handle);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "NVS open failed: %s", esp_err_to_name(err));
        return -1;
    }

    err = nvs_set_blob(handle, GOSTT_NVS_KEY_AES, ctx->aes_key, GOSTT_AES_KEY_LEN);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "NVS write AES key failed: %s", esp_err_to_name(err));
        nvs_close(handle);
        return -1;
    }

    if (peer_pubkey) {
        err = nvs_set_blob(handle, GOSTT_NVS_KEY_PEER_PUB, peer_pubkey, GOSTT_COMPRESSED_PUBKEY_LEN);
        if (err != ESP_OK) {
            ESP_LOGW(TAG, "NVS write peer pubkey failed: %s", esp_err_to_name(err));
        }
    }

    nvs_commit(handle);
    nvs_close(handle);
    return 0;
}

// --- Compressed EC pubkey helpers (using ESP-IDF-ported mbedtls/ecp.h) ---

// Decompress 33-byte compressed pubkey to 65-byte uncompressed (04 || x || y)
static int decompress_pubkey(const uint8_t *compressed, uint8_t *uncompressed_out)
{
    int ret = -1;
    mbedtls_ecp_group grp;
    mbedtls_ecp_point pt;

    mbedtls_ecp_group_init(&grp);
    mbedtls_ecp_point_init(&pt);

    if (mbedtls_ecp_group_load(&grp, MBEDTLS_ECP_DP_SECP256R1) != 0) goto cleanup;
    if (mbedtls_ecp_point_read_binary(&grp, &pt, compressed, GOSTT_COMPRESSED_PUBKEY_LEN) != 0) goto cleanup;
    if (mbedtls_ecp_check_pubkey(&grp, &pt) != 0) goto cleanup;

    size_t olen;
    if (mbedtls_ecp_point_write_binary(&grp, &pt, MBEDTLS_ECP_PF_UNCOMPRESSED,
                                        &olen, uncompressed_out, UNCOMPRESSED_PUBKEY_LEN) != 0) goto cleanup;
    if (olen != UNCOMPRESSED_PUBKEY_LEN) goto cleanup;

    ret = 0;
cleanup:
    mbedtls_ecp_point_free(&pt);
    mbedtls_ecp_group_free(&grp);
    return ret;
}

// Compress 65-byte uncompressed pubkey to 33-byte compressed (02/03 || x)
static int compress_pubkey(const uint8_t *uncompressed, uint8_t *compressed_out)
{
    int ret = -1;
    mbedtls_ecp_group grp;
    mbedtls_ecp_point pt;

    mbedtls_ecp_group_init(&grp);
    mbedtls_ecp_point_init(&pt);

    if (mbedtls_ecp_group_load(&grp, MBEDTLS_ECP_DP_SECP256R1) != 0) goto cleanup;
    if (mbedtls_ecp_point_read_binary(&grp, &pt, uncompressed, UNCOMPRESSED_PUBKEY_LEN) != 0) goto cleanup;

    size_t olen;
    if (mbedtls_ecp_point_write_binary(&grp, &pt, MBEDTLS_ECP_PF_COMPRESSED,
                                        &olen, compressed_out, GOSTT_COMPRESSED_PUBKEY_LEN) != 0) goto cleanup;
    if (olen != GOSTT_COMPRESSED_PUBKEY_LEN) goto cleanup;

    ret = 0;
cleanup:
    mbedtls_ecp_point_free(&pt);
    mbedtls_ecp_group_free(&grp);
    return ret;
}

// --- Public API ---

int gostt_crypto_init(gostt_crypto_ctx_t *ctx)
{
    memset(ctx, 0, sizeof(*ctx));

    psa_status_t status = psa_crypto_init();
    if (status != PSA_SUCCESS) {
        ESP_LOGE(TAG, "PSA crypto init failed: %d", (int)status);
        return -1;
    }

    if (load_key_from_nvs(ctx) == 0) {
        ESP_LOGI(TAG, "Loaded encryption key from NVS");
    } else {
        ESP_LOGI(TAG, "No stored key — pairing required");
    }
    return 0;
}

int gostt_crypto_pair(gostt_crypto_ctx_t *ctx,
                      const uint8_t *peer_compressed_pubkey,
                      uint8_t *own_pubkey_out)
{
    psa_status_t status;
    mbedtls_svc_key_id_t key_id = MBEDTLS_SVC_KEY_ID_INIT;
    uint8_t shared_secret[32];
    int ret = -1;

    // 1. Generate our ECC key pair (secp256r1)
    psa_key_attributes_t key_attr = PSA_KEY_ATTRIBUTES_INIT;
    psa_set_key_usage_flags(&key_attr, PSA_KEY_USAGE_DERIVE | PSA_KEY_USAGE_EXPORT);
    psa_set_key_algorithm(&key_attr, PSA_ALG_ECDH);
    psa_set_key_type(&key_attr, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_R1));
    psa_set_key_bits(&key_attr, 256);

    status = psa_generate_key(&key_attr, &key_id);
    if (status != PSA_SUCCESS) {
        ESP_LOGE(TAG, "Key generation failed: %d", (int)status);
        return -1;
    }

    // 2. Export our public key (uncompressed format: 04 || x || y)
    uint8_t our_uncompressed[UNCOMPRESSED_PUBKEY_LEN];
    size_t our_pub_len = 0;
    status = psa_export_public_key(key_id, our_uncompressed, sizeof(our_uncompressed), &our_pub_len);
    if (status != PSA_SUCCESS || our_pub_len != UNCOMPRESSED_PUBKEY_LEN) {
        ESP_LOGE(TAG, "Export public key failed: %d", (int)status);
        goto cleanup;
    }

    // 3. Compress our public key to 33 bytes for the peer
    if (compress_pubkey(our_uncompressed, own_pubkey_out) != 0) {
        ESP_LOGE(TAG, "Compress own pubkey failed");
        goto cleanup;
    }

    // 4. Decompress peer's compressed public key to uncompressed format
    uint8_t peer_uncompressed[UNCOMPRESSED_PUBKEY_LEN];
    if (decompress_pubkey(peer_compressed_pubkey, peer_uncompressed) != 0) {
        ESP_LOGE(TAG, "Decompress peer pubkey failed");
        goto cleanup;
    }

    // 5. ECDH: compute shared secret
    size_t shared_len = 0;
    status = psa_raw_key_agreement(PSA_ALG_ECDH,
                                    key_id,
                                    peer_uncompressed, UNCOMPRESSED_PUBKEY_LEN,
                                    shared_secret, sizeof(shared_secret),
                                    &shared_len);
    if (status != PSA_SUCCESS || shared_len != 32) {
        ESP_LOGE(TAG, "ECDH key agreement failed: %d", (int)status);
        goto cleanup;
    }

    // 6. HKDF-SHA256: salt=NULL, info="toothpaste", output=32 bytes
    {
        // Import shared secret as a PSA key (required for HKDF SECRET step)
        psa_key_attributes_t ikm_attr = PSA_KEY_ATTRIBUTES_INIT;
        psa_set_key_usage_flags(&ikm_attr, PSA_KEY_USAGE_DERIVE);
        psa_set_key_algorithm(&ikm_attr, PSA_ALG_HKDF(PSA_ALG_SHA_256));
        psa_set_key_type(&ikm_attr, PSA_KEY_TYPE_DERIVE);
        psa_set_key_bits(&ikm_attr, shared_len * 8);

        mbedtls_svc_key_id_t ikm_key = MBEDTLS_SVC_KEY_ID_INIT;
        status = psa_import_key(&ikm_attr, shared_secret, shared_len, &ikm_key);
        if (status != PSA_SUCCESS) {
            ESP_LOGE(TAG, "HKDF IKM import failed: %d", (int)status);
            goto cleanup;
        }

        psa_key_derivation_operation_t kdf = PSA_KEY_DERIVATION_OPERATION_INIT;
        status = psa_key_derivation_setup(&kdf, PSA_ALG_HKDF(PSA_ALG_SHA_256));
        if (status != PSA_SUCCESS) {
            ESP_LOGE(TAG, "HKDF setup failed: %d", (int)status);
            psa_key_derivation_abort(&kdf);
            psa_destroy_key(ikm_key);
            goto cleanup;
        }

        // Salt (empty/NULL = zero-length salt, which HKDF treats as hash-length zeros)
        status = psa_key_derivation_input_bytes(&kdf, PSA_KEY_DERIVATION_INPUT_SALT, NULL, 0);
        if (status != PSA_SUCCESS) {
            ESP_LOGE(TAG, "HKDF salt input failed: %d", (int)status);
            psa_key_derivation_abort(&kdf);
            psa_destroy_key(ikm_key);
            goto cleanup;
        }

        // IKM (shared secret as key handle)
        status = psa_key_derivation_input_key(&kdf, PSA_KEY_DERIVATION_INPUT_SECRET, ikm_key);
        if (status != PSA_SUCCESS) {
            ESP_LOGE(TAG, "HKDF IKM input failed: %d", (int)status);
            psa_key_derivation_abort(&kdf);
            psa_destroy_key(ikm_key);
            goto cleanup;
        }

        // Info
        status = psa_key_derivation_input_bytes(&kdf, PSA_KEY_DERIVATION_INPUT_INFO,
                                                 (const uint8_t *)GOSTT_HKDF_INFO,
                                                 GOSTT_HKDF_INFO_LEN);
        if (status != PSA_SUCCESS) {
            ESP_LOGE(TAG, "HKDF info input failed: %d", (int)status);
            psa_key_derivation_abort(&kdf);
            psa_destroy_key(ikm_key);
            goto cleanup;
        }

        // Output
        status = psa_key_derivation_output_bytes(&kdf, ctx->aes_key, GOSTT_AES_KEY_LEN);
        psa_key_derivation_abort(&kdf);
        psa_destroy_key(ikm_key);
        if (status != PSA_SUCCESS) {
            ESP_LOGE(TAG, "HKDF output failed: %d", (int)status);
            goto cleanup;
        }
    }

    ctx->has_key = true;
    memcpy(ctx->peer_pubkey, peer_compressed_pubkey, GOSTT_COMPRESSED_PUBKEY_LEN);

    // Persist to NVS
    if (save_key_to_nvs(ctx, peer_compressed_pubkey) != 0) {
        ESP_LOGW(TAG, "Key derived but NVS save failed");
        // Non-fatal — key works for this session
    }

    ESP_LOGI(TAG, "Pairing complete — AES key derived and stored");
    ret = 0;

cleanup:
    // Wipe shared secret
    memset(shared_secret, 0, sizeof(shared_secret));
    // Destroy the ephemeral key
    psa_destroy_key(key_id);
    return ret;
}

int gostt_crypto_decrypt(const gostt_crypto_ctx_t *ctx,
                         const uint8_t *iv,
                         const uint8_t *tag,
                         const uint8_t *ciphertext, size_t ciphertext_len,
                         uint8_t *plaintext_out)
{
    if (!ctx->has_key) {
        ESP_LOGE(TAG, "No encryption key — cannot decrypt");
        return -1;
    }

    if (ciphertext_len == 0 || ciphertext_len > INT_MAX) {
        return -1;
    }

    // Import AES key into PSA
    psa_key_attributes_t key_attr = PSA_KEY_ATTRIBUTES_INIT;
    psa_set_key_usage_flags(&key_attr, PSA_KEY_USAGE_DECRYPT);
    psa_set_key_algorithm(&key_attr, PSA_ALG_GCM);
    psa_set_key_type(&key_attr, PSA_KEY_TYPE_AES);
    psa_set_key_bits(&key_attr, GOSTT_AES_KEY_LEN * 8);

    mbedtls_svc_key_id_t key_id = MBEDTLS_SVC_KEY_ID_INIT;
    psa_status_t status = psa_import_key(&key_attr, ctx->aes_key, GOSTT_AES_KEY_LEN, &key_id);
    if (status != PSA_SUCCESS) {
        ESP_LOGE(TAG, "AES key import failed: %d", (int)status);
        return -1;
    }

    // PSA AEAD expects ciphertext || tag concatenated
    // Build combined buffer: ciphertext + tag
    size_t combined_len = ciphertext_len + GOSTT_TAG_LEN;
    uint8_t *combined = (uint8_t *)malloc(combined_len);
    if (!combined) {
        ESP_LOGE(TAG, "Alloc failed for AEAD input");
        psa_destroy_key(key_id);
        return -1;
    }
    memcpy(combined, ciphertext, ciphertext_len);
    memcpy(combined + ciphertext_len, tag, GOSTT_TAG_LEN);

    size_t plaintext_len = 0;
    status = psa_aead_decrypt(key_id, PSA_ALG_GCM,
                               iv, GOSTT_IV_LEN,
                               NULL, 0,  // no AAD
                               combined, combined_len,
                               plaintext_out, ciphertext_len,
                               &plaintext_len);
    free(combined);
    psa_destroy_key(key_id);

    if (status != PSA_SUCCESS) {
        ESP_LOGE(TAG, "AES-GCM decrypt failed: %d", (int)status);
        return -1;
    }

    return (int)plaintext_len;
}

int gostt_crypto_erase(gostt_crypto_ctx_t *ctx)
{
    nvs_handle_t handle;
    esp_err_t err = nvs_open(GOSTT_NVS_NAMESPACE, NVS_READWRITE, &handle);
    if (err == ESP_OK) {
        nvs_erase_key(handle, GOSTT_NVS_KEY_AES);
        nvs_erase_key(handle, GOSTT_NVS_KEY_PEER_PUB);
        nvs_erase_key(handle, GOSTT_NVS_KEY_MUTE_CFG);
        nvs_commit(handle);
        nvs_close(handle);
    }

    memset(ctx, 0, sizeof(*ctx));
    ESP_LOGI(TAG, "All keys erased");
    return 0;
}
