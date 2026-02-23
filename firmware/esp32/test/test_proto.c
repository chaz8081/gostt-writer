// firmware/esp32/test/test_proto.c
// Host-compilable test (not ESP-IDF) — validates protobuf wire compatibility with Go app.
// Compile: gcc -I../main -o test_proto test_proto.c ../main/proto.c
#include <stdio.h>
#include <string.h>
#include <assert.h>
#include "proto.h"

static int tests_run = 0;
static int tests_passed = 0;

#define TEST(name) do { printf("  %-50s ", #name); tests_run++; } while(0)
#define PASS() do { printf("PASS\n"); tests_passed++; } while(0)
#define FAIL(msg) do { printf("FAIL: %s\n", msg); } while(0)

// Golden bytes from Go proto_test.go
void test_decode_keyboard_packet_hello(void)
{
    TEST(decode_keyboard_packet_hello);
    // MarshalKeyboardPacket("hello") from Go:
    uint8_t data[] = {0x0a, 0x05, 'h', 'e', 'l', 'l', 'o', 0x10, 0x05};
    gostt_keyboard_packet_t pkt;
    int rc = gostt_decode_keyboard_packet(data, sizeof(data), &pkt);
    assert(rc == 0);
    assert(pkt.message_len == 5);
    assert(memcmp(pkt.message, "hello", 5) == 0);
    assert(pkt.length == 5);
    PASS();
}

void test_decode_keyboard_packet_empty(void)
{
    TEST(decode_keyboard_packet_empty);
    // MarshalKeyboardPacket("") from Go:
    uint8_t data[] = {0x0a, 0x00, 0x10, 0x00};
    gostt_keyboard_packet_t pkt;
    int rc = gostt_decode_keyboard_packet(data, sizeof(data), &pkt);
    assert(rc == 0);
    assert(pkt.message_len == 0);
    assert(pkt.length == 0);
    PASS();
}

void test_encode_response_packet(void)
{
    TEST(encode_response_packet);
    // Expected from Go test: type=1, status=0, data=[0xDE, 0xAD]
    uint8_t expected[] = {0x08, 0x01, 0x10, 0x00, 0x1a, 0x02, 0xDE, 0xAD};
    uint8_t peer_data[] = {0xDE, 0xAD};

    uint8_t buf[64];
    int len = gostt_encode_response_packet(buf, sizeof(buf),
                                            GOSTT_RESP_PEER_STATUS,
                                            GOSTT_PEER_UNKNOWN,
                                            peer_data, sizeof(peer_data));
    assert(len == (int)sizeof(expected));
    assert(memcmp(buf, expected, len) == 0);
    PASS();
}

void test_decode_data_packet(void)
{
    TEST(decode_data_packet);
    // Build DataPacket golden bytes matching Go test:
    // iv: 0xAA followed by 11 zeros (12 bytes)
    // tag: 0xBB followed by 15 zeros (16 bytes)
    // encrypted: [0x01, 0x02, 0x03]
    // packet_num: 42 (varint 0x2a)
    uint8_t data[] = {
        0x0a, 0x0c, // field 1: iv (12 bytes)
        0xAA, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x12, 0x10, // field 2: tag (16 bytes)
        0xBB, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x1a, 0x03, // field 3: encrypted (3 bytes)
        0x01, 0x02, 0x03,
        0x20, 0x2a, // field 4: packet_num = 42
    };

    gostt_data_packet_t pkt;
    int rc = gostt_decode_data_packet(data, sizeof(data), &pkt);
    assert(rc == 0);
    assert(pkt.iv[0] == 0xAA);
    assert(pkt.iv[1] == 0x00);
    assert(pkt.tag[0] == 0xBB);
    assert(pkt.encrypted_data_len == 3);
    assert(pkt.encrypted_data[0] == 0x01);
    assert(pkt.encrypted_data[1] == 0x02);
    assert(pkt.encrypted_data[2] == 0x03);
    assert(pkt.packet_num == 42);
    PASS();
}

void test_decode_encrypted_data(void)
{
    TEST(decode_encrypted_data);
    // EncryptedData wrapping a KeyboardPacket("hello"):
    // inner = MarshalKeyboardPacket("hello") = [0x0a, 0x05, 'h','e','l','l','o', 0x10, 0x05]
    // MarshalEncryptedData(inner) = [0x0a, 0x09, ...inner...]
    uint8_t data[] = {
        0x0a, 0x09,
        0x0a, 0x05, 'h', 'e', 'l', 'l', 'o', 0x10, 0x05
    };

    gostt_encrypted_data_t enc;
    int rc = gostt_decode_encrypted_data(data, sizeof(data), &enc);
    assert(rc == 0);
    assert(enc.keyboard_packet_data != NULL);
    assert(enc.keyboard_packet_data_len == 9);
    assert(enc.command_type == 0);
    PASS();
}

int main(void)
{
    printf("GOSTT-KBD Protocol Cross-Validation Tests\n");
    printf("==========================================\n");

    test_decode_keyboard_packet_hello();
    test_decode_keyboard_packet_empty();
    test_encode_response_packet();
    test_decode_data_packet();
    test_decode_encrypted_data();

    printf("\n%d/%d tests passed\n", tests_passed, tests_run);
    return (tests_passed == tests_run) ? 0 : 1;
}
