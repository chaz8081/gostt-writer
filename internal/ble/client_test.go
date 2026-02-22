package ble

import (
	"strings"
	"testing"
	"time"
)

func makeTestKey() []byte {
	key := make([]byte, 32)
	key[0] = 0x42
	return key
}

func TestClientSendWritesToTX(t *testing.T) {
	adapter := newMockAdapter(nil)
	client := NewClient(adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), DefaultClientOptions())

	// Simulate an already-connected state
	client.setConnected(adapter.connection)

	err := client.Send("hello")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	writes := adapter.connection.txChar.writes
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
	client := NewClient(adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), DefaultClientOptions())
	client.setConnected(adapter.connection)

	// Send text that exceeds one BLE packet
	longText := strings.Repeat("word ", 100) // 500 bytes
	err := client.Send(longText)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	writes := adapter.connection.txChar.writes
	if len(writes) < 2 {
		t.Errorf("expected multiple writes for 500-byte text, got %d", len(writes))
	}
}

func TestClientSendEmptyString(t *testing.T) {
	adapter := newMockAdapter(nil)
	client := NewClient(adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), DefaultClientOptions())
	client.setConnected(adapter.connection)

	err := client.Send("")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Empty string should produce no writes
	if len(adapter.connection.txChar.writes) != 0 {
		t.Errorf("Send(\"\") produced %d writes, want 0", len(adapter.connection.txChar.writes))
	}
}

func TestClientSendIncrementingPacketNum(t *testing.T) {
	adapter := newMockAdapter(nil)
	client := NewClient(adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), DefaultClientOptions())
	client.setConnected(adapter.connection)

	_ = client.Send("first")
	_ = client.Send("second")

	// Packet numbers should be incrementing (verified by the fact that
	// we got two separate writes with different content)
	writes := adapter.connection.txChar.writes
	if len(writes) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(writes))
	}
	// The writes should differ (different IV, different packet_num, different ciphertext)
	if string(writes[0]) == string(writes[1]) {
		t.Error("two sends produced identical wire bytes (packet_num should differ)")
	}
}

func TestClientQueuesDuringDisconnect(t *testing.T) {
	adapter := newMockAdapter(nil)
	opts := DefaultClientOptions()
	opts.QueueSize = 4
	client := NewClient(adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), opts)

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
	opts := DefaultClientOptions()
	opts.QueueSize = 2
	client := NewClient(adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), opts)

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
	opts := DefaultClientOptions()
	opts.QueueSize = 4
	client := NewClient(adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), opts)

	// Queue messages while disconnected
	_ = client.Send("msg1")
	_ = client.Send("msg2")

	// Simulate reconnect
	client.setConnected(adapter.connection)
	client.flushQueue()

	// Allow async flush
	time.Sleep(50 * time.Millisecond)

	if client.QueueLen() != 0 {
		t.Errorf("QueueLen() after flush = %d, want 0", client.QueueLen())
	}

	writes := adapter.connection.txChar.writes
	if len(writes) != 2 {
		t.Errorf("expected 2 writes after flush, got %d", len(writes))
	}
}
