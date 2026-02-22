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
#define GOSTT_SHORTCUT_HOLD_MS      10
#define GOSTT_CONSUMER_PRESS_MS     10
#define GOSTT_HID_READY_TIMEOUT_MS  50

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

// BLE command types (must match Go app)
#define GOSTT_CMD_MUTE_TOGGLE    1
#define GOSTT_CMD_MUTE_CONFIGURE 2

#endif // GOSTT_KBD_CONFIG_H
