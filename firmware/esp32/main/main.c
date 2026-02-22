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
