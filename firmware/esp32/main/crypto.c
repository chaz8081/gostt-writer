// firmware/esp32/main/crypto.c
#include "crypto.h"
#include "config.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "nvs.h"

#include "mbedtls/ecdh.h"
#include "mbedtls/ecp.h"
#include "mbedtls/ctr_drbg.h"
#include "mbedtls/entropy.h"
#include "mbedtls/hkdf.h"
#include "mbedtls/md.h"
#include "mbedtls/gcm.h"

#include <string.h>

static const char *TAG = "gostt-crypto";

// Load AES key from NVS
static int load_key_from_nvs(gostt_crypto_ctx_t *ctx)
{
    nvs_handle_t handle;
    esp_err_t err = nvs_open(GOSTT_NVS_NAMESPACE, NVS_READONLY, &handle);
    if (err != ESP_OK) return -1;

    size_t key_len = GOSTT_AES_KEY_LEN;
    err = nvs_get_blob(handle, GOSTT_NVS_KEY_AES, ctx->aes_key, &key_len);
    if (err == ESP_OK && key_len == GOSTT_AES_KEY_LEN) {
        ctx->has_key = true;
        // Also load peer pubkey if available
        size_t pub_len = GOSTT_COMPRESSED_PUBKEY_LEN;
        nvs_get_blob(handle, GOSTT_NVS_KEY_PEER_PUB, ctx->peer_pubkey, &pub_len);
    }

    nvs_close(handle);
    return (ctx->has_key) ? 0 : -1;
}

// Save AES key and peer pubkey to NVS
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
        nvs_set_blob(handle, GOSTT_NVS_KEY_PEER_PUB, peer_pubkey, GOSTT_COMPRESSED_PUBKEY_LEN);
    }

    nvs_commit(handle);
    nvs_close(handle);
    return 0;
}

int gostt_crypto_init(gostt_crypto_ctx_t *ctx)
{
    memset(ctx, 0, sizeof(*ctx));
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
    int ret = -1;
    mbedtls_ecdh_context ecdh;
    mbedtls_entropy_context entropy;
    mbedtls_ctr_drbg_context ctr_drbg;

    mbedtls_ecdh_init(&ecdh);
    mbedtls_entropy_init(&entropy);
    mbedtls_ctr_drbg_init(&ctr_drbg);

    // Seed RNG
    if (mbedtls_ctr_drbg_seed(&ctr_drbg, mbedtls_entropy_func, &entropy,
                               (const unsigned char *)"gostt-kbd", 9) != 0) {
        ESP_LOGE(TAG, "RNG seed failed");
        goto cleanup;
    }

    // Setup ECDH with secp256r1
    if (mbedtls_ecdh_setup(&ecdh, MBEDTLS_ECP_DP_SECP256R1) != 0) {
        ESP_LOGE(TAG, "ECDH setup failed");
        goto cleanup;
    }

    // Generate our keypair
    if (mbedtls_ecdh_gen_public(&ecdh.MBEDTLS_PRIVATE(grp),
                                 &ecdh.MBEDTLS_PRIVATE(d),
                                 &ecdh.MBEDTLS_PRIVATE(Q),
                                 mbedtls_ctr_drbg_random, &ctr_drbg) != 0) {
        ESP_LOGE(TAG, "ECDH gen public failed");
        goto cleanup;
    }

    // Export our compressed public key (33 bytes)
    {
        mbedtls_ecp_point *Q = &ecdh.MBEDTLS_PRIVATE(Q);
        size_t olen;
        if (mbedtls_ecp_point_write_binary(&ecdh.MBEDTLS_PRIVATE(grp), Q,
                                            MBEDTLS_ECP_PF_COMPRESSED,
                                            &olen, own_pubkey_out,
                                            GOSTT_COMPRESSED_PUBKEY_LEN) != 0 || olen != 33) {
            ESP_LOGE(TAG, "Export compressed pubkey failed");
            goto cleanup;
        }
    }

    // Import peer's compressed public key
    {
        mbedtls_ecp_point peer_Q;
        mbedtls_ecp_point_init(&peer_Q);
        if (mbedtls_ecp_point_read_binary(&ecdh.MBEDTLS_PRIVATE(grp), &peer_Q,
                                           peer_compressed_pubkey,
                                           GOSTT_COMPRESSED_PUBKEY_LEN) != 0) {
            ESP_LOGE(TAG, "Import peer pubkey failed");
            mbedtls_ecp_point_free(&peer_Q);
            goto cleanup;
        }
        mbedtls_ecp_copy(&ecdh.MBEDTLS_PRIVATE(Qp), &peer_Q);
        mbedtls_ecp_point_free(&peer_Q);
    }

    // Compute shared secret
    uint8_t shared_secret[32];
    {
        mbedtls_mpi z;
        mbedtls_mpi_init(&z);
        if (mbedtls_ecdh_compute_shared(&ecdh.MBEDTLS_PRIVATE(grp), &z,
                                         &ecdh.MBEDTLS_PRIVATE(Qp),
                                         &ecdh.MBEDTLS_PRIVATE(d),
                                         mbedtls_ctr_drbg_random, &ctr_drbg) != 0) {
            ESP_LOGE(TAG, "ECDH compute shared failed");
            mbedtls_mpi_free(&z);
            goto cleanup;
        }
        if (mbedtls_mpi_write_binary(&z, shared_secret, 32) != 0) {
            ESP_LOGE(TAG, "MPI write binary failed");
            mbedtls_mpi_free(&z);
            goto cleanup;
        }
        mbedtls_mpi_free(&z);
    }

    // HKDF-SHA256: salt=NULL, info="toothpaste", output=32 bytes
    {
        const mbedtls_md_info_t *md_info = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256);
        if (mbedtls_hkdf(md_info,
                          NULL, 0,                                    // salt
                          shared_secret, 32,                          // ikm
                          (const uint8_t *)GOSTT_HKDF_INFO, GOSTT_HKDF_INFO_LEN, // info
                          ctx->aes_key, GOSTT_AES_KEY_LEN) != 0) {   // output
            ESP_LOGE(TAG, "HKDF failed");
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

    // Clear shared secret from stack
    memset(shared_secret, 0, sizeof(shared_secret));

cleanup:
    mbedtls_ecdh_free(&ecdh);
    mbedtls_ctr_drbg_free(&ctr_drbg);
    mbedtls_entropy_free(&entropy);
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

    mbedtls_gcm_context gcm;
    mbedtls_gcm_init(&gcm);

    if (mbedtls_gcm_setkey(&gcm, MBEDTLS_CIPHER_ID_AES,
                            ctx->aes_key, GOSTT_AES_KEY_LEN * 8) != 0) {
        ESP_LOGE(TAG, "GCM setkey failed");
        mbedtls_gcm_free(&gcm);
        return -1;
    }

    int ret = mbedtls_gcm_auth_decrypt(&gcm,
                                        ciphertext_len,
                                        iv, GOSTT_IV_LEN,
                                        NULL, 0,           // no AAD
                                        tag, GOSTT_TAG_LEN,
                                        ciphertext,
                                        plaintext_out);
    mbedtls_gcm_free(&gcm);

    if (ret != 0) {
        ESP_LOGE(TAG, "AES-GCM decrypt failed: %d", ret);
        return -1;
    }

    return (int)ciphertext_len; // plaintext same length as ciphertext for GCM
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
