idf_component_register(
    SRCS "main.c" "proto.c" "crypto.c" "usb_hid.c" "mute.c" "led.c" "ble_server.c"
    INCLUDE_DIRS "."
    REQUIRES bt nvs_flash esp_tinyusb driver led_strip mbedtls
)
