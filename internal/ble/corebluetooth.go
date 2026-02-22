package ble

import (
	"context"
	"fmt"
	"sync"

	"tinygo.org/x/bluetooth"
)

// CoreBluetoothAdapter wraps tinygo-org/bluetooth for macOS.
// On macOS, BLE device addresses are CoreBluetooth UUIDs (not MAC addresses).
// The "MAC" field in config and Device structs stores this UUID string.
type CoreBluetoothAdapter struct {
	adapter *bluetooth.Adapter

	// mu protects the connections map.
	mu          sync.Mutex
	connections map[string]*coreBluetoothConnection // keyed by device UUID
}

// NewCoreBluetoothAdapter creates a new BLE adapter using CoreBluetooth.
func NewCoreBluetoothAdapter() *CoreBluetoothAdapter {
	return &CoreBluetoothAdapter{
		adapter:     bluetooth.DefaultAdapter,
		connections: make(map[string]*coreBluetoothConnection),
	}
}

func (a *CoreBluetoothAdapter) Enable() error {
	if err := a.adapter.Enable(); err != nil {
		return err
	}

	// Register the adapter-level connect/disconnect handler.
	// On macOS, tinygo/bluetooth fires this callback (with connected=false)
	// when a peripheral disconnects, via DidDisconnectPeripheral.
	a.adapter.SetConnectHandler(func(device bluetooth.Device, connected bool) {
		if connected {
			return
		}
		id := device.Address.UUID.String()
		a.mu.Lock()
		conn, ok := a.connections[id]
		if ok {
			delete(a.connections, id) // remove stale entry
		}
		a.mu.Unlock()
		if ok {
			conn.fireDisconnect()
		}
	})

	return nil
}

func (a *CoreBluetoothAdapter) Scan(ctx context.Context, serviceUUID string) ([]Device, error) {
	uuid, err := bluetooth.ParseUUID(serviceUUID)
	if err != nil {
		return nil, fmt.Errorf("ble: parse service UUID: %w", err)
	}

	var mu sync.Mutex
	var devices []Device
	seen := make(map[string]bool)

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			a.adapter.StopScan()
		case <-done:
		}
	}()

	err = a.adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		if !result.HasServiceUUID(uuid) {
			return
		}
		mac := result.Address.String()
		mu.Lock()
		defer mu.Unlock()
		if seen[mac] {
			return
		}
		seen[mac] = true
		devices = append(devices, Device{
			Name: result.LocalName(),
			MAC:  mac,
			RSSI: int(result.RSSI),
		})
	})
	close(done)

	if err != nil && ctx.Err() == nil {
		return nil, fmt.Errorf("ble: scan: %w", err)
	}
	return devices, nil
}

func (a *CoreBluetoothAdapter) Connect(ctx context.Context, mac string) (Connection, error) {
	// On macOS, bluetooth.Address wraps a UUID, not a MAC.
	// Address.Set() parses the UUID string.
	var addr bluetooth.Address
	addr.Set(mac)

	// tinygo/bluetooth's Connect blocks internally with its own timeout.
	// We wrap it to also respect our ctx cancellation.
	type connectResult struct {
		device bluetooth.Device
		err    error
	}
	ch := make(chan connectResult)
	go func() {
		device, err := a.adapter.Connect(addr, bluetooth.ConnectionParams{})
		select {
		case ch <- connectResult{device, err}:
			// caller received the result
		default:
			// ctx was cancelled; caller already returned.
			// Clean up the connection we just established.
			if err == nil {
				device.Disconnect()
			}
		}
	}()

	select {
	case <-ctx.Done():
		// Context cancelled. The underlying Connect will eventually time out
		// or succeed. The goroutine cleans up any connection it establishes.
		return nil, fmt.Errorf("ble: connect to %s: %w", mac, ctx.Err())
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("ble: connect to %s: %w", mac, result.err)
		}
		conn := &coreBluetoothConnection{device: &result.device}

		// Track this connection so the adapter-level disconnect handler
		// can find it and fire its OnDisconnect callback.
		// Use the canonical UUID string from the device for key consistency
		// with the disconnect handler's device.Address.UUID.String().
		id := result.device.Address.UUID.String()
		a.mu.Lock()
		a.connections[id] = conn
		a.mu.Unlock()

		return conn, nil
	}
}

// Compile-time check that CoreBluetoothAdapter implements Adapter.
var _ Adapter = (*CoreBluetoothAdapter)(nil)

type coreBluetoothConnection struct {
	device *bluetooth.Device

	mu           sync.Mutex
	disconnectCb func()
}

// fireDisconnect invokes the disconnect callback if set. Thread-safe.
func (c *coreBluetoothConnection) fireDisconnect() {
	c.mu.Lock()
	cb := c.disconnectCb
	c.mu.Unlock()
	if cb != nil {
		cb()
	}
}

func (c *coreBluetoothConnection) DiscoverCharacteristic(serviceUUID, charUUID string) (Characteristic, error) {
	svcUUID, err := bluetooth.ParseUUID(serviceUUID)
	if err != nil {
		return nil, err
	}
	charUUIDParsed, err := bluetooth.ParseUUID(charUUID)
	if err != nil {
		return nil, err
	}

	svcs, err := c.device.DiscoverServices([]bluetooth.UUID{svcUUID})
	if err != nil {
		return nil, fmt.Errorf("ble: discover services: %w", err)
	}
	if len(svcs) == 0 {
		return nil, fmt.Errorf("ble: service %s not found", serviceUUID)
	}

	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{charUUIDParsed})
	if err != nil {
		return nil, fmt.Errorf("ble: discover characteristics: %w", err)
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("ble: characteristic %s not found", charUUID)
	}

	return &coreBluetoothCharacteristic{char: &chars[0]}, nil
}

func (c *coreBluetoothConnection) Disconnect() error {
	return c.device.Disconnect()
}

func (c *coreBluetoothConnection) OnDisconnect(cb func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disconnectCb = cb
}

type coreBluetoothCharacteristic struct {
	char *bluetooth.DeviceCharacteristic
}

func (c *coreBluetoothCharacteristic) Write(data []byte) error {
	_, err := c.char.WriteWithoutResponse(data)
	return err
}

func (c *coreBluetoothCharacteristic) Subscribe(cb func([]byte)) error {
	return c.char.EnableNotifications(func(buf []byte) {
		cb(buf)
	})
}
