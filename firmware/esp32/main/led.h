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
