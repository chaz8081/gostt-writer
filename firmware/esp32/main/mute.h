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
