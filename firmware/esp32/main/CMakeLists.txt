idf_component_register(
    SRCS "main.c" "proto.c" "crypto.c" "usb_hid.c" "mute.c" "led.c"
    INCLUDE_DIRS "."
    REQUIRES bt nvs_flash tinyusb driver led_strip
)
