package ble

import (
	"context"
	"sync"
	"testing"
	"time"

	blecrypto "github.com/chaz8081/gostt-writer/internal/ble/crypto"
)

func TestScanForDevices(t *testing.T) {
	devices := []Device{
		{Name: "ToothPaste-S3", MAC: "AA:BB:CC:DD:EE:FF", RSSI: -45},
	}
	adapter := newMockAdapter(devices)

	result, err := ScanForDevices(adapter, 5*time.Second)
	if err != nil {
		t.Fatalf("ScanForDevices() error = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d devices, want 1", len(result))
	}
	if result[0].Name != "ToothPaste-S3" {
		t.Errorf("Name = %q, want %q", result[0].Name, "ToothPaste-S3")
	}
	if result[0].MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("MAC = %q, want %q", result[0].MAC, "AA:BB:CC:DD:EE:FF")
	}
}

func TestScanForDevicesEmpty(t *testing.T) {
	adapter := newMockAdapter(nil)
	result, err := ScanForDevices(adapter, 5*time.Second)
	if err != nil {
		t.Fatalf("ScanForDevices() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d devices, want 0", len(result))
	}
}

func TestPairExchangeKeys(t *testing.T) {
	adapter := newMockPairingAdapter()

	result, err := Pair(adapter, "AA:BB:CC:DD:EE:FF", PairOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Pair() error = %v", err)
	}
	if result.DeviceMAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("DeviceMAC = %q, want %q", result.DeviceMAC, "AA:BB:CC:DD:EE:FF")
	}
	if len(result.SharedSecret) != 32 {
		t.Errorf("SharedSecret length = %d, want 32", len(result.SharedSecret))
	}
}

func TestPairTimeout(t *testing.T) {
	// Use regular mock adapter that doesn't respond with a public key
	adapter := newMockAdapter(nil)

	// This should timeout since the mock won't send a peer public key
	_, err := Pair(adapter, "AA:BB:CC:DD:EE:FF", PairOptions{Timeout: 100 * time.Millisecond})
	if err == nil {
		t.Fatal("Pair() should have timed out")
	}
}

// mockPairingAdapter is a mock BLE adapter that simulates the ECDH pairing flow.
// When a 33-byte compressed public key is written to the TX characteristic,
// it generates its own keypair and sends back a ResponsePacket with its
// compressed public key on the response characteristic.
type mockPairingAdapter struct {
	mu         sync.Mutex
	connection *mockPairingConnection
}

func newMockPairingAdapter() *mockPairingAdapter {
	return &mockPairingAdapter{}
}

func (a *mockPairingAdapter) Enable() error { return nil }

func (a *mockPairingAdapter) Scan(_ context.Context, _ string) ([]Device, error) {
	return nil, nil
}

func (a *mockPairingAdapter) Connect(_ context.Context, _ string) (Connection, error) {
	conn := newMockPairingConnection()
	a.mu.Lock()
	a.connection = conn
	a.mu.Unlock()
	return conn, nil
}

// mockPairingConnection wraps mockConnection to add a write hook on the TX
// characteristic that simulates the ESP32 side of the ECDH key exchange.
type mockPairingConnection struct {
	base     *mockConnection
	txChar   *mockPairingCharacteristic
	respChar *mockCharacteristic
}

func newMockPairingConnection() *mockPairingConnection {
	base := newMockConnection()
	pc := &mockPairingConnection{
		base:     base,
		respChar: base.respChar,
	}
	pc.txChar = &mockPairingCharacteristic{
		inner:    base.txChar,
		respChar: base.respChar,
	}
	return pc
}

func (c *mockPairingConnection) DiscoverCharacteristic(serviceUUID, charUUID string) (Characteristic, error) {
	switch charUUID {
	case TXCharUUID:
		return c.txChar, nil
	case ResponseCharUUID:
		return c.respChar, nil
	default:
		return c.base.DiscoverCharacteristic(serviceUUID, charUUID)
	}
}

func (c *mockPairingConnection) Disconnect() error {
	return c.base.Disconnect()
}

func (c *mockPairingConnection) OnDisconnect(cb func()) {
	c.base.OnDisconnect(cb)
}

// mockPairingCharacteristic wraps mockCharacteristic and intercepts Write calls.
// When a 33-byte compressed public key is written, it generates the ESP32 side
// of the ECDH exchange and sends back a ResponsePacket notification.
type mockPairingCharacteristic struct {
	inner    *mockCharacteristic
	respChar *mockCharacteristic
}

func (c *mockPairingCharacteristic) Write(data []byte) error {
	if err := c.inner.Write(data); err != nil {
		return err
	}

	// Detect 33-byte compressed public key write (pairing initiation)
	if len(data) == 33 && (data[0] == 0x02 || data[0] == 0x03) {
		go c.simulatePeerKeyExchange(data)
	}

	return nil
}

func (c *mockPairingCharacteristic) Subscribe(cb func([]byte)) error {
	return c.inner.Subscribe(cb)
}

// simulatePeerKeyExchange generates the ESP32's ECDH keypair and sends
// back a ResponsePacket with the compressed public key.
func (c *mockPairingCharacteristic) simulatePeerKeyExchange(_ []byte) {
	// Generate ESP32's keypair
	_, peerPub, err := blecrypto.GenerateKeyPair()
	if err != nil {
		return
	}
	compressed := blecrypto.CompressPublicKey(peerPub)

	// Build protobuf ResponsePacket manually:
	// field 1 (type): tag=0x08, varint=1 (PeerStatus)
	// field 2 (peer_status): tag=0x10, varint=0 (Unknown)
	// field 3 (data): tag=0x1a, length=0x21 (33), then 33 bytes
	var buf []byte
	buf = append(buf, 0x08, 0x01) // type = PeerStatus (1)
	buf = append(buf, 0x10, 0x00) // peer_status = Unknown (0)
	buf = append(buf, 0x1a, 0x21) // data field, length 33
	buf = append(buf, compressed...)

	// Small delay to simulate BLE latency and ensure subscriber is registered
	time.Sleep(10 * time.Millisecond)

	c.respChar.SimulateNotification(buf)
}
