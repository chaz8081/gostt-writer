package ble

import (
	"encoding/binary"
	"strings"
	"testing"
)

func makeTestKey() []byte {
	key := make([]byte, 32)
	key[0] = 0x42
	return key
}

func mustNewClient(t *testing.T, adapter *mockAdapter, mac string, key []byte, opts ClientOptions) *Client {
	t.Helper()
	client, err := NewClient(adapter, mac, key, opts)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

// zeroDelayOpts returns options with no inter-chunk delay for fast tests.
func zeroDelayOpts() ClientOptions {
	opts := DefaultClientOptions()
	opts.InterChunkDelay = 0
	return opts
}

func TestClientSendWritesToTX(t *testing.T) {
	adapter := newMockAdapter(nil)
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), zeroDelayOpts())

	// Simulate an already-connected state
	conn := adapter.latestConnection()
	if err := client.setConnected(conn); err != nil {
		t.Fatalf("setConnected() error = %v", err)
	}

	err := client.Send("hello")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	writes := conn.txChar.writes
	if len(writes) == 0 {
		t.Fatal("Send() produced no writes to TX characteristic")
	}
	// Each write should be a valid DataPacket (starts with 0x0a for field 1 = iv)
	if writes[0][0] != 0x0a {
		t.Errorf("first write byte = 0x%02x, want 0x0a (DataPacket field 1)", writes[0][0])
	}
}

func TestClientSendChunksLongText(t *testing.T) {
	adapter := newMockAdapter(nil)
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), zeroDelayOpts())
	conn := adapter.latestConnection()
	if err := client.setConnected(conn); err != nil {
		t.Fatalf("setConnected() error = %v", err)
	}

	// Send text that exceeds one BLE packet
	longText := strings.Repeat("word ", 100) // 500 bytes
	err := client.Send(longText)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	writes := conn.txChar.writes
	if len(writes) < 2 {
		t.Errorf("expected multiple writes for 500-byte text, got %d", len(writes))
	}
}

func TestClientSendEmptyString(t *testing.T) {
	adapter := newMockAdapter(nil)
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), zeroDelayOpts())
	conn := adapter.latestConnection()
	if err := client.setConnected(conn); err != nil {
		t.Fatalf("setConnected() error = %v", err)
	}

	err := client.Send("")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Empty string should produce no writes
	if len(conn.txChar.writes) != 0 {
		t.Errorf("Send(\"\") produced %d writes, want 0", len(conn.txChar.writes))
	}
}

func TestClientSendIncrementingPacketNum(t *testing.T) {
	adapter := newMockAdapter(nil)
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), zeroDelayOpts())
	conn := adapter.latestConnection()
	if err := client.setConnected(conn); err != nil {
		t.Fatalf("setConnected() error = %v", err)
	}

	_ = client.Send("first")
	_ = client.Send("second")

	writes := conn.txChar.writes
	if len(writes) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(writes))
	}

	// Extract packet_num (protobuf field 4, varint) from each DataPacket.
	// Field 4 tag = (4 << 3) | 0 = 0x20
	pktNum1 := extractPacketNum(t, writes[0])
	pktNum2 := extractPacketNum(t, writes[1])

	if pktNum1 != 1 {
		t.Errorf("first packet_num = %d, want 1", pktNum1)
	}
	if pktNum2 != 2 {
		t.Errorf("second packet_num = %d, want 2", pktNum2)
	}
}

// extractPacketNum parses a DataPacket protobuf and extracts field 4 (packet_num).
// DataPacket layout: field 1 (bytes, iv), field 2 (bytes, tag), field 3 (bytes, encrypted), field 4 (varint, packet_num)
func extractPacketNum(t *testing.T, data []byte) uint32 {
	t.Helper()
	pos := 0
	for pos < len(data) {
		if pos >= len(data) {
			break
		}
		tagByte := data[pos]
		fieldNum := tagByte >> 3
		wireType := tagByte & 0x07
		pos++

		switch wireType {
		case 0: // varint
			val, n := binary.Uvarint(data[pos:])
			if n <= 0 {
				t.Fatalf("failed to read varint at offset %d", pos)
			}
			pos += n
			if fieldNum == 4 {
				return uint32(val)
			}
		case 2: // length-delimited
			length, n := binary.Uvarint(data[pos:])
			if n <= 0 {
				t.Fatalf("failed to read length varint at offset %d", pos)
			}
			pos += n + int(length)
		default:
			t.Fatalf("unexpected wire type %d at offset %d", wireType, pos-1)
		}
	}
	t.Fatal("packet_num field (field 4) not found in DataPacket")
	return 0
}

func TestClientQueuesDuringDisconnect(t *testing.T) {
	adapter := newMockAdapter(nil)
	opts := zeroDelayOpts()
	opts.QueueSize = 4
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), opts)

	// Client starts disconnected — Send should queue
	err := client.Send("queued message")
	if err != nil {
		t.Fatalf("Send() while disconnected should not error, got: %v", err)
	}

	if client.QueueLen() != 1 {
		t.Errorf("QueueLen() = %d, want 1", client.QueueLen())
	}
}

func TestClientQueueOverflow(t *testing.T) {
	adapter := newMockAdapter(nil)
	opts := zeroDelayOpts()
	opts.QueueSize = 2
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), opts)

	// Fill queue
	_ = client.Send("msg1")
	_ = client.Send("msg2")
	_ = client.Send("msg3") // should drop oldest

	if client.QueueLen() != 2 {
		t.Errorf("QueueLen() = %d, want 2 (overflow should drop oldest)", client.QueueLen())
	}
}

func TestClientFlushQueueOnReconnect(t *testing.T) {
	adapter := newMockAdapter(nil)
	opts := zeroDelayOpts()
	opts.QueueSize = 4
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), opts)

	// Queue messages while disconnected
	_ = client.Send("msg1")
	_ = client.Send("msg2")

	// Simulate reconnect
	conn := adapter.latestConnection()
	if err := client.setConnected(conn); err != nil {
		t.Fatalf("setConnected() error = %v", err)
	}
	client.flushQueue()

	if client.QueueLen() != 0 {
		t.Errorf("QueueLen() after flush = %d, want 0", client.QueueLen())
	}

	writes := conn.txChar.writes
	if len(writes) != 2 {
		t.Errorf("expected 2 writes after flush, got %d", len(writes))
	}
}

func TestNewClientRejectsInvalidKeyLength(t *testing.T) {
	adapter := newMockAdapter(nil)
	_, err := NewClient(adapter, "AA:BB:CC:DD:EE:FF", make([]byte, 16), DefaultClientOptions())
	if err == nil {
		t.Error("NewClient() should reject 16-byte key")
	}
}
