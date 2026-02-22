package ble

import (
	"context"
	"fmt"
	"sync"

	"tinygo.org/x/bluetooth"
)

// CoreBluetoothAdapter wraps tinygo-org/bluetooth for macOS.
type CoreBluetoothAdapter struct {
	adapter *bluetooth.Adapter
}

// NewCoreBluetoothAdapter creates a new BLE adapter using CoreBluetooth.
func NewCoreBluetoothAdapter() *CoreBluetoothAdapter {
	return &CoreBluetoothAdapter{
		adapter: bluetooth.DefaultAdapter,
	}
}

func (a *CoreBluetoothAdapter) Enable() error {
	return a.adapter.Enable()
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
	addr := bluetooth.Address{}
	device, err := a.adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("ble: connect to %s: %w", mac, err)
	}
	return &coreBluetoothConnection{device: &device}, nil
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
	return nil // tinygo/bluetooth macOS doesn't support direct disconnect yet
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
