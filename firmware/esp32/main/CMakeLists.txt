idf_component_register(
    SRCS "main.c" "proto.c"
    INCLUDE_DIRS "."
    REQUIRES bt nvs_flash tinyusb driver led_strip
)
