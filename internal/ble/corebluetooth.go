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
		a.mu.Unlock()
		if ok && conn.disconnectCb != nil {
			conn.disconnectCb()
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
	ch := make(chan connectResult, 1)
	go func() {
		device, err := a.adapter.Connect(addr, bluetooth.ConnectionParams{})
		ch <- connectResult{device, err}
	}()

	select {
	case <-ctx.Done():
		// Context cancelled. The underlying Connect will eventually time out
		// or succeed. We can't cancel it from here, but we return immediately.
		return nil, fmt.Errorf("ble: connect to %s: %w", mac, ctx.Err())
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("ble: connect to %s: %w", mac, result.err)
		}
		conn := &coreBluetoothConnection{device: &result.device}

		// Track this connection so the adapter-level disconnect handler
		// can find it and fire its OnDisconnect callback.
		a.mu.Lock()
		a.connections[mac] = conn
		a.mu.Unlock()

		return conn, nil
	}
}

// Compile-time check that CoreBluetoothAdapter implements Adapter.
var _ Adapter = (*CoreBluetoothAdapter)(nil)

type coreBluetoothConnection struct {
	device       *bluetooth.Device
	disconnectCb func()
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
