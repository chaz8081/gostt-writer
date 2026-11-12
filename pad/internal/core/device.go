package core

import (
	"fmt"
	"os"
	"time"
)

// DeviceWriter writes keyboard reports to a HID gadget device such as
// /dev/hidg0. It opens the device on first use and reopens it after an
// error, so the pad survives the cable being unplugged and plugged back in.
// A write that the host does not take within Timeout fails instead of
// blocking for ever.
type DeviceWriter struct {
	Path    string
	Timeout time.Duration // zero means one second

	f *os.File
}

// Write sends one report.
func (d *DeviceWriter) Write(p []byte) (int, error) {
	if d.f == nil {
		f, err := os.OpenFile(d.Path, os.O_RDWR, 0)
		if err != nil {
			return 0, fmt.Errorf("pad: open hid device: %w", err)
		}
		d.f = f
	}
	timeout := d.Timeout
	if timeout == 0 {
		timeout = time.Second
	}
	// Not every file supports deadlines; a device that does not simply
	// blocks as before.
	_ = d.f.SetWriteDeadline(time.Now().Add(timeout))
	n, err := d.f.Write(p)
	if err != nil {
		_ = d.f.Close()
		d.f = nil
		return n, fmt.Errorf("pad: write hid device: %w", err)
	}
	return n, nil
}

// Close closes the device if it is open.
func (d *DeviceWriter) Close() error {
	if d.f == nil {
		return nil
	}
	err := d.f.Close()
	d.f = nil
	return err
}
