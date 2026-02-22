package ble

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// mockCharacteristic records writes and allows subscribing.
type mockCharacteristic struct {
	mu       sync.Mutex
	writes   [][]byte
	callback func([]byte)
}

func (c *mockCharacteristic) Write(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	c.writes = append(c.writes, cp)
	return nil
}

func (c *mockCharacteristic) Subscribe(cb func([]byte)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callback = cb
	return nil
}

// SimulateNotification sends a notification to the subscriber.
func (c *mockCharacteristic) SimulateNotification(data []byte) {
	c.mu.Lock()
	cb := c.callback
	c.mu.Unlock()
	if cb != nil {
		cb(data)
	}
}

// mockConnection simulates a BLE connection.
type mockConnection struct {
	mu           sync.Mutex
	txChar       *mockCharacteristic
	respChar     *mockCharacteristic
	disconnectCb func()
	disconnected bool
}

func newMockConnection() *mockConnection {
	return &mockConnection{
		txChar:   &mockCharacteristic{},
		respChar: &mockCharacteristic{},
	}
}

func (c *mockConnection) DiscoverCharacteristic(serviceUUID, charUUID string) (Characteristic, error) {
	switch charUUID {
	case TXCharUUID:
		return c.txChar, nil
	case ResponseCharUUID:
		return c.respChar, nil
	default:
		return nil, fmt.Errorf("mock: unknown characteristic UUID %q", charUUID)
	}
}

func (c *mockConnection) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disconnected = true
	return nil
}

func (c *mockConnection) OnDisconnect(cb func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disconnectCb = cb
}

// SimulateDisconnect triggers the disconnect callback.
func (c *mockConnection) SimulateDisconnect() {
	c.mu.Lock()
	cb := c.disconnectCb
	c.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// mockAdapter simulates the BLE adapter.
type mockAdapter struct {
	mu         sync.Mutex
	devices    []Device
	connection *mockConnection // most recent connection for test assertions
}

func newMockAdapter(devices []Device) *mockAdapter {
	return &mockAdapter{
		devices:    devices,
		connection: newMockConnection(),
	}
}

func (a *mockAdapter) Enable() error { return nil }

func (a *mockAdapter) Scan(_ context.Context, _ string) ([]Device, error) {
	return a.devices, nil
}

func (a *mockAdapter) Connect(_ context.Context, _ string) (Connection, error) {
	conn := newMockConnection()
	a.mu.Lock()
	a.connection = conn
	a.mu.Unlock()
	return conn, nil
}

// latestConnection returns the most recently created connection (thread-safe).
func (a *mockAdapter) latestConnection() *mockConnection {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connection
}

func TestMockAdapterImplementsInterface(t *testing.T) {
	var _ Adapter = (*mockAdapter)(nil)
}

func TestMockConnectionImplementsInterface(t *testing.T) {
	var _ Connection = (*mockConnection)(nil)
}

func TestMockCharacteristicImplementsInterface(t *testing.T) {
	var _ Characteristic = (*mockCharacteristic)(nil)
}
