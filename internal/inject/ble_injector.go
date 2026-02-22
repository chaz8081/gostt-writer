package inject

// BLESender is the interface the BLE client exposes for sending text.
type BLESender interface {
	Send(text string) error
}

// BLEInjector sends transcribed text over BLE to an ESP32-S3.
type BLEInjector struct {
	sender BLESender
}

// Compile-time interface satisfaction check.
var _ TextInjector = (*BLEInjector)(nil)

// NewBLEInjector creates a BLEInjector backed by the given sender.
// Panics if sender is nil (programmer error).
func NewBLEInjector(sender BLESender) *BLEInjector {
	if sender == nil {
		panic("inject: NewBLEInjector called with nil sender")
	}
	return &BLEInjector{sender: sender}
}

// Inject sends text to the ESP32 via BLE.
func (b *BLEInjector) Inject(text string) error {
	if text == "" {
		return nil
	}
	return b.sender.Send(text)
}
