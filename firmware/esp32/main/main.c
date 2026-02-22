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
