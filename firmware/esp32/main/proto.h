// firmware/esp32/main/proto.h
#ifndef GOSTT_KBD_PROTO_H
#define GOSTT_KBD_PROTO_H

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>

// DataPacket (received from app, outer envelope)
typedef struct {
    uint8_t  iv[12];
    uint8_t  tag[16];
    uint8_t *encrypted_data;
    size_t   encrypted_data_len;
    uint32_t packet_num;
} gostt_data_packet_t;

// KeyboardPacket (inner, after decryption)
typedef struct {
    char    *message;
    size_t   message_len;
    uint32_t length;       // redundant length field from protobuf
} gostt_keyboard_packet_t;

// EncryptedData (inner wrapper)
// command_type: 0=text (keyboard_packet present), 1=mute_toggle, 2=configure_mute
typedef struct {
    uint8_t *keyboard_packet_data;
    size_t   keyboard_packet_data_len;
    uint32_t command_type;
    uint8_t *command_data;
    size_t   command_data_len;
} gostt_encrypted_data_t;

// ResponsePacket types
typedef enum {
    GOSTT_RESP_KEEPALIVE   = 0,
    GOSTT_RESP_PEER_STATUS = 1,
} gostt_response_type_t;

typedef enum {
    GOSTT_PEER_UNKNOWN = 0,
    GOSTT_PEER_KNOWN   = 1,
} gostt_peer_status_t;

// Decode a DataPacket from raw protobuf bytes.
// encrypted_data pointer is into the input buffer (not copied) — caller must not free input while using result.
// Returns 0 on success, -1 on error.
int gostt_decode_data_packet(const uint8_t *buf, size_t len, gostt_data_packet_t *out);

// Decode a KeyboardPacket from raw protobuf bytes.
// message pointer is into the input buffer — caller must not free input while using result.
// Returns 0 on success, -1 on error.
int gostt_decode_keyboard_packet(const uint8_t *buf, size_t len, gostt_keyboard_packet_t *out);

// Decode an EncryptedData wrapper from raw protobuf bytes.
// Pointers are into the input buffer.
// Returns 0 on success, -1 on error.
int gostt_decode_encrypted_data(const uint8_t *buf, size_t len, gostt_encrypted_data_t *out);

// Encode a ResponsePacket into buf. Returns number of bytes written, or -1 on error.
// buf must be at least 64 + data_len bytes.
int gostt_encode_response_packet(uint8_t *buf, size_t buf_len,
                                  gostt_response_type_t type,
                                  gostt_peer_status_t peer_status,
                                  const uint8_t *data, size_t data_len);

#endif // GOSTT_KBD_PROTO_H
