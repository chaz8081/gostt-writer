package inject

// BLESender is the interface the BLE client exposes for sending text.
type BLESender interface {
	Send(text string) error
}

// BLEInjector sends transcribed text over BLE to an ESP32-S3.
type BLEInjector struct {
	sender BLESender
}

// NewBLEInjector creates a BLEInjector backed by the given sender.
func NewBLEInjector(sender BLESender) *BLEInjector {
	return &BLEInjector{sender: sender}
}

// Inject sends text to the ESP32 via BLE.
func (b *BLEInjector) Inject(text string) error {
	if text == "" {
		return nil
	}
	return b.sender.Send(text)
}
