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
