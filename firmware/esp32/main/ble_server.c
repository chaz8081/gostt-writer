// firmware/esp32/main/ble_server.c
#include "ble_server.h"
#include "config.h"
#include "proto.h"
#include "led.h"
#include "esp_log.h"
#include "esp_nimble_hci.h"
#include "nimble/nimble_port.h"
#include "nimble/nimble_port_freertos.h"
#include "host/ble_hs.h"
#include "host/util/util.h"
#include "services/gap/ble_svc_gap.h"
#include "services/gatt/ble_svc_gatt.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/timers.h"
#include <string.h>

static const char *TAG = "gostt-ble";

static gostt_ble_config_t s_config;
static uint16_t s_conn_handle = BLE_HS_CONN_HANDLE_NONE;
static uint16_t s_resp_attr_handle;
static bool s_connected = false;
static TimerHandle_t s_keepalive_timer = NULL;

// Forward declarations
static int ble_gap_event(struct ble_gap_event *event, void *arg);
static void start_advertising(void);

// --- GATT Characteristic Handlers ---

// TX characteristic write handler (app sends encrypted data or pairing key here)
static int tx_char_write_cb(uint16_t conn_handle, uint16_t attr_handle,
                             struct ble_gatt_access_ctxt *ctxt, void *arg)
{
    (void)conn_handle; (void)attr_handle; (void)arg;

    struct os_mbuf *om = ctxt->om;
    uint16_t len = OS_MBUF_PKTLEN(om);
    if (len == 0) return 0;

    uint8_t buf[512];
    if (len > sizeof(buf)) {
        ESP_LOGW(TAG, "TX write too large: %d", len);
        return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    }
    os_mbuf_copydata(om, 0, len, buf);

    // Pairing detection: 33-byte compressed public key (not a DataPacket)
    if (len == GOSTT_COMPRESSED_PUBKEY_LEN &&
        (buf[0] == 0x02 || buf[0] == 0x03)) {
        ESP_LOGI(TAG, "Pairing request received (33-byte pubkey)");

        uint8_t own_pubkey[GOSTT_COMPRESSED_PUBKEY_LEN];
        if (gostt_crypto_pair(s_config.crypto, buf, own_pubkey) != 0) {
            ESP_LOGE(TAG, "Pairing failed");
            gostt_led_flash_error();
            return 0;
        }

        // Send our public key back as ResponsePacket(PeerStatus, Known, own_pubkey)
        uint8_t resp_buf[128];
        int resp_len = gostt_encode_response_packet(resp_buf, sizeof(resp_buf),
                                                     GOSTT_RESP_PEER_STATUS,
                                                     GOSTT_PEER_KNOWN,
                                                     own_pubkey,
                                                     GOSTT_COMPRESSED_PUBKEY_LEN);
        if (resp_len > 0 && s_conn_handle != BLE_HS_CONN_HANDLE_NONE) {
            struct os_mbuf *om_resp = ble_hs_mbuf_from_flat(resp_buf, resp_len);
            if (om_resp) {
                ble_gatts_notify_custom(s_conn_handle, s_resp_attr_handle, om_resp);
            }
        }

        gostt_led_set(GOSTT_LED_PAIRED);
        ESP_LOGI(TAG, "Pairing complete");
        return 0;
    }

    // Normal operation: decode DataPacket, decrypt, dispatch
    if (!s_config.crypto->has_key) {
        ESP_LOGW(TAG, "No key — ignoring encrypted packet");
        return 0;
    }

    gostt_data_packet_t pkt;
    if (gostt_decode_data_packet(buf, len, &pkt) != 0) {
        ESP_LOGW(TAG, "Failed to decode DataPacket");
        gostt_led_flash_error();
        return 0;
    }

    // Decrypt
    uint8_t plaintext[512];
    int pt_len = gostt_crypto_decrypt(s_config.crypto,
                                       pkt.iv, pkt.tag,
                                       pkt.encrypted_data, pkt.encrypted_data_len,
                                       plaintext);
    if (pt_len < 0) {
        ESP_LOGW(TAG, "Decrypt failed for packet %u", pkt.packet_num);
        gostt_led_flash_error();
        return 0;
    }

    // Decode EncryptedData wrapper
    gostt_encrypted_data_t enc_data;
    if (gostt_decode_encrypted_data(plaintext, (size_t)pt_len, &enc_data) != 0) {
        ESP_LOGW(TAG, "Failed to decode EncryptedData");
        gostt_led_flash_error();
        return 0;
    }

    if (enc_data.command_type == 0 && enc_data.keyboard_packet_data != NULL) {
        // Text command
        gostt_keyboard_packet_t kbd;
        if (gostt_decode_keyboard_packet(enc_data.keyboard_packet_data,
                                          enc_data.keyboard_packet_data_len, &kbd) == 0) {
            gostt_led_flash_typing();
            if (s_config.on_text) {
                s_config.on_text(kbd.message, kbd.message_len);
            }
        }
    } else if (enc_data.command_type > 0 && s_config.on_command) {
        s_config.on_command(enc_data.command_type,
                           enc_data.command_data, enc_data.command_data_len);
    }

    return 0;
}

// Response characteristic: supports notifications only (no read/write from client)
// The attribute handle is captured during registration for sending notifications.

// MAC characteristic read handler
static int mac_char_read_cb(uint16_t conn_handle, uint16_t attr_handle,
                             struct ble_gatt_access_ctxt *ctxt, void *arg)
{
    (void)conn_handle; (void)attr_handle; (void)arg;

    uint8_t mac[6];
    ble_hs_id_copy_addr(BLE_ADDR_PUBLIC, mac, NULL);
    os_mbuf_append(ctxt->om, mac, sizeof(mac));
    return 0;
}

// --- GATT Service Definition ---

// UUIDs (128-bit, little-endian byte order for NimBLE)
static const ble_uuid128_t svc_uuid =
    BLE_UUID128_INIT(0x14, 0x12, 0x8a, 0x76, 0x04, 0xd1, 0x6c, 0x4f,
                     0x7e, 0x53, 0xf2, 0xe8, 0x00, 0x00, 0xb1, 0x19);
static const ble_uuid128_t tx_char_uuid =
    BLE_UUID128_INIT(0x07, 0x59, 0x2c, 0xdd, 0x7d, 0xcf, 0x42, 0xbf,
                     0x5a, 0x45, 0x7b, 0x2c, 0x19, 0xe1, 0x56, 0x68);
static const ble_uuid128_t resp_char_uuid =
    BLE_UUID128_INIT(0x08, 0x59, 0x2c, 0xdd, 0x7d, 0xcf, 0x42, 0xbf,
                     0x5a, 0x45, 0x7b, 0x2c, 0x19, 0xe1, 0x56, 0x68);
static const ble_uuid128_t mac_char_uuid =
    BLE_UUID128_INIT(0x14, 0x12, 0x8a, 0x76, 0x04, 0xd1, 0x6c, 0x4f,
                     0x7e, 0x53, 0xf2, 0xe8, 0x02, 0x00, 0xb1, 0x19);

static const struct ble_gatt_svc_def gatt_svr_svcs[] = {
    {
        .type = BLE_GATT_SVC_TYPE_PRIMARY,
        .uuid = &svc_uuid.u,
        .characteristics = (struct ble_gatt_chr_def[]) {
            {
                // TX characteristic (write)
                .uuid = &tx_char_uuid.u,
                .access_cb = tx_char_write_cb,
                .flags = BLE_GATT_CHR_F_WRITE | BLE_GATT_CHR_F_WRITE_NO_RSP,
            },
            {
                // Response characteristic (notify)
                .uuid = &resp_char_uuid.u,
                .access_cb = NULL,
                .val_handle = &s_resp_attr_handle,
                .flags = BLE_GATT_CHR_F_NOTIFY,
            },
            {
                // MAC characteristic (read)
                .uuid = &mac_char_uuid.u,
                .access_cb = mac_char_read_cb,
                .flags = BLE_GATT_CHR_F_READ,
            },
            {0}, // sentinel
        },
    },
    {0}, // sentinel
};

// --- Keepalive Timer ---

static void keepalive_timer_cb(TimerHandle_t timer)
{
    (void)timer;
    if (s_conn_handle == BLE_HS_CONN_HANDLE_NONE) return;

    uint8_t buf[16];
    int len = gostt_encode_response_packet(buf, sizeof(buf),
                                            GOSTT_RESP_KEEPALIVE,
                                            GOSTT_PEER_UNKNOWN,
                                            NULL, 0);
    if (len > 0) {
        struct os_mbuf *om = ble_hs_mbuf_from_flat(buf, len);
        if (om) {
            ble_gatts_notify_custom(s_conn_handle, s_resp_attr_handle, om);
        }
    }
}

// --- GAP Event Handler ---

static void start_advertising(void)
{
    struct ble_gap_adv_params adv_params = {0};
    adv_params.conn_mode = BLE_GAP_CONN_MODE_UND;
    adv_params.disc_mode = BLE_GAP_DISC_MODE_GEN;

    struct ble_hs_adv_fields fields = {0};
    fields.flags = BLE_HS_ADV_F_DISC_GEN | BLE_HS_ADV_F_BREDR_UNSUP;
    fields.name = (uint8_t *)GOSTT_BLE_DEVICE_NAME;
    fields.name_len = strlen(GOSTT_BLE_DEVICE_NAME);
    fields.name_is_complete = 1;

    ble_gap_adv_set_fields(&fields);

    // Advertise service UUID in scan response
    struct ble_hs_adv_fields rsp_fields = {0};
    rsp_fields.uuids128 = (ble_uuid128_t[]){{
        BLE_UUID128_INIT(0x14, 0x12, 0x8a, 0x76, 0x04, 0xd1, 0x6c, 0x4f,
                         0x7e, 0x53, 0xf2, 0xe8, 0x00, 0x00, 0xb1, 0x19)
    }};
    rsp_fields.num_uuids128 = 1;
    rsp_fields.uuids128_is_complete = 1;
    ble_gap_adv_rsp_set_fields(&rsp_fields);

    ble_gap_adv_start(BLE_OWN_ADDR_PUBLIC, NULL, BLE_HS_FOREVER,
                      &adv_params, ble_gap_event, NULL);

    ESP_LOGI(TAG, "Advertising started as '%s'", GOSTT_BLE_DEVICE_NAME);
    gostt_led_set(GOSTT_LED_ADVERTISING);
}

static int ble_gap_event(struct ble_gap_event *event, void *arg)
{
    (void)arg;

    switch (event->type) {
        case BLE_GAP_EVENT_CONNECT:
            if (event->connect.status == 0) {
                s_conn_handle = event->connect.conn_handle;
                s_connected = true;
                ESP_LOGI(TAG, "Client connected (handle=%d)", s_conn_handle);

                if (s_config.crypto->has_key) {
                    gostt_led_set(GOSTT_LED_PAIRED);
                } else {
                    gostt_led_set(GOSTT_LED_CONNECTED);
                }

                // Start keepalive timer
                if (s_keepalive_timer) {
                    xTimerStart(s_keepalive_timer, 0);
                }
            } else {
                start_advertising();
            }
            break;

        case BLE_GAP_EVENT_DISCONNECT:
            ESP_LOGI(TAG, "Client disconnected (reason=%d)", event->disconnect.reason);
            s_conn_handle = BLE_HS_CONN_HANDLE_NONE;
            s_connected = false;

            if (s_keepalive_timer) {
                xTimerStop(s_keepalive_timer, 0);
            }

            start_advertising();
            break;

        case BLE_GAP_EVENT_MTU:
            ESP_LOGI(TAG, "MTU updated: %d", event->mtu.value);
            break;

        default:
            break;
    }
    return 0;
}

// --- NimBLE Host Task ---

static void ble_host_task(void *param)
{
    (void)param;
    nimble_port_run();
    nimble_port_freertos_deinit();
}

static void on_sync(void)
{
    ble_hs_util_ensure_addr(0);
    start_advertising();
}

// --- Public API ---

int gostt_ble_server_init(const gostt_ble_config_t *config)
{
    s_config = *config;

    // Initialize NimBLE
    esp_err_t ret = nimble_port_init();
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "NimBLE init failed: %s", esp_err_to_name(ret));
        return -1;
    }

    // Register GATT services
    ble_svc_gap_init();
    ble_svc_gatt_init();

    int rc = ble_gatts_count_cfg(gatt_svr_svcs);
    if (rc != 0) {
        ESP_LOGE(TAG, "GATT count cfg failed: %d", rc);
        return -1;
    }
    rc = ble_gatts_add_svcs(gatt_svr_svcs);
    if (rc != 0) {
        ESP_LOGE(TAG, "GATT add svcs failed: %d", rc);
        return -1;
    }

    ble_svc_gap_device_name_set(GOSTT_BLE_DEVICE_NAME);

    ble_hs_cfg.sync_cb = on_sync;

    // Create keepalive timer
    s_keepalive_timer = xTimerCreate("keepalive",
                                      pdMS_TO_TICKS(GOSTT_KEEPALIVE_INTERVAL_MS),
                                      pdTRUE, NULL, keepalive_timer_cb);

    // Start NimBLE host task
    nimble_port_freertos_init(ble_host_task);

    ESP_LOGI(TAG, "BLE server initialized");
    return 0;
}

bool gostt_ble_is_connected(void)
{
    return s_connected;
}
