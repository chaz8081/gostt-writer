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
