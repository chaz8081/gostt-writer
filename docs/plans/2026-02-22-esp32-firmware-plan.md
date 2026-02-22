# ESP32-S3 Firmware (GOSTT-KBD) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build custom ESP-IDF firmware that receives encrypted BLE text from gostt-writer and types it as USB HID keystrokes.

**Architecture:** ESP-IDF project with three subsystems: BLE GATT server (NimBLE), crypto engine (mbedtls ECDH + AES-GCM), and USB HID output (TinyUSB composite keyboard + consumer control). Single FreeRTOS task, event-driven state machine.

**Tech Stack:** ESP-IDF v5.x, NimBLE, TinyUSB, mbedtls, WS2812 LED via RMT driver. C17.

**Design doc:** `docs/plans/2026-02-22-esp32-firmware-design.md`

**Reference files (Go app — defines the wire protocol the firmware must implement):**
- `internal/ble/protocol/proto.go` — Protobuf marshal/unmarshal (golden bytes in tests)
- `internal/ble/protocol/proto_test.go` — Golden byte sequences for cross-validation
- `internal/ble/crypto/crypto.go` — ECDH, HKDF, AES-GCM (app-side encrypt)
- `internal/ble/crypto/crypto_test.go` — Crypto test vectors
- `internal/ble/adapter.go` — UUID constants (ServiceUUID, TXCharUUID, etc.)
- `internal/ble/pair.go` — Pairing flow (app side — firmware implements device side)

**Important notes for implementer:**
- This is ESP-IDF C code, NOT Go. The Go files above are REFERENCE ONLY.
- All C files go in `firmware/esp32/main/`.
- ESP-IDF uses CMake, not Make.
- NimBLE is ESP-IDF's BLE stack; TinyUSB is ESP-IDF's USB stack.
- mbedtls ships with ESP-IDF and is configured via `menuconfig`/`sdkconfig.defaults`.
- Test with `idf.py build` — no hardware needed for compilation checks.
- For unit tests, use ESP-IDF's host test framework or a simple `test_main.c` compiled with the host GCC.

---

### Task 1: Project Scaffold

**Files:**
- Create: `firmware/esp32/CMakeLists.txt`
- Create: `firmware/esp32/main/CMakeLists.txt`
- Create: `firmware/esp32/main/main.c`
- Create: `firmware/esp32/sdkconfig.defaults`
- Create: `firmware/esp32/partitions.csv`

**Step 1: Create top-level CMakeLists.txt**

```cmake
# firmware/esp32/CMakeLists.txt
cmake_minimum_required(VERSION 3.16)

include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(gostt-kbd)
```

**Step 2: Create main component CMakeLists.txt**

```cmake
# firmware/esp32/main/CMakeLists.txt
idf_component_register(
    SRCS "main.c"
    INCLUDE_DIRS "."
    REQUIRES bt nvs_flash tinyusb driver led_strip
)
```

Note: `SRCS` will grow as we add more `.c` files in later tasks.

**Step 3: Create minimal main.c**

```c
// firmware/esp32/main/main.c
#include <stdio.h>
#include "esp_log.h"
#include "nvs_flash.h"

static const char *TAG = "gostt-kbd";

void app_main(void)
{
    ESP_LOGI(TAG, "GOSTT-KBD firmware starting...");

    // Initialize NVS
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        ret = nvs_flash_init();
    }
    ESP_ERROR_CHECK(ret);

    ESP_LOGI(TAG, "NVS initialized");
    ESP_LOGI(TAG, "GOSTT-KBD ready (scaffold only)");
}
```

**Step 4: Create sdkconfig.defaults**

```ini
# firmware/esp32/sdkconfig.defaults

# Target
CONFIG_IDF_TARGET="esp32s3"

# BLE (NimBLE)
CONFIG_BT_ENABLED=y
CONFIG_BT_NIMBLE_ENABLED=y
CONFIG_BT_NIMBLE_MAX_CONNECTIONS=1
CONFIG_BT_NIMBLE_ROLE_CENTRAL=n
CONFIG_BT_NIMBLE_ROLE_OBSERVER=n
CONFIG_BT_NIMBLE_ROLE_BROADCASTER=y
CONFIG_BT_NIMBLE_ROLE_PERIPHERAL=y

# USB (TinyUSB)
CONFIG_TINYUSB_ENABLED=y
CONFIG_TINYUSB_HID_ENABLED=y

# mbedtls (ECDH + AES-GCM)
CONFIG_MBEDTLS_ECDH_C=y
CONFIG_MBEDTLS_ECP_DP_SECP256R1_ENABLED=y
CONFIG_MBEDTLS_GCM_C=y
CONFIG_MBEDTLS_HKDF_C=y

# NVS
CONFIG_NVS_ENCRYPTION=n

# Partition table
CONFIG_PARTITION_TABLE_CUSTOM=y
CONFIG_PARTITION_TABLE_CUSTOM_FILENAME="partitions.csv"

# Logging
CONFIG_LOG_DEFAULT_LEVEL_INFO=y
```

**Step 5: Create partitions.csv**

```csv
# Name,   Type, SubType, Offset,  Size, Flags
nvs,      data, nvs,     0x9000,  0x6000,
phy_init, data, phy,     0xf000,  0x1000,
factory,  app,  factory, 0x10000, 1M,
```

**Step 6: Create config.h with constants**

Create `firmware/esp32/main/config.h`:

```c
// firmware/esp32/main/config.h
#ifndef GOSTT_KBD_CONFIG_H
#define GOSTT_KBD_CONFIG_H

// BLE
#define GOSTT_BLE_DEVICE_NAME       "GOSTT-KBD"
#define GOSTT_BLE_SERVICE_UUID      "19b10000-e8f2-537e-4f6c-d104768a1214"
#define GOSTT_BLE_TX_CHAR_UUID      "6856e119-2c7b-455a-bf42-cf7ddd2c5907"
#define GOSTT_BLE_RESP_CHAR_UUID    "6856e119-2c7b-455a-bf42-cf7ddd2c5908"
#define GOSTT_BLE_MAC_CHAR_UUID     "19b10002-e8f2-537e-4f6c-d104768a1214"

// Keepalive interval (ms)
#define GOSTT_KEEPALIVE_INTERVAL_MS 5000

// USB HID typing cadence (ms)
#define GOSTT_KEY_PRESS_MS          5
#define GOSTT_KEY_GAP_MS            2

// LED GPIO (WS2812 on most ESP32-S3 dev boards)
#define GOSTT_LED_GPIO              48

// NVS namespace
#define GOSTT_NVS_NAMESPACE         "gostt_kbd"
#define GOSTT_NVS_KEY_AES           "aes_key"
#define GOSTT_NVS_KEY_PEER_PUB      "peer_pub"
#define GOSTT_NVS_KEY_MUTE_CFG      "mute_cfg"

// Factory reset: hold BOOT button for this many ms
#define GOSTT_FACTORY_RESET_MS      5000

// HKDF info string (must match Go app: "toothpaste")
#define GOSTT_HKDF_INFO             "toothpaste"
#define GOSTT_HKDF_INFO_LEN         10

// Crypto sizes
#define GOSTT_AES_KEY_LEN           32
#define GOSTT_IV_LEN                12
#define GOSTT_TAG_LEN               16
#define GOSTT_COMPRESSED_PUBKEY_LEN 33

// Mute defaults
#define GOSTT_DEFAULT_MUTE_USAGE_ID 0x00E2  // USB HID Consumer Control: Mute

#endif // GOSTT_KBD_CONFIG_H
```

**Step 7: Verify build compiles**

This requires ESP-IDF installed. If not available, verify the file structure is correct and all files parse as valid C/CMake.

```bash
cd firmware/esp32 && idf.py set-target esp32s3 && idf.py build
```

**Step 8: Commit**

```bash
git add firmware/esp32/
git commit -m "feat(firmware): add ESP32-S3 project scaffold with CMake, NVS, and config"
```

---

### Task 2: Protobuf Decoder/Encoder

**Files:**
- Create: `firmware/esp32/main/proto.h`
- Create: `firmware/esp32/main/proto.c`
- Modify: `firmware/esp32/main/CMakeLists.txt` — add `proto.c` to SRCS

**Context:** The Go app sends `DataPacket` protobuf messages. The firmware must decode them. The firmware must also encode `ResponsePacket` messages. Wire format is documented in the design doc and matches `internal/ble/protocol/proto.go`.

**Golden bytes for cross-validation (from Go tests):**

KeyboardPacket("hello"):
```
0x0a 0x05 'h' 'e' 'l' 'l' 'o' 0x10 0x05
```

ResponsePacket(type=PeerStatus, status=Unknown, data=[0xDE,0xAD]):
```
0x08 0x01 0x10 0x00 0x1a 0x02 0xDE 0xAD
```

**Step 1: Create proto.h**

```c
// firmware/esp32/main/proto.h
#ifndef GOSTT_KBD_PROTO_H
#define GOSTT_KBD_PROTO_H

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>

// DataPacket (received from app, outer envelope)
typedef struct {
    uint8_t  iv[12];
    uint8_t  tag[16];
    uint8_t *encrypted_data;
    size_t   encrypted_data_len;
    uint32_t packet_num;
} gostt_data_packet_t;

// KeyboardPacket (inner, after decryption)
typedef struct {
    char    *message;
    size_t   message_len;
    uint32_t length;       // redundant length field from protobuf
} gostt_keyboard_packet_t;

// EncryptedData (inner wrapper)
// command_type: 0=text (keyboard_packet present), 1=mute_toggle, 2=configure_mute
typedef struct {
    uint8_t *keyboard_packet_data;
    size_t   keyboard_packet_data_len;
    uint32_t command_type;
    uint8_t *command_data;
    size_t   command_data_len;
} gostt_encrypted_data_t;

// ResponsePacket types
typedef enum {
    GOSTT_RESP_KEEPALIVE   = 0,
    GOSTT_RESP_PEER_STATUS = 1,
} gostt_response_type_t;

typedef enum {
    GOSTT_PEER_UNKNOWN = 0,
    GOSTT_PEER_KNOWN   = 1,
} gostt_peer_status_t;

// Decode a DataPacket from raw protobuf bytes.
// encrypted_data pointer is into the input buffer (not copied) — caller must not free input while using result.
// Returns 0 on success, -1 on error.
int gostt_decode_data_packet(const uint8_t *buf, size_t len, gostt_data_packet_t *out);

// Decode a KeyboardPacket from raw protobuf bytes.
// message pointer is into the input buffer — caller must not free input while using result.
// Returns 0 on success, -1 on error.
int gostt_decode_keyboard_packet(const uint8_t *buf, size_t len, gostt_keyboard_packet_t *out);

// Decode an EncryptedData wrapper from raw protobuf bytes.
// Pointers are into the input buffer.
// Returns 0 on success, -1 on error.
int gostt_decode_encrypted_data(const uint8_t *buf, size_t len, gostt_encrypted_data_t *out);

// Encode a ResponsePacket into buf. Returns number of bytes written, or -1 on error.
// buf must be at least 64 + data_len bytes.
int gostt_encode_response_packet(uint8_t *buf, size_t buf_len,
                                  gostt_response_type_t type,
                                  gostt_peer_status_t peer_status,
                                  const uint8_t *data, size_t data_len);

#endif // GOSTT_KBD_PROTO_H
```

**Step 2: Create proto.c**

```c
// firmware/esp32/main/proto.c
#include "proto.h"
#include <string.h>

// Read a protobuf varint from buf. Returns bytes consumed, or 0 on error.
static int read_varint(const uint8_t *buf, size_t len, uint64_t *value)
{
    *value = 0;
    int shift = 0;
    for (size_t i = 0; i < len && i < 10; i++) {
        *value |= (uint64_t)(buf[i] & 0x7F) << shift;
        shift += 7;
        if ((buf[i] & 0x80) == 0) {
            return (int)(i + 1);
        }
    }
    return 0; // error: truncated or too long
}

// Write a protobuf varint to buf. Returns bytes written.
static int write_varint(uint8_t *buf, size_t buf_len, uint64_t value)
{
    int n = 0;
    do {
        if ((size_t)n >= buf_len) return -1;
        buf[n] = (uint8_t)(value & 0x7F);
        value >>= 7;
        if (value > 0) buf[n] |= 0x80;
        n++;
    } while (value > 0);
    return n;
}

int gostt_decode_data_packet(const uint8_t *buf, size_t len, gostt_data_packet_t *out)
{
    memset(out, 0, sizeof(*out));
    size_t pos = 0;

    while (pos < len) {
        uint64_t tag_val;
        int n = read_varint(buf + pos, len - pos, &tag_val);
        if (n == 0) return -1;
        pos += n;

        uint8_t field_num = (uint8_t)(tag_val >> 3);
        uint8_t wire_type = (uint8_t)(tag_val & 0x07);

        if (wire_type == 0) { // varint
            uint64_t val;
            n = read_varint(buf + pos, len - pos, &val);
            if (n == 0) return -1;
            pos += n;
            if (field_num == 4) out->packet_num = (uint32_t)val;
        } else if (wire_type == 2) { // length-delimited
            uint64_t field_len;
            n = read_varint(buf + pos, len - pos, &field_len);
            if (n == 0) return -1;
            pos += n;
            if (pos + field_len > len) return -1;
            switch (field_num) {
                case 1:
                    if (field_len != 12) return -1;
                    memcpy(out->iv, buf + pos, 12);
                    break;
                case 2:
                    if (field_len != 16) return -1;
                    memcpy(out->tag, buf + pos, 16);
                    break;
                case 3:
                    out->encrypted_data = (uint8_t *)(buf + pos);
                    out->encrypted_data_len = (size_t)field_len;
                    break;
            }
            pos += (size_t)field_len;
        } else {
            return -1; // unsupported wire type
        }
    }
    return 0;
}

int gostt_decode_keyboard_packet(const uint8_t *buf, size_t len, gostt_keyboard_packet_t *out)
{
    memset(out, 0, sizeof(*out));
    size_t pos = 0;

    while (pos < len) {
        uint64_t tag_val;
        int n = read_varint(buf + pos, len - pos, &tag_val);
        if (n == 0) return -1;
        pos += n;

        uint8_t field_num = (uint8_t)(tag_val >> 3);
        uint8_t wire_type = (uint8_t)(tag_val & 0x07);

        if (wire_type == 0) { // varint
            uint64_t val;
            n = read_varint(buf + pos, len - pos, &val);
            if (n == 0) return -1;
            pos += n;
            if (field_num == 2) out->length = (uint32_t)val;
        } else if (wire_type == 2) { // length-delimited
            uint64_t field_len;
            n = read_varint(buf + pos, len - pos, &field_len);
            if (n == 0) return -1;
            pos += n;
            if (pos + field_len > len) return -1;
            if (field_num == 1) {
                out->message = (char *)(buf + pos);
                out->message_len = (size_t)field_len;
            }
            pos += (size_t)field_len;
        } else {
            return -1;
        }
    }
    return 0;
}

int gostt_decode_encrypted_data(const uint8_t *buf, size_t len, gostt_encrypted_data_t *out)
{
    memset(out, 0, sizeof(*out));
    size_t pos = 0;

    while (pos < len) {
        uint64_t tag_val;
        int n = read_varint(buf + pos, len - pos, &tag_val);
        if (n == 0) return -1;
        pos += n;

        uint8_t field_num = (uint8_t)(tag_val >> 3);
        uint8_t wire_type = (uint8_t)(tag_val & 0x07);

        if (wire_type == 0) {
            uint64_t val;
            n = read_varint(buf + pos, len - pos, &val);
            if (n == 0) return -1;
            pos += n;
            if (field_num == 2) out->command_type = (uint32_t)val;
        } else if (wire_type == 2) {
            uint64_t field_len;
            n = read_varint(buf + pos, len - pos, &field_len);
            if (n == 0) return -1;
            pos += n;
            if (pos + field_len > len) return -1;
            switch (field_num) {
                case 1:
                    out->keyboard_packet_data = (uint8_t *)(buf + pos);
                    out->keyboard_packet_data_len = (size_t)field_len;
                    break;
                case 3:
                    out->command_data = (uint8_t *)(buf + pos);
                    out->command_data_len = (size_t)field_len;
                    break;
            }
            pos += (size_t)field_len;
        } else {
            return -1;
        }
    }
    return 0;
}

int gostt_encode_response_packet(uint8_t *buf, size_t buf_len,
                                  gostt_response_type_t type,
                                  gostt_peer_status_t peer_status,
                                  const uint8_t *data, size_t data_len)
{
    size_t pos = 0;

    // Field 1: type (varint), tag = (1 << 3) | 0 = 0x08
    if (pos >= buf_len) return -1;
    buf[pos++] = 0x08;
    int n = write_varint(buf + pos, buf_len - pos, (uint64_t)type);
    if (n < 0) return -1;
    pos += n;

    // Field 2: peer_status (varint), tag = (2 << 3) | 0 = 0x10
    if (pos >= buf_len) return -1;
    buf[pos++] = 0x10;
    n = write_varint(buf + pos, buf_len - pos, (uint64_t)peer_status);
    if (n < 0) return -1;
    pos += n;

    // Field 3: data (bytes), tag = (3 << 3) | 2 = 0x1a
    if (data != NULL && data_len > 0) {
        if (pos >= buf_len) return -1;
        buf[pos++] = 0x1a;
        n = write_varint(buf + pos, buf_len - pos, (uint64_t)data_len);
        if (n < 0) return -1;
        pos += n;
        if (pos + data_len > buf_len) return -1;
        memcpy(buf + pos, data, data_len);
        pos += data_len;
    }

    return (int)pos;
}
```

**Step 3: Update CMakeLists.txt to include proto.c**

In `firmware/esp32/main/CMakeLists.txt`, change SRCS to:
```cmake
SRCS "main.c" "proto.c"
```

**Step 4: Verify build compiles**

```bash
cd firmware/esp32 && idf.py build
```

**Step 5: Commit**

```bash
git add firmware/esp32/main/proto.h firmware/esp32/main/proto.c firmware/esp32/main/CMakeLists.txt
git commit -m "feat(firmware): add protobuf decoder/encoder for BLE wire protocol"
```

---

### Task 3: Crypto Engine (ECDH + HKDF + AES-GCM)

**Files:**
- Create: `firmware/esp32/main/crypto.h`
- Create: `firmware/esp32/main/crypto.c`
- Modify: `firmware/esp32/main/CMakeLists.txt` — add `crypto.c` to SRCS

**Context:** Uses mbedtls (bundled with ESP-IDF). Must produce identical derived keys as the Go app's `internal/ble/crypto/crypto.go`. Key details:
- ECDH on secp256r1 (P-256)
- Compressed public keys: 33 bytes (0x02/0x03 prefix)
- HKDF-SHA256: salt=NULL, info="toothpaste", output=32 bytes
- AES-256-GCM: 12-byte IV, 16-byte tag, empty AAD
- Key and peer pubkey stored in NVS

**Step 1: Create crypto.h**

```c
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
```

**Step 2: Create crypto.c**

```c
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
```

**Step 3: Update CMakeLists.txt**

```cmake
SRCS "main.c" "proto.c" "crypto.c"
```

**Step 4: Verify build compiles**

```bash
cd firmware/esp32 && idf.py build
```

**Step 5: Commit**

```bash
git add firmware/esp32/main/crypto.h firmware/esp32/main/crypto.c firmware/esp32/main/CMakeLists.txt
git commit -m "feat(firmware): add crypto engine — ECDH pairing, HKDF, AES-GCM decrypt"
```

---

### Task 4: USB HID Output (Keyboard + Consumer Control)

**Files:**
- Create: `firmware/esp32/main/usb_hid.h`
- Create: `firmware/esp32/main/usb_hid.c`
- Modify: `firmware/esp32/main/CMakeLists.txt` — add `usb_hid.c` to SRCS

**Context:** TinyUSB composite device: HID Keyboard (interface 0) + HID Consumer Control (interface 1). The keyboard types ASCII text; consumer control sends mute.

**Step 1: Create usb_hid.h**

```c
// firmware/esp32/main/usb_hid.h
#ifndef GOSTT_KBD_USB_HID_H
#define GOSTT_KBD_USB_HID_H

#include <stdint.h>
#include <stddef.h>

// Initialize USB HID composite device (keyboard + consumer control).
// Must be called once during startup.
int gostt_usb_hid_init(void);

// Type a string as USB HID keystrokes.
// Only ASCII printable characters (0x20-0x7E), \n, and \t are supported.
// Blocks until all characters are typed.
// Returns 0 on success, -1 on error.
int gostt_usb_hid_type_text(const char *text, size_t len);

// Send a USB HID Consumer Control usage code (e.g., 0x00E2 for Mute).
// Sends press and release.
// Returns 0 on success, -1 on error.
int gostt_usb_hid_consumer_control(uint16_t usage_id);

// Send a keyboard shortcut (modifier mask + keycode).
// modifier: USB HID modifier bits (e.g., 0x01=Left Ctrl, 0x02=Left Shift, etc.)
// keycode: USB HID keycode
// Returns 0 on success, -1 on error.
int gostt_usb_hid_send_shortcut(uint8_t modifier, uint8_t keycode);

#endif // GOSTT_KBD_USB_HID_H
```

**Step 2: Create usb_hid.c**

This file contains:
- TinyUSB descriptors (device, config, HID report descriptors for keyboard + consumer)
- ASCII-to-keycode lookup table
- `type_text` function that sends key-down/key-up reports with timing
- `consumer_control` function for mute
- `send_shortcut` function for keyboard combos

The file is substantial (~250 lines) due to the HID report descriptors and keycode table. Key implementation details:

- Use `tinyusb.h` and `class/hid/hid_device.h` from ESP-IDF's TinyUSB component
- Boot keyboard protocol (6KRO) for maximum compatibility
- ASCII lookup table maps 0x20–0x7E to {keycode, needs_shift} pairs
- `vTaskDelay` for inter-key timing (GOSTT_KEY_PRESS_MS, GOSTT_KEY_GAP_MS from config.h)

```c
// firmware/esp32/main/usb_hid.c
#include "usb_hid.h"
#include "config.h"
#include "esp_log.h"
#include "tinyusb.h"
#include "class/hid/hid_device.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static const char *TAG = "gostt-usb";

// HID Report IDs
#define REPORT_ID_KEYBOARD  1
#define REPORT_ID_CONSUMER  2

// HID Report Descriptor: Keyboard + Consumer Control composite
static const uint8_t hid_report_descriptor[] = {
    // Keyboard
    0x05, 0x01,        // Usage Page (Generic Desktop)
    0x09, 0x06,        // Usage (Keyboard)
    0xA1, 0x01,        // Collection (Application)
    0x85, REPORT_ID_KEYBOARD, // Report ID
    0x05, 0x07,        //   Usage Page (Keyboard/Keypad)
    0x19, 0xE0,        //   Usage Minimum (Left Control)
    0x29, 0xE7,        //   Usage Maximum (Right GUI)
    0x15, 0x00,        //   Logical Minimum (0)
    0x25, 0x01,        //   Logical Maximum (1)
    0x75, 0x01,        //   Report Size (1)
    0x95, 0x08,        //   Report Count (8)
    0x81, 0x02,        //   Input (Data, Variable, Absolute) — modifier byte
    0x95, 0x01,        //   Report Count (1)
    0x75, 0x08,        //   Report Size (8)
    0x81, 0x01,        //   Input (Constant) — reserved byte
    0x95, 0x06,        //   Report Count (6)
    0x75, 0x08,        //   Report Size (8)
    0x15, 0x00,        //   Logical Minimum (0)
    0x25, 0x65,        //   Logical Maximum (101)
    0x05, 0x07,        //   Usage Page (Keyboard/Keypad)
    0x19, 0x00,        //   Usage Minimum (0)
    0x29, 0x65,        //   Usage Maximum (101)
    0x81, 0x00,        //   Input (Data, Array) — keycodes
    0xC0,              // End Collection

    // Consumer Control
    0x05, 0x0C,        // Usage Page (Consumer)
    0x09, 0x01,        // Usage (Consumer Control)
    0xA1, 0x01,        // Collection (Application)
    0x85, REPORT_ID_CONSUMER, // Report ID
    0x15, 0x00,        //   Logical Minimum (0)
    0x26, 0xFF, 0x03,  //   Logical Maximum (1023)
    0x19, 0x00,        //   Usage Minimum (0)
    0x2A, 0xFF, 0x03,  //   Usage Maximum (1023)
    0x75, 0x10,        //   Report Size (16)
    0x95, 0x01,        //   Report Count (1)
    0x81, 0x00,        //   Input (Data, Array)
    0xC0,              // End Collection
};

// Keyboard report: modifier + reserved + 6 keycodes
typedef struct __attribute__((packed)) {
    uint8_t modifier;
    uint8_t reserved;
    uint8_t keycodes[6];
} keyboard_report_t;

// ASCII to HID keycode mapping
typedef struct {
    uint8_t keycode;
    bool    shift;
} ascii_to_hid_t;

// Lookup table for ASCII 0x20 (space) through 0x7E (~)
// Index = ascii_code - 0x20
static const ascii_to_hid_t ascii_map[95] = {
    {0x2C, false}, // 0x20 space
    {0x1E, true},  // 0x21 !
    {0x34, true},  // 0x22 "
    {0x20, true},  // 0x23 #
    {0x21, true},  // 0x24 $
    {0x22, true},  // 0x25 %
    {0x24, true},  // 0x26 &
    {0x34, false}, // 0x27 '
    {0x26, true},  // 0x28 (
    {0x27, true},  // 0x29 )
    {0x25, true},  // 0x2A *
    {0x2E, true},  // 0x2B +
    {0x36, false}, // 0x2C ,
    {0x2D, false}, // 0x2D -
    {0x37, false}, // 0x2E .
    {0x38, false}, // 0x2F /
    {0x27, false}, // 0x30 0
    {0x1E, false}, // 0x31 1
    {0x1F, false}, // 0x32 2
    {0x20, false}, // 0x33 3
    {0x21, false}, // 0x34 4
    {0x22, false}, // 0x35 5
    {0x23, false}, // 0x36 6
    {0x24, false}, // 0x37 7
    {0x25, false}, // 0x38 8
    {0x26, false}, // 0x39 9
    {0x33, true},  // 0x3A :
    {0x33, false}, // 0x3B ;
    {0x36, true},  // 0x3C <
    {0x2E, false}, // 0x3D =
    {0x37, true},  // 0x3E >
    {0x38, true},  // 0x3F ?
    {0x1F, true},  // 0x40 @
    {0x04, true},  // 0x41 A
    {0x05, true},  // 0x42 B
    {0x06, true},  // 0x43 C
    {0x07, true},  // 0x44 D
    {0x08, true},  // 0x45 E
    {0x09, true},  // 0x46 F
    {0x0A, true},  // 0x47 G
    {0x0B, true},  // 0x48 H
    {0x0C, true},  // 0x49 I
    {0x0D, true},  // 0x4A J
    {0x0E, true},  // 0x4B K
    {0x0F, true},  // 0x4C L
    {0x10, true},  // 0x4D M
    {0x11, true},  // 0x4E N
    {0x12, true},  // 0x4F O
    {0x13, true},  // 0x50 P
    {0x14, true},  // 0x51 Q
    {0x15, true},  // 0x52 R
    {0x16, true},  // 0x53 S
    {0x17, true},  // 0x54 T
    {0x18, true},  // 0x55 U
    {0x19, true},  // 0x56 V
    {0x1A, true},  // 0x57 W
    {0x1B, true},  // 0x58 X
    {0x1C, true},  // 0x59 Y
    {0x1D, true},  // 0x5A Z
    {0x2F, false}, // 0x5B [
    {0x31, false}, // 0x5C backslash
    {0x30, false}, // 0x5D ]
    {0x23, true},  // 0x5E ^
    {0x2D, true},  // 0x5F _
    {0x35, false}, // 0x60 `
    {0x04, false}, // 0x61 a
    {0x05, false}, // 0x62 b
    {0x06, false}, // 0x63 c
    {0x07, false}, // 0x64 d
    {0x08, false}, // 0x65 e
    {0x09, false}, // 0x66 f
    {0x0A, false}, // 0x67 g
    {0x0B, false}, // 0x68 h
    {0x0C, false}, // 0x69 i
    {0x0D, false}, // 0x6A j
    {0x0E, false}, // 0x6B k
    {0x0F, false}, // 0x6C l
    {0x10, false}, // 0x6D m
    {0x11, false}, // 0x6E n
    {0x12, false}, // 0x6F o
    {0x13, false}, // 0x70 p
    {0x14, false}, // 0x71 q
    {0x15, false}, // 0x72 r
    {0x16, false}, // 0x73 s
    {0x17, false}, // 0x74 t
    {0x18, false}, // 0x75 u
    {0x19, false}, // 0x76 v
    {0x1A, false}, // 0x77 w
    {0x1B, false}, // 0x78 x
    {0x1C, false}, // 0x79 y
    {0x1D, false}, // 0x7A z
    {0x2F, true},  // 0x7B {
    {0x31, true},  // 0x7C |
    {0x30, true},  // 0x7D }
    {0x35, true},  // 0x7E ~
};

int gostt_usb_hid_init(void)
{
    const tinyusb_config_t tusb_cfg = {
        .device_descriptor = NULL,  // use default
        .string_descriptor = NULL,  // use default
        .external_phy = false,
    };

    esp_err_t ret = tinyusb_driver_install(&tusb_cfg);
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "TinyUSB install failed: %s", esp_err_to_name(ret));
        return -1;
    }

    ESP_LOGI(TAG, "USB HID initialized");
    return 0;
}

// TinyUSB callbacks
uint8_t const *tud_hid_descriptor_report_cb(uint8_t instance)
{
    (void)instance;
    return hid_report_descriptor;
}

uint16_t tud_hid_get_report_cb(uint8_t instance, uint8_t report_id,
                                hid_report_type_t report_type,
                                uint8_t *buffer, uint16_t reqlen)
{
    (void)instance; (void)report_id; (void)report_type; (void)buffer; (void)reqlen;
    return 0;
}

void tud_hid_set_report_cb(uint8_t instance, uint8_t report_id,
                            hid_report_type_t report_type,
                            uint8_t const *buffer, uint16_t bufsize)
{
    (void)instance; (void)report_id; (void)report_type; (void)buffer; (void)bufsize;
}

static void send_keyboard_report(uint8_t modifier, uint8_t keycode)
{
    keyboard_report_t report = {0};
    report.modifier = modifier;
    if (keycode) report.keycodes[0] = keycode;
    tud_hid_report(REPORT_ID_KEYBOARD, &report, sizeof(report));
    vTaskDelay(pdMS_TO_TICKS(GOSTT_KEY_PRESS_MS));
}

static void release_keyboard(void)
{
    keyboard_report_t report = {0};
    tud_hid_report(REPORT_ID_KEYBOARD, &report, sizeof(report));
    vTaskDelay(pdMS_TO_TICKS(GOSTT_KEY_GAP_MS));
}

int gostt_usb_hid_type_text(const char *text, size_t len)
{
    if (!tud_mounted()) {
        ESP_LOGW(TAG, "USB not mounted — cannot type");
        return -1;
    }

    for (size_t i = 0; i < len; i++) {
        char c = text[i];
        uint8_t modifier = 0;
        uint8_t keycode = 0;

        if (c == '\n') {
            keycode = 0x28; // Enter
        } else if (c == '\t') {
            keycode = 0x2B; // Tab
        } else if (c >= 0x20 && c <= 0x7E) {
            int idx = c - 0x20;
            keycode = ascii_map[idx].keycode;
            if (ascii_map[idx].shift) modifier = 0x02; // Left Shift
        } else {
            continue; // skip non-ASCII
        }

        send_keyboard_report(modifier, keycode);
        release_keyboard();
    }
    return 0;
}

int gostt_usb_hid_consumer_control(uint16_t usage_id)
{
    if (!tud_mounted()) {
        ESP_LOGW(TAG, "USB not mounted — cannot send consumer control");
        return -1;
    }

    // Press
    tud_hid_report(REPORT_ID_CONSUMER, &usage_id, sizeof(usage_id));
    vTaskDelay(pdMS_TO_TICKS(10));

    // Release
    uint16_t zero = 0;
    tud_hid_report(REPORT_ID_CONSUMER, &zero, sizeof(zero));
    return 0;
}

int gostt_usb_hid_send_shortcut(uint8_t modifier, uint8_t keycode)
{
    if (!tud_mounted()) {
        ESP_LOGW(TAG, "USB not mounted — cannot send shortcut");
        return -1;
    }

    send_keyboard_report(modifier, keycode);
    vTaskDelay(pdMS_TO_TICKS(10)); // hold a bit longer for shortcuts
    release_keyboard();
    return 0;
}
```

**Step 3: Update CMakeLists.txt**

```cmake
SRCS "main.c" "proto.c" "crypto.c" "usb_hid.c"
```

**Step 4: Verify build compiles**

```bash
cd firmware/esp32 && idf.py build
```

**Step 5: Commit**

```bash
git add firmware/esp32/main/usb_hid.h firmware/esp32/main/usb_hid.c firmware/esp32/main/CMakeLists.txt
git commit -m "feat(firmware): add USB HID composite device — keyboard + consumer control"
```

---

### Task 5: Mute Action System

**Files:**
- Create: `firmware/esp32/main/mute.h`
- Create: `firmware/esp32/main/mute.c`
- Modify: `firmware/esp32/main/CMakeLists.txt` — add `mute.c` to SRCS

**Context:** Flexible mute system. Default: USB Consumer Control 0xE2. Reconfigurable over BLE to keyboard shortcuts or multi-step macros. Config persisted in NVS.

**Step 1: Create mute.h**

```c
// firmware/esp32/main/mute.h
#ifndef GOSTT_KBD_MUTE_H
#define GOSTT_KBD_MUTE_H

#include <stdint.h>
#include <stddef.h>

// Mute action types
typedef enum {
    GOSTT_MUTE_CONSUMER_CONTROL = 0,  // Single consumer control key
    GOSTT_MUTE_KEYBOARD_SHORTCUT = 1, // Modifier + keycode combo
    GOSTT_MUTE_MACRO = 2,             // Multi-step sequence (future)
} gostt_mute_type_t;

// Mute action configuration
typedef struct {
    gostt_mute_type_t type;
    union {
        uint16_t usage_id;     // for CONSUMER_CONTROL
        struct {
            uint8_t modifier;  // for KEYBOARD_SHORTCUT
            uint8_t keycode;
        } shortcut;
    };
} gostt_mute_config_t;

// Initialize mute system. Loads config from NVS or uses default.
int gostt_mute_init(void);

// Execute the configured mute action.
int gostt_mute_toggle(void);

// Update mute configuration from BLE command data and persist to NVS.
// Format: byte 0 = type, bytes 1+ = type-specific data.
int gostt_mute_configure(const uint8_t *data, size_t len);

// Get current mute config (for debugging/status).
const gostt_mute_config_t *gostt_mute_get_config(void);

#endif // GOSTT_KBD_MUTE_H
```

**Step 2: Create mute.c**

```c
// firmware/esp32/main/mute.c
#include "mute.h"
#include "usb_hid.h"
#include "config.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "nvs.h"
#include <string.h>

static const char *TAG = "gostt-mute";

static gostt_mute_config_t s_mute_cfg;

static void set_default_config(void)
{
    s_mute_cfg.type = GOSTT_MUTE_CONSUMER_CONTROL;
    s_mute_cfg.usage_id = GOSTT_DEFAULT_MUTE_USAGE_ID;
}

int gostt_mute_init(void)
{
    set_default_config();

    nvs_handle_t handle;
    esp_err_t err = nvs_open(GOSTT_NVS_NAMESPACE, NVS_READONLY, &handle);
    if (err != ESP_OK) {
        ESP_LOGI(TAG, "No NVS mute config — using default (Consumer Control 0x%04X)",
                 GOSTT_DEFAULT_MUTE_USAGE_ID);
        return 0;
    }

    size_t cfg_len = sizeof(s_mute_cfg);
    err = nvs_get_blob(handle, GOSTT_NVS_KEY_MUTE_CFG, &s_mute_cfg, &cfg_len);
    if (err != ESP_OK || cfg_len != sizeof(s_mute_cfg)) {
        set_default_config();
        ESP_LOGI(TAG, "NVS mute config invalid — using default");
    } else {
        ESP_LOGI(TAG, "Loaded mute config from NVS (type=%d)", s_mute_cfg.type);
    }

    nvs_close(handle);
    return 0;
}

int gostt_mute_toggle(void)
{
    switch (s_mute_cfg.type) {
        case GOSTT_MUTE_CONSUMER_CONTROL:
            ESP_LOGI(TAG, "Mute: consumer control 0x%04X", s_mute_cfg.usage_id);
            return gostt_usb_hid_consumer_control(s_mute_cfg.usage_id);

        case GOSTT_MUTE_KEYBOARD_SHORTCUT:
            ESP_LOGI(TAG, "Mute: shortcut mod=0x%02X key=0x%02X",
                     s_mute_cfg.shortcut.modifier, s_mute_cfg.shortcut.keycode);
            return gostt_usb_hid_send_shortcut(s_mute_cfg.shortcut.modifier,
                                                s_mute_cfg.shortcut.keycode);

        case GOSTT_MUTE_MACRO:
            ESP_LOGW(TAG, "Macro mute not yet implemented");
            return -1;

        default:
            ESP_LOGE(TAG, "Unknown mute type: %d", s_mute_cfg.type);
            return -1;
    }
}

int gostt_mute_configure(const uint8_t *data, size_t len)
{
    if (len < 1) return -1;

    gostt_mute_config_t new_cfg = {0};
    new_cfg.type = (gostt_mute_type_t)data[0];

    switch (new_cfg.type) {
        case GOSTT_MUTE_CONSUMER_CONTROL:
            if (len < 3) return -1;
            new_cfg.usage_id = (uint16_t)(data[1] | (data[2] << 8)); // little-endian
            break;

        case GOSTT_MUTE_KEYBOARD_SHORTCUT:
            if (len < 3) return -1;
            new_cfg.shortcut.modifier = data[1];
            new_cfg.shortcut.keycode = data[2];
            break;

        case GOSTT_MUTE_MACRO:
            ESP_LOGW(TAG, "Macro mute configuration not yet implemented");
            return -1;

        default:
            ESP_LOGE(TAG, "Unknown mute type in config: %d", data[0]);
            return -1;
    }

    // Persist to NVS
    nvs_handle_t handle;
    esp_err_t err = nvs_open(GOSTT_NVS_NAMESPACE, NVS_READWRITE, &handle);
    if (err == ESP_OK) {
        nvs_set_blob(handle, GOSTT_NVS_KEY_MUTE_CFG, &new_cfg, sizeof(new_cfg));
        nvs_commit(handle);
        nvs_close(handle);
    }

    s_mute_cfg = new_cfg;
    ESP_LOGI(TAG, "Mute config updated (type=%d)", new_cfg.type);
    return 0;
}

const gostt_mute_config_t *gostt_mute_get_config(void)
{
    return &s_mute_cfg;
}
```

**Step 3: Update CMakeLists.txt**

```cmake
SRCS "main.c" "proto.c" "crypto.c" "usb_hid.c" "mute.c"
```

**Step 4: Verify build compiles**

```bash
cd firmware/esp32 && idf.py build
```

**Step 5: Commit**

```bash
git add firmware/esp32/main/mute.h firmware/esp32/main/mute.c firmware/esp32/main/CMakeLists.txt
git commit -m "feat(firmware): add flexible mute action system with NVS persistence"
```

---

### Task 6: LED Status Controller

**Files:**
- Create: `firmware/esp32/main/led.h`
- Create: `firmware/esp32/main/led.c`
- Modify: `firmware/esp32/main/CMakeLists.txt` — add `led.c` to SRCS

**Context:** WS2812 RGB LED on GPIO 48 via ESP-IDF's `led_strip` component (RMT driver). Five states: ADVERTISING (blue blink), CONNECTED (solid blue), PAIRED (solid green), TYPING (white flash), ERROR (red flash).

**Step 1: Create led.h**

```c
// firmware/esp32/main/led.h
#ifndef GOSTT_KBD_LED_H
#define GOSTT_KBD_LED_H

typedef enum {
    GOSTT_LED_OFF = 0,
    GOSTT_LED_ADVERTISING,  // slow blue blink
    GOSTT_LED_CONNECTED,    // solid blue
    GOSTT_LED_PAIRED,       // solid green
    GOSTT_LED_TYPING,       // brief white flash
    GOSTT_LED_ERROR,        // red flash 3x
    GOSTT_LED_FACTORY_RESET,// red/blue alternating
} gostt_led_state_t;

// Initialize LED. Starts a background FreeRTOS task for animations.
int gostt_led_init(void);

// Set LED state. Thread-safe.
void gostt_led_set(gostt_led_state_t state);

// Briefly flash for typing (returns to previous state after flash).
void gostt_led_flash_typing(void);

// Flash error (returns to previous state after 3 flashes).
void gostt_led_flash_error(void);

#endif // GOSTT_KBD_LED_H
```

**Step 2: Create led.c**

```c
// firmware/esp32/main/led.c
#include "led.h"
#include "config.h"
#include "esp_log.h"
#include "led_strip.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static const char *TAG = "gostt-led";

static led_strip_handle_t s_led_strip = NULL;
static gostt_led_state_t s_state = GOSTT_LED_OFF;
static gostt_led_state_t s_prev_state = GOSTT_LED_OFF;
static volatile bool s_flash_typing = false;
static volatile bool s_flash_error = false;

static void set_color(uint8_t r, uint8_t g, uint8_t b)
{
    if (s_led_strip) {
        led_strip_set_pixel(s_led_strip, 0, r, g, b);
        led_strip_refresh(s_led_strip);
    }
}

static void clear_led(void)
{
    if (s_led_strip) {
        led_strip_clear(s_led_strip);
    }
}

static void led_task(void *arg)
{
    (void)arg;
    bool blink_on = false;
    int tick = 0;

    while (1) {
        // Handle one-shot flashes
        if (s_flash_typing) {
            s_flash_typing = false;
            set_color(40, 40, 40); // dim white
            vTaskDelay(pdMS_TO_TICKS(50));
            // Fall through to restore state
        }

        if (s_flash_error) {
            s_flash_error = false;
            for (int i = 0; i < 3; i++) {
                set_color(60, 0, 0);
                vTaskDelay(pdMS_TO_TICKS(150));
                clear_led();
                vTaskDelay(pdMS_TO_TICKS(150));
            }
            // Fall through to restore state
        }

        switch (s_state) {
            case GOSTT_LED_OFF:
                clear_led();
                break;
            case GOSTT_LED_ADVERTISING:
                if (blink_on) set_color(0, 0, 40);
                else clear_led();
                blink_on = !blink_on;
                break;
            case GOSTT_LED_CONNECTED:
                set_color(0, 0, 40); // solid blue
                break;
            case GOSTT_LED_PAIRED:
                set_color(0, 40, 0); // solid green
                break;
            case GOSTT_LED_TYPING:
                set_color(40, 40, 40); // white
                break;
            case GOSTT_LED_FACTORY_RESET:
                if (blink_on) set_color(60, 0, 0);
                else set_color(0, 0, 60);
                blink_on = !blink_on;
                break;
            default:
                clear_led();
                break;
        }

        // Tick at ~10Hz for smooth blink. Advertising blinks at ~1Hz (every 5 ticks).
        tick++;
        if (s_state == GOSTT_LED_ADVERTISING) {
            vTaskDelay(pdMS_TO_TICKS(500));
        } else {
            vTaskDelay(pdMS_TO_TICKS(100));
        }
    }
}

int gostt_led_init(void)
{
    led_strip_config_t strip_config = {
        .strip_gpio_num = GOSTT_LED_GPIO,
        .max_leds = 1,
    };
    led_strip_rmt_config_t rmt_config = {
        .resolution_hz = 10 * 1000 * 1000, // 10 MHz
    };

    esp_err_t err = led_strip_new_rmt_device(&strip_config, &rmt_config, &s_led_strip);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "LED strip init failed: %s", esp_err_to_name(err));
        return -1;
    }

    clear_led();

    xTaskCreate(led_task, "led_task", 2048, NULL, 2, NULL);
    ESP_LOGI(TAG, "LED initialized on GPIO %d", GOSTT_LED_GPIO);
    return 0;
}

void gostt_led_set(gostt_led_state_t state)
{
    s_prev_state = s_state;
    s_state = state;
}

void gostt_led_flash_typing(void)
{
    s_flash_typing = true;
}

void gostt_led_flash_error(void)
{
    s_flash_error = true;
}
```

**Step 3: Update CMakeLists.txt**

```cmake
SRCS "main.c" "proto.c" "crypto.c" "usb_hid.c" "mute.c" "led.c"
```

**Step 4: Verify build compiles**

```bash
cd firmware/esp32 && idf.py build
```

**Step 5: Commit**

```bash
git add firmware/esp32/main/led.h firmware/esp32/main/led.c firmware/esp32/main/CMakeLists.txt
git commit -m "feat(firmware): add WS2812 LED status controller with state machine"
```

---

### Task 7: BLE GATT Server

**Files:**
- Create: `firmware/esp32/main/ble_server.h`
- Create: `firmware/esp32/main/ble_server.c`
- Modify: `firmware/esp32/main/CMakeLists.txt` — add `ble_server.c` to SRCS

**Context:** NimBLE GATT server. Advertises as "GOSTT-KBD". Service with TX (write), Response (notify), MAC (read) characteristics. On TX write: either handle pairing (33-byte raw pubkey) or decrypt DataPacket → dispatch to typing or mute.

**Step 1: Create ble_server.h**

```c
// firmware/esp32/main/ble_server.h
#ifndef GOSTT_KBD_BLE_SERVER_H
#define GOSTT_KBD_BLE_SERVER_H

#include "crypto.h"

// Callback for when decrypted text is ready to type.
typedef void (*gostt_text_callback_t)(const char *text, size_t len);

// Callback for commands (mute toggle, configure mute, etc.)
typedef void (*gostt_command_callback_t)(uint32_t command_type,
                                         const uint8_t *data, size_t data_len);

// BLE server configuration
typedef struct {
    gostt_crypto_ctx_t    *crypto;         // crypto context for pairing/decrypt
    gostt_text_callback_t  on_text;        // called when text is decrypted
    gostt_command_callback_t on_command;    // called for non-text commands
} gostt_ble_config_t;

// Initialize and start the BLE GATT server.
int gostt_ble_server_init(const gostt_ble_config_t *config);

// Check if a BLE client is currently connected.
bool gostt_ble_is_connected(void);

#endif // GOSTT_KBD_BLE_SERVER_H
```

**Step 2: Create ble_server.c**

This is the largest single file (~200-250 lines). It sets up:
- NimBLE host with GATT service definition
- Advertising parameters
- Write handler for TX characteristic (pairing detection + DataPacket decrypt)
- Notification handler for Response characteristic (keepalive timer)
- Read handler for MAC characteristic
- Connection/disconnection GAP event handlers
- Keepalive timer task

```c
// firmware/esp32/main/ble_server.c
#include "ble_server.h"
#include "config.h"
#include "proto.h"
#include "led.h"
#include "esp_log.h"
#include "esp_nimble_hci.h"
#include "nimble/nimble_port.h"
#include "nimble/nimble_port_freertos.h"
#include "host/ble_hs.h"
#include "host/util/util.h"
#include "services/gap/ble_svc_gap.h"
#include "services/gatt/ble_svc_gatt.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/timers.h"
#include <string.h>

static const char *TAG = "gostt-ble";

static gostt_ble_config_t s_config;
static uint16_t s_conn_handle = BLE_HS_CONN_HANDLE_NONE;
static uint16_t s_resp_attr_handle;
static bool s_connected = false;
static TimerHandle_t s_keepalive_timer = NULL;

// Forward declarations
static int ble_gap_event(struct ble_gap_event *event, void *arg);
static void start_advertising(void);

// --- GATT Characteristic Handlers ---

// TX characteristic write handler (app sends encrypted data or pairing key here)
static int tx_char_write_cb(uint16_t conn_handle, uint16_t attr_handle,
                             struct ble_gatt_access_ctxt *ctxt, void *arg)
{
    (void)conn_handle; (void)attr_handle; (void)arg;

    struct os_mbuf *om = ctxt->om;
    uint16_t len = OS_MBUF_PKTLEN(om);
    if (len == 0) return 0;

    uint8_t buf[512];
    if (len > sizeof(buf)) {
        ESP_LOGW(TAG, "TX write too large: %d", len);
        return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    }
    os_mbuf_copydata(om, 0, len, buf);

    // Pairing detection: 33-byte compressed public key (not a DataPacket)
    if (len == GOSTT_COMPRESSED_PUBKEY_LEN &&
        (buf[0] == 0x02 || buf[0] == 0x03)) {
        ESP_LOGI(TAG, "Pairing request received (33-byte pubkey)");

        uint8_t own_pubkey[GOSTT_COMPRESSED_PUBKEY_LEN];
        if (gostt_crypto_pair(s_config.crypto, buf, own_pubkey) != 0) {
            ESP_LOGE(TAG, "Pairing failed");
            gostt_led_flash_error();
            return 0;
        }

        // Send our public key back as ResponsePacket(PeerStatus, Known, own_pubkey)
        uint8_t resp_buf[128];
        int resp_len = gostt_encode_response_packet(resp_buf, sizeof(resp_buf),
                                                     GOSTT_RESP_PEER_STATUS,
                                                     GOSTT_PEER_KNOWN,
                                                     own_pubkey,
                                                     GOSTT_COMPRESSED_PUBKEY_LEN);
        if (resp_len > 0 && s_conn_handle != BLE_HS_CONN_HANDLE_NONE) {
            struct os_mbuf *om_resp = ble_hs_mbuf_from_flat(resp_buf, resp_len);
            if (om_resp) {
                ble_gatts_notify_custom(s_conn_handle, s_resp_attr_handle, om_resp);
            }
        }

        gostt_led_set(GOSTT_LED_PAIRED);
        ESP_LOGI(TAG, "Pairing complete");
        return 0;
    }

    // Normal operation: decode DataPacket, decrypt, dispatch
    if (!s_config.crypto->has_key) {
        ESP_LOGW(TAG, "No key — ignoring encrypted packet");
        return 0;
    }

    gostt_data_packet_t pkt;
    if (gostt_decode_data_packet(buf, len, &pkt) != 0) {
        ESP_LOGW(TAG, "Failed to decode DataPacket");
        gostt_led_flash_error();
        return 0;
    }

    // Decrypt
    uint8_t plaintext[512];
    int pt_len = gostt_crypto_decrypt(s_config.crypto,
                                       pkt.iv, pkt.tag,
                                       pkt.encrypted_data, pkt.encrypted_data_len,
                                       plaintext);
    if (pt_len < 0) {
        ESP_LOGW(TAG, "Decrypt failed for packet %u", pkt.packet_num);
        gostt_led_flash_error();
        return 0;
    }

    // Decode EncryptedData wrapper
    gostt_encrypted_data_t enc_data;
    if (gostt_decode_encrypted_data(plaintext, (size_t)pt_len, &enc_data) != 0) {
        ESP_LOGW(TAG, "Failed to decode EncryptedData");
        gostt_led_flash_error();
        return 0;
    }

    if (enc_data.command_type == 0 && enc_data.keyboard_packet_data != NULL) {
        // Text command
        gostt_keyboard_packet_t kbd;
        if (gostt_decode_keyboard_packet(enc_data.keyboard_packet_data,
                                          enc_data.keyboard_packet_data_len, &kbd) == 0) {
            gostt_led_flash_typing();
            if (s_config.on_text) {
                s_config.on_text(kbd.message, kbd.message_len);
            }
        }
    } else if (enc_data.command_type > 0 && s_config.on_command) {
        s_config.on_command(enc_data.command_type,
                           enc_data.command_data, enc_data.command_data_len);
    }

    return 0;
}

// Response characteristic: supports notifications only (no read/write from client)
// The attribute handle is captured during registration for sending notifications.

// MAC characteristic read handler
static int mac_char_read_cb(uint16_t conn_handle, uint16_t attr_handle,
                             struct ble_gatt_access_ctxt *ctxt, void *arg)
{
    (void)conn_handle; (void)attr_handle; (void)arg;

    uint8_t mac[6];
    ble_hs_id_copy_addr(BLE_ADDR_PUBLIC, mac, NULL);
    os_mbuf_append(ctxt->om, mac, sizeof(mac));
    return 0;
}

// --- GATT Service Definition ---

// UUIDs (128-bit)
static const ble_uuid128_t svc_uuid =
    BLE_UUID128_INIT(0x14, 0x12, 0x8a, 0x76, 0x04, 0xd1, 0x6c, 0x4f,
                     0x7e, 0x53, 0xf2, 0xe8, 0x00, 0x00, 0xb1, 0x19);
static const ble_uuid128_t tx_char_uuid =
    BLE_UUID128_INIT(0x07, 0x59, 0x2c, 0xdd, 0x7d, 0xcf, 0x42, 0xbf,
                     0x5a, 0x45, 0x7b, 0x2c, 0x19, 0xe1, 0x56, 0x68);
static const ble_uuid128_t resp_char_uuid =
    BLE_UUID128_INIT(0x08, 0x59, 0x2c, 0xdd, 0x7d, 0xcf, 0x42, 0xbf,
                     0x5a, 0x45, 0x7b, 0x2c, 0x19, 0xe1, 0x56, 0x68);
static const ble_uuid128_t mac_char_uuid =
    BLE_UUID128_INIT(0x14, 0x12, 0x8a, 0x76, 0x04, 0xd1, 0x6c, 0x4f,
                     0x7e, 0x53, 0xf2, 0xe8, 0x02, 0x00, 0xb1, 0x19);

static const struct ble_gatt_svc_def gatt_svr_svcs[] = {
    {
        .type = BLE_GATT_SVC_TYPE_PRIMARY,
        .uuid = &svc_uuid.u,
        .characteristics = (struct ble_gatt_chr_def[]) {
            {
                // TX characteristic (write)
                .uuid = &tx_char_uuid.u,
                .access_cb = tx_char_write_cb,
                .flags = BLE_GATT_CHR_F_WRITE | BLE_GATT_CHR_F_WRITE_NO_RSP,
            },
            {
                // Response characteristic (notify)
                .uuid = &resp_char_uuid.u,
                .access_cb = NULL,
                .val_handle = &s_resp_attr_handle,
                .flags = BLE_GATT_CHR_F_NOTIFY,
            },
            {
                // MAC characteristic (read)
                .uuid = &mac_char_uuid.u,
                .access_cb = mac_char_read_cb,
                .flags = BLE_GATT_CHR_F_READ,
            },
            {0}, // sentinel
        },
    },
    {0}, // sentinel
};

// --- Keepalive Timer ---

static void keepalive_timer_cb(TimerHandle_t timer)
{
    (void)timer;
    if (s_conn_handle == BLE_HS_CONN_HANDLE_NONE) return;

    uint8_t buf[16];
    int len = gostt_encode_response_packet(buf, sizeof(buf),
                                            GOSTT_RESP_KEEPALIVE,
                                            GOSTT_PEER_UNKNOWN,
                                            NULL, 0);
    if (len > 0) {
        struct os_mbuf *om = ble_hs_mbuf_from_flat(buf, len);
        if (om) {
            ble_gatts_notify_custom(s_conn_handle, s_resp_attr_handle, om);
        }
    }
}

// --- GAP Event Handler ---

static void start_advertising(void)
{
    struct ble_gap_adv_params adv_params = {0};
    adv_params.conn_mode = BLE_GAP_CONN_MODE_UND;
    adv_params.disc_mode = BLE_GAP_DISC_MODE_GEN;

    struct ble_hs_adv_fields fields = {0};
    fields.flags = BLE_HS_ADV_F_DISC_GEN | BLE_HS_ADV_F_BREDR_UNSUP;
    fields.name = (uint8_t *)GOSTT_BLE_DEVICE_NAME;
    fields.name_len = strlen(GOSTT_BLE_DEVICE_NAME);
    fields.name_is_complete = 1;

    ble_gap_adv_set_fields(&fields);

    // Advertise service UUID in scan response
    struct ble_hs_adv_fields rsp_fields = {0};
    rsp_fields.uuids128 = (ble_uuid128_t[]){{
        BLE_UUID128_INIT(0x14, 0x12, 0x8a, 0x76, 0x04, 0xd1, 0x6c, 0x4f,
                         0x7e, 0x53, 0xf2, 0xe8, 0x00, 0x00, 0xb1, 0x19)
    }};
    rsp_fields.num_uuids128 = 1;
    rsp_fields.uuids128_is_complete = 1;
    ble_gap_adv_rsp_set_fields(&rsp_fields);

    ble_gap_adv_start(BLE_OWN_ADDR_PUBLIC, NULL, BLE_HS_FOREVER,
                      &adv_params, ble_gap_event, NULL);

    ESP_LOGI(TAG, "Advertising started as '%s'", GOSTT_BLE_DEVICE_NAME);
    gostt_led_set(GOSTT_LED_ADVERTISING);
}

static int ble_gap_event(struct ble_gap_event *event, void *arg)
{
    (void)arg;

    switch (event->type) {
        case BLE_GAP_EVENT_CONNECT:
            if (event->connect.status == 0) {
                s_conn_handle = event->connect.conn_handle;
                s_connected = true;
                ESP_LOGI(TAG, "Client connected (handle=%d)", s_conn_handle);

                if (s_config.crypto->has_key) {
                    gostt_led_set(GOSTT_LED_PAIRED);
                } else {
                    gostt_led_set(GOSTT_LED_CONNECTED);
                }

                // Start keepalive timer
                if (s_keepalive_timer) {
                    xTimerStart(s_keepalive_timer, 0);
                }
            } else {
                start_advertising();
            }
            break;

        case BLE_GAP_EVENT_DISCONNECT:
            ESP_LOGI(TAG, "Client disconnected (reason=%d)", event->disconnect.reason);
            s_conn_handle = BLE_HS_CONN_HANDLE_NONE;
            s_connected = false;

            if (s_keepalive_timer) {
                xTimerStop(s_keepalive_timer, 0);
            }

            start_advertising();
            break;

        case BLE_GAP_EVENT_MTU:
            ESP_LOGI(TAG, "MTU updated: %d", event->mtu.value);
            break;

        default:
            break;
    }
    return 0;
}

// --- NimBLE Host Task ---

static void ble_host_task(void *param)
{
    (void)param;
    nimble_port_run();
    nimble_port_freertos_deinit();
}

static void on_sync(void)
{
    ble_hs_util_ensure_addr(0);
    start_advertising();
}

// --- Public API ---

int gostt_ble_server_init(const gostt_ble_config_t *config)
{
    s_config = *config;

    // Initialize NimBLE
    esp_err_t ret = nimble_port_init();
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "NimBLE init failed: %s", esp_err_to_name(ret));
        return -1;
    }

    // Register GATT services
    ble_svc_gap_init();
    ble_svc_gatt_init();

    int rc = ble_gatts_count_cfg(gatt_svr_svcs);
    if (rc != 0) {
        ESP_LOGE(TAG, "GATT count cfg failed: %d", rc);
        return -1;
    }
    rc = ble_gatts_add_svcs(gatt_svr_svcs);
    if (rc != 0) {
        ESP_LOGE(TAG, "GATT add svcs failed: %d", rc);
        return -1;
    }

    ble_svc_gap_device_name_set(GOSTT_BLE_DEVICE_NAME);

    ble_hs_cfg.sync_cb = on_sync;

    // Create keepalive timer
    s_keepalive_timer = xTimerCreate("keepalive",
                                      pdMS_TO_TICKS(GOSTT_KEEPALIVE_INTERVAL_MS),
                                      pdTRUE, NULL, keepalive_timer_cb);

    // Start NimBLE host task
    nimble_port_freertos_init(ble_host_task);

    ESP_LOGI(TAG, "BLE server initialized");
    return 0;
}

bool gostt_ble_is_connected(void)
{
    return s_connected;
}
```

**Step 3: Update CMakeLists.txt**

```cmake
SRCS "main.c" "proto.c" "crypto.c" "usb_hid.c" "mute.c" "led.c" "ble_server.c"
```

**Step 4: Verify build compiles**

```bash
cd firmware/esp32 && idf.py build
```

**Step 5: Commit**

```bash
git add firmware/esp32/main/ble_server.h firmware/esp32/main/ble_server.c firmware/esp32/main/CMakeLists.txt
git commit -m "feat(firmware): add BLE GATT server with pairing, decrypt, and dispatch"
```

---

### Task 8: Main Integration (State Machine + Factory Reset)

**Files:**
- Modify: `firmware/esp32/main/main.c` — full integration of all subsystems

**Context:** Wire everything together in `app_main()`. Initialize all subsystems, set up callbacks, implement factory reset via BOOT button.

**Step 1: Update main.c**

```c
// firmware/esp32/main/main.c
#include <stdio.h>
#include <string.h>
#include "esp_log.h"
#include "esp_system.h"
#include "nvs_flash.h"
#include "driver/gpio.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

#include "config.h"
#include "led.h"
#include "crypto.h"
#include "usb_hid.h"
#include "mute.h"
#include "ble_server.h"

static const char *TAG = "gostt-kbd";

static gostt_crypto_ctx_t s_crypto;

// Callback: text received from BLE, type it
static void on_text_received(const char *text, size_t len)
{
    ESP_LOGI(TAG, "Typing %zu chars", len);
    gostt_usb_hid_type_text(text, len);
}

// Callback: command received from BLE
static void on_command_received(uint32_t command_type,
                                 const uint8_t *data, size_t data_len)
{
    switch (command_type) {
        case 1: // mute toggle
            ESP_LOGI(TAG, "Mute toggle command");
            gostt_mute_toggle();
            break;
        case 2: // configure mute
            ESP_LOGI(TAG, "Configure mute command (%zu bytes)", data_len);
            gostt_mute_configure(data, data_len);
            break;
        default:
            ESP_LOGW(TAG, "Unknown command type: %u", (unsigned)command_type);
            break;
    }
}

// Check if BOOT button is held for factory reset
static void check_factory_reset(void)
{
    // BOOT button is typically GPIO 0, active low
    gpio_config_t io_conf = {
        .pin_bit_mask = (1ULL << GPIO_NUM_0),
        .mode = GPIO_MODE_INPUT,
        .pull_up_en = GPIO_PULLUP_ENABLE,
        .pull_down_en = GPIO_PULLDOWN_DISABLE,
    };
    gpio_config(&io_conf);

    if (gpio_get_level(GPIO_NUM_0) == 0) {
        ESP_LOGW(TAG, "BOOT button held — checking for factory reset...");
        int held_ms = 0;
        while (gpio_get_level(GPIO_NUM_0) == 0 && held_ms < GOSTT_FACTORY_RESET_MS) {
            vTaskDelay(pdMS_TO_TICKS(100));
            held_ms += 100;
        }
        if (held_ms >= GOSTT_FACTORY_RESET_MS) {
            ESP_LOGW(TAG, "Factory reset triggered!");
            gostt_led_set(GOSTT_LED_FACTORY_RESET);
            gostt_crypto_erase(&s_crypto);
            vTaskDelay(pdMS_TO_TICKS(2000)); // show animation
            esp_restart();
        }
    }
}

void app_main(void)
{
    ESP_LOGI(TAG, "GOSTT-KBD firmware starting...");

    // Initialize NVS
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        ret = nvs_flash_init();
    }
    ESP_ERROR_CHECK(ret);

    // Check for factory reset before initializing subsystems
    check_factory_reset();

    // Initialize LED
    gostt_led_init();
    gostt_led_set(GOSTT_LED_OFF);

    // Initialize crypto (loads key from NVS if available)
    gostt_crypto_init(&s_crypto);

    // Initialize USB HID
    gostt_usb_hid_init();

    // Initialize mute system
    gostt_mute_init();

    // Initialize BLE server
    gostt_ble_config_t ble_cfg = {
        .crypto = &s_crypto,
        .on_text = on_text_received,
        .on_command = on_command_received,
    };
    gostt_ble_server_init(&ble_cfg);

    ESP_LOGI(TAG, "GOSTT-KBD ready — %s",
             s_crypto.has_key ? "paired (key loaded)" : "awaiting pairing");
}
```

**Step 2: Verify build compiles**

```bash
cd firmware/esp32 && idf.py build
```

**Step 3: Commit**

```bash
git add firmware/esp32/main/main.c
git commit -m "feat(firmware): integrate all subsystems in main with factory reset"
```

---

### Task 9: Taskfile Integration

**Files:**
- Modify: `Taskfile.yml` — add `fw-build`, `fw-flash`, `fw-monitor` tasks

**Step 1: Add firmware tasks to Taskfile.yml**

Add the following tasks (the exact insertion point depends on current Taskfile structure — add them alongside existing tasks):

```yaml
  fw-build:
    desc: Build ESP32-S3 firmware
    dir: firmware/esp32
    cmds:
      - idf.py build

  fw-flash:
    desc: Flash firmware to connected ESP32-S3
    dir: firmware/esp32
    cmds:
      - idf.py -p {{.CLI_ARGS | default "/dev/cu.usbmodem*"}} flash

  fw-monitor:
    desc: Serial monitor for ESP32-S3
    dir: firmware/esp32
    interactive: true
    cmds:
      - idf.py -p {{.CLI_ARGS | default "/dev/cu.usbmodem*"}} monitor

  fw-flash-monitor:
    desc: Flash firmware and open serial monitor
    dir: firmware/esp32
    interactive: true
    cmds:
      - idf.py -p {{.CLI_ARGS | default "/dev/cu.usbmodem*"}} flash monitor
```

**Step 2: Verify tasks appear in listing**

```bash
task --list
```

**Step 3: Commit**

```bash
git add Taskfile.yml
git commit -m "feat: add firmware build/flash/monitor tasks to Taskfile"
```

---

### Task 10: Cross-Validation Test (Go ↔ C Protocol Compatibility)

**Files:**
- Create: `firmware/esp32/test/test_proto.c` — host-compilable C test for protobuf codec
- Create: `firmware/esp32/test/Makefile` — simple host GCC build for tests

**Context:** The C protobuf decoder must produce identical results to the Go encoder, and vice versa. We test this by using the exact golden byte sequences from `internal/ble/protocol/proto_test.go` in the C tests.

**Step 1: Create test directory and test_proto.c**

```c
// firmware/esp32/test/test_proto.c
// Host-compilable test (not ESP-IDF) — validates protobuf wire compatibility with Go app.
// Compile: gcc -I../main -o test_proto test_proto.c ../main/proto.c
#include <stdio.h>
#include <string.h>
#include <assert.h>
#include "proto.h"

static int tests_run = 0;
static int tests_passed = 0;

#define TEST(name) do { printf("  %-50s ", #name); tests_run++; } while(0)
#define PASS() do { printf("PASS\n"); tests_passed++; } while(0)
#define FAIL(msg) do { printf("FAIL: %s\n", msg); } while(0)

// Golden bytes from Go proto_test.go
void test_decode_keyboard_packet_hello(void)
{
    TEST(decode_keyboard_packet_hello);
    // MarshalKeyboardPacket("hello") from Go:
    uint8_t data[] = {0x0a, 0x05, 'h', 'e', 'l', 'l', 'o', 0x10, 0x05};
    gostt_keyboard_packet_t pkt;
    int rc = gostt_decode_keyboard_packet(data, sizeof(data), &pkt);
    assert(rc == 0);
    assert(pkt.message_len == 5);
    assert(memcmp(pkt.message, "hello", 5) == 0);
    assert(pkt.length == 5);
    PASS();
}

void test_decode_keyboard_packet_empty(void)
{
    TEST(decode_keyboard_packet_empty);
    // MarshalKeyboardPacket("") from Go:
    uint8_t data[] = {0x0a, 0x00, 0x10, 0x00};
    gostt_keyboard_packet_t pkt;
    int rc = gostt_decode_keyboard_packet(data, sizeof(data), &pkt);
    assert(rc == 0);
    assert(pkt.message_len == 0);
    assert(pkt.length == 0);
    PASS();
}

void test_encode_response_packet(void)
{
    TEST(encode_response_packet);
    // Expected from Go test: type=1, status=0, data=[0xDE, 0xAD]
    uint8_t expected[] = {0x08, 0x01, 0x10, 0x00, 0x1a, 0x02, 0xDE, 0xAD};
    uint8_t peer_data[] = {0xDE, 0xAD};

    uint8_t buf[64];
    int len = gostt_encode_response_packet(buf, sizeof(buf),
                                            GOSTT_RESP_PEER_STATUS,
                                            GOSTT_PEER_UNKNOWN,
                                            peer_data, sizeof(peer_data));
    assert(len == (int)sizeof(expected));
    assert(memcmp(buf, expected, len) == 0);
    PASS();
}

void test_decode_data_packet(void)
{
    TEST(decode_data_packet);
    // Build DataPacket golden bytes matching Go test:
    // iv: 0xAA followed by 11 zeros (12 bytes)
    // tag: 0xBB followed by 15 zeros (16 bytes)
    // encrypted: [0x01, 0x02, 0x03]
    // packet_num: 42 (varint 0x2a)
    uint8_t data[] = {
        0x0a, 0x0c, // field 1: iv (12 bytes)
        0xAA, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x12, 0x10, // field 2: tag (16 bytes)
        0xBB, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x1a, 0x03, // field 3: encrypted (3 bytes)
        0x01, 0x02, 0x03,
        0x20, 0x2a, // field 4: packet_num = 42
    };

    gostt_data_packet_t pkt;
    int rc = gostt_decode_data_packet(data, sizeof(data), &pkt);
    assert(rc == 0);
    assert(pkt.iv[0] == 0xAA);
    assert(pkt.iv[1] == 0x00);
    assert(pkt.tag[0] == 0xBB);
    assert(pkt.encrypted_data_len == 3);
    assert(pkt.encrypted_data[0] == 0x01);
    assert(pkt.encrypted_data[1] == 0x02);
    assert(pkt.encrypted_data[2] == 0x03);
    assert(pkt.packet_num == 42);
    PASS();
}

void test_decode_encrypted_data(void)
{
    TEST(decode_encrypted_data);
    // EncryptedData wrapping a KeyboardPacket("hello"):
    // inner = MarshalKeyboardPacket("hello") = [0x0a, 0x05, 'h','e','l','l','o', 0x10, 0x05]
    // MarshalEncryptedData(inner) = [0x0a, 0x09, ...inner...]
    uint8_t data[] = {
        0x0a, 0x09,
        0x0a, 0x05, 'h', 'e', 'l', 'l', 'o', 0x10, 0x05
    };

    gostt_encrypted_data_t enc;
    int rc = gostt_decode_encrypted_data(data, sizeof(data), &enc);
    assert(rc == 0);
    assert(enc.keyboard_packet_data != NULL);
    assert(enc.keyboard_packet_data_len == 9);
    assert(enc.command_type == 0);
    PASS();
}

int main(void)
{
    printf("GOSTT-KBD Protocol Cross-Validation Tests\n");
    printf("==========================================\n");

    test_decode_keyboard_packet_hello();
    test_decode_keyboard_packet_empty();
    test_encode_response_packet();
    test_decode_data_packet();
    test_decode_encrypted_data();

    printf("\n%d/%d tests passed\n", tests_passed, tests_run);
    return (tests_passed == tests_run) ? 0 : 1;
}
```

**Step 2: Create Makefile for host tests**

```makefile
# firmware/esp32/test/Makefile
CC = gcc
CFLAGS = -Wall -Wextra -I../main -std=c17

test_proto: test_proto.c ../main/proto.c
	$(CC) $(CFLAGS) -o $@ $^
	./$@

clean:
	rm -f test_proto

.PHONY: clean
```

**Step 3: Run tests**

```bash
cd firmware/esp32/test && make test_proto
```

Expected output:
```
GOSTT-KBD Protocol Cross-Validation Tests
==========================================
  decode_keyboard_packet_hello                       PASS
  decode_keyboard_packet_empty                       PASS
  encode_response_packet                             PASS
  decode_data_packet                                 PASS
  decode_encrypted_data                              PASS

5/5 tests passed
```

**Step 4: Add test task to Taskfile.yml**

```yaml
  fw-test:
    desc: Run firmware host-side unit tests
    dir: firmware/esp32/test
    cmds:
      - make test_proto
```

**Step 5: Commit**

```bash
git add firmware/esp32/test/ Taskfile.yml
git commit -m "test(firmware): add host-side protobuf cross-validation tests"
```

---

### Task 11: README and Documentation Update

**Files:**
- Modify: `README.md` — add ESP32-S3 firmware section

**Step 1: Add firmware documentation to README**

Add a new section covering:
- Hardware requirements (any ESP32-S3 dev board)
- Firmware build/flash instructions (requires ESP-IDF v5.x)
- Pairing workflow: `task ble-pair` on the Mac, firmware auto-detects pairing
- LED status guide (what each color means)
- Factory reset instructions (hold BOOT 5s)
- Mute configuration (default and how to change)
- Troubleshooting common issues

Keep it concise — reference the design doc for architecture details.

**Step 2: Verify README renders correctly**

Quick visual check of markdown formatting.

**Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add ESP32-S3 firmware setup and usage to README"
```

---

## Task Summary

| # | Task | Est. Lines | Dependencies |
|---|------|-----------|--------------|
| 1 | Project scaffold | ~80 | None |
| 2 | Protobuf decoder/encoder | ~200 | Task 1 |
| 3 | Crypto engine | ~220 | Task 1 |
| 4 | USB HID output | ~280 | Task 1 |
| 5 | Mute action system | ~130 | Task 4 |
| 6 | LED status controller | ~150 | Task 1 |
| 7 | BLE GATT server | ~280 | Tasks 2, 3, 6 |
| 8 | Main integration | ~90 | Tasks 4, 5, 6, 7 |
| 9 | Taskfile integration | ~20 | Task 1 |
| 10 | Cross-validation tests | ~120 | Task 2 |
| 11 | README update | ~50 | All |

**Total estimated:** ~1,600 lines (slightly above initial estimate due to thorough test coverage and mute system).

**Critical path:** 1 → 2 → 7 → 8 (scaffold → proto → BLE server → main). Tasks 3, 4, 5, 6 can run in parallel after Task 1.
