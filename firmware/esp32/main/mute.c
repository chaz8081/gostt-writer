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
