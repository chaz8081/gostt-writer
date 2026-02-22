// Package ble provides the BLE client for communicating with an ESP32-S3
// running ToothPaste firmware. It handles connection management, encryption,
// and text transmission over Bluetooth Low Energy.
package ble

import "context"

// ToothPaste BLE UUIDs
const (
	ServiceUUID      = "19b10000-e8f2-537e-4f6c-d104768a1214"
	TXCharUUID       = "6856e119-2c7b-455a-bf42-cf7ddd2c5907"
	ResponseCharUUID = "6856e119-2c7b-455a-bf42-cf7ddd2c5908"
	MACCharUUID      = "19b10002-e8f2-537e-4f6c-d104768a1214"
)

// Characteristic represents a BLE GATT characteristic.
type Characteristic interface {
	// Write sends data to the characteristic.
	Write(data []byte) error
	// Subscribe registers a callback for notifications on this characteristic.
	Subscribe(callback func(data []byte)) error
}

// Device represents a discovered BLE peripheral.
type Device struct {
	Name string
	MAC  string
	RSSI int
}

// Connection represents an active BLE connection to a peripheral.
type Connection interface {
	// DiscoverCharacteristic finds a characteristic by UUID within a service.
	DiscoverCharacteristic(serviceUUID, charUUID string) (Characteristic, error)
	// Disconnect terminates the connection.
	Disconnect() error
	// OnDisconnect registers a callback invoked when the connection drops.
	OnDisconnect(callback func())
}

// Adapter abstracts the BLE hardware adapter for testing.
type Adapter interface {
	// Enable powers on the BLE adapter.
	Enable() error
	// Scan discovers BLE peripherals advertising the given service UUID.
	// Returns discovered devices until ctx is cancelled or timeout.
	Scan(ctx context.Context, serviceUUID string) ([]Device, error)
	// Connect establishes a connection to the device with the given MAC address.
	Connect(ctx context.Context, mac string) (Connection, error)
}
