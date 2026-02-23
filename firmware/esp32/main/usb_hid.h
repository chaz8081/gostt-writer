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
// Text is queued to a dedicated typer task and typed asynchronously.
// Returns 0 on success (queued), -1 on error.
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
