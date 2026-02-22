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
