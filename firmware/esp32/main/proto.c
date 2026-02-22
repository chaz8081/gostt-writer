// firmware/esp32/main/proto.c
#include "proto.h"
#include <string.h>

// Read a protobuf varint from buf. Returns bytes consumed, or 0 on error.
static int read_varint(const uint8_t *buf, size_t len, uint64_t *value)
{
    *value = 0;
    int shift = 0;
    for (size_t i = 0; i < len && i < 10; i++) {
        *value |= (uint64_t)(buf[i] & 0x7F) << shift;
        shift += 7;
        if ((buf[i] & 0x80) == 0) {
            return (int)(i + 1);
        }
    }
    return 0; // error: truncated or too long
}

// Write a protobuf varint to buf. Returns bytes written.
static int write_varint(uint8_t *buf, size_t buf_len, uint64_t value)
{
    int n = 0;
    do {
        if ((size_t)n >= buf_len) return -1;
        buf[n] = (uint8_t)(value & 0x7F);
        value >>= 7;
        if (value > 0) buf[n] |= 0x80;
        n++;
    } while (value > 0);
    return n;
}

int gostt_decode_data_packet(const uint8_t *buf, size_t len, gostt_data_packet_t *out)
{
    memset(out, 0, sizeof(*out));
    size_t pos = 0;

    while (pos < len) {
        uint64_t tag_val;
        int n = read_varint(buf + pos, len - pos, &tag_val);
        if (n == 0) return -1;
        pos += n;

        uint32_t field_num = (uint32_t)(tag_val >> 3);
        uint32_t wire_type = (uint32_t)(tag_val & 0x07);

        if (wire_type == 0) { // varint
            uint64_t val;
            n = read_varint(buf + pos, len - pos, &val);
            if (n == 0) return -1;
            pos += n;
            if (field_num == 4) out->packet_num = (uint32_t)val;
        } else if (wire_type == 2) { // length-delimited
            uint64_t field_len;
            n = read_varint(buf + pos, len - pos, &field_len);
            if (n == 0) return -1;
            pos += n;
            if (field_len > len) return -1;
            if (pos + field_len > len) return -1;
            switch (field_num) {
                case 1:
                    if (field_len != 12) return -1;
                    memcpy(out->iv, buf + pos, 12);
                    break;
                case 2:
                    if (field_len != 16) return -1;
                    memcpy(out->tag, buf + pos, 16);
                    break;
                case 3:
                    out->encrypted_data = (uint8_t *)(buf + pos);
                    out->encrypted_data_len = (size_t)field_len;
                    break;
            }
            pos += (size_t)field_len;
        } else {
            return -1; // unsupported wire type
        }
    }
    return 0;
}

int gostt_decode_keyboard_packet(const uint8_t *buf, size_t len, gostt_keyboard_packet_t *out)
{
    memset(out, 0, sizeof(*out));
    size_t pos = 0;

    while (pos < len) {
        uint64_t tag_val;
        int n = read_varint(buf + pos, len - pos, &tag_val);
        if (n == 0) return -1;
        pos += n;

        uint32_t field_num = (uint32_t)(tag_val >> 3);
        uint32_t wire_type = (uint32_t)(tag_val & 0x07);

        if (wire_type == 0) { // varint
            uint64_t val;
            n = read_varint(buf + pos, len - pos, &val);
            if (n == 0) return -1;
            pos += n;
            if (field_num == 2) out->length = (uint32_t)val;
        } else if (wire_type == 2) { // length-delimited
            uint64_t field_len;
            n = read_varint(buf + pos, len - pos, &field_len);
            if (n == 0) return -1;
            pos += n;
            if (field_len > len) return -1;
            if (pos + field_len > len) return -1;
            if (field_num == 1) {
                out->message = (char *)(buf + pos);
                out->message_len = (size_t)field_len;
            }
            pos += (size_t)field_len;
        } else {
            return -1;
        }
    }
    return 0;
}

int gostt_decode_encrypted_data(const uint8_t *buf, size_t len, gostt_encrypted_data_t *out)
{
    memset(out, 0, sizeof(*out));
    size_t pos = 0;

    while (pos < len) {
        uint64_t tag_val;
        int n = read_varint(buf + pos, len - pos, &tag_val);
        if (n == 0) return -1;
        pos += n;

        uint32_t field_num = (uint32_t)(tag_val >> 3);
        uint32_t wire_type = (uint32_t)(tag_val & 0x07);

        if (wire_type == 0) {
            uint64_t val;
            n = read_varint(buf + pos, len - pos, &val);
            if (n == 0) return -1;
            pos += n;
            if (field_num == 2) out->command_type = (uint32_t)val;
        } else if (wire_type == 2) {
            uint64_t field_len;
            n = read_varint(buf + pos, len - pos, &field_len);
            if (n == 0) return -1;
            pos += n;
            if (field_len > len) return -1;
            if (pos + field_len > len) return -1;
            switch (field_num) {
                case 1:
                    out->keyboard_packet_data = (uint8_t *)(buf + pos);
                    out->keyboard_packet_data_len = (size_t)field_len;
                    break;
                case 3:
                    out->command_data = (uint8_t *)(buf + pos);
                    out->command_data_len = (size_t)field_len;
                    break;
            }
            pos += (size_t)field_len;
        } else {
            return -1;
        }
    }
    return 0;
}

int gostt_encode_response_packet(uint8_t *buf, size_t buf_len,
                                  gostt_response_type_t type,
                                  gostt_peer_status_t peer_status,
                                  const uint8_t *data, size_t data_len)
{
    size_t pos = 0;

    // Field 1: type (varint), tag = (1 << 3) | 0 = 0x08
    if (pos >= buf_len) return -1;
    buf[pos++] = 0x08;
    int n = write_varint(buf + pos, buf_len - pos, (uint64_t)type);
    if (n < 0) return -1;
    pos += n;

    // Field 2: peer_status (varint), tag = (2 << 3) | 0 = 0x10
    if (pos >= buf_len) return -1;
    buf[pos++] = 0x10;
    n = write_varint(buf + pos, buf_len - pos, (uint64_t)peer_status);
    if (n < 0) return -1;
    pos += n;

    // Field 3: data (bytes), tag = (3 << 3) | 2 = 0x1a
    if (data != NULL && data_len > 0) {
        if (pos >= buf_len) return -1;
        buf[pos++] = 0x1a;
        n = write_varint(buf + pos, buf_len - pos, (uint64_t)data_len);
        if (n < 0) return -1;
        pos += n;
        if (pos + data_len > buf_len) return -1;
        memcpy(buf + pos, data, data_len);
        pos += data_len;
    }

    return (int)pos;
}
