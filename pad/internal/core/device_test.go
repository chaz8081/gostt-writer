package core

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestDeviceWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hidg0")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	d := &DeviceWriter{Path: path}
	if err := WriteReports(d, Tap(0, KeyEnter)); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Error("closing twice must be harmless")
	}
	b, _ := os.ReadFile(path)
	if len(b) != 16 || b[2] != KeyEnter {
		t.Errorf("device got %v", b)
	}

	missing := &DeviceWriter{Path: filepath.Join(t.TempDir(), "absent")}
	if _, err := missing.Write(make([]byte, 8)); err == nil {
		t.Error("want an error when the device does not exist")
	}
}

// A host that stops reading must not block the pad: a FIFO nobody reads
// stands in for a HID gadget with the cable unplugged.
func TestDeviceWriterTimesOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("no FIFO support here: %v", err)
	}
	d := &DeviceWriter{Path: path, Timeout: 50 * time.Millisecond}
	defer func() { _ = d.Close() }()
	block := make([]byte, 4096)
	start := time.Now()
	var err error
	for i := 0; i < 1000 && err == nil; i++ { // fill the pipe until a write cannot complete
		_, err = d.Write(block)
	}
	if err == nil {
		t.Fatal("writes to a full pipe never failed")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %v to give up", time.Since(start))
	}
	if d.f != nil {
		t.Error("the device should be closed after a failed write, to be reopened later")
	}
}

func TestReportQueueDropsWhenFull(t *testing.T) {
	release := make(chan struct{})
	w := writerFunc(func(p []byte) (int, error) { <-release; return len(p), nil })
	q := NewReportQueue(w, 0, nil)
	batches := 0
	q.OnBatch = func() { batches++ }
	done := make(chan struct{})
	go func() {
		for range 3000 { // far more than the queue holds; must not block
			q.Send(Tap(0, KeyEnter))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Send blocked on a stalled host")
	}
	close(release)
	q.Close()
	if batches == 0 || batches >= 3000 {
		t.Errorf("batches written = %d; some should be written and some dropped", batches)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
