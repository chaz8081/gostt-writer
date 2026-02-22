package inject

import (
	"errors"
	"testing"
)

// mockBLESender records Send calls.
type mockBLESender struct {
	sent []string
}

func (m *mockBLESender) Send(text string) error {
	m.sent = append(m.sent, text)
	return nil
}

func TestBLEInjectorInject(t *testing.T) {
	mock := &mockBLESender{}
	inj := NewBLEInjector(mock)

	err := inj.Inject("hello world")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if len(mock.sent) != 1 || mock.sent[0] != "hello world" {
		t.Errorf("sent = %v, want [\"hello world\"]", mock.sent)
	}
}

func TestBLEInjectorInjectEmpty(t *testing.T) {
	mock := &mockBLESender{}
	inj := NewBLEInjector(mock)

	err := inj.Inject("")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if len(mock.sent) != 0 {
		t.Errorf("sent = %v, want empty", mock.sent)
	}
}

// errBLESender always returns a fixed error.
type errBLESender struct{ err error }

func (e *errBLESender) Send(string) error { return e.err }

func TestBLEInjectorInjectError(t *testing.T) {
	want := errors.New("ble: disconnected")
	inj := NewBLEInjector(&errBLESender{err: want})
	got := inj.Inject("hello")
	if got != want {
		t.Errorf("Inject() error = %v, want %v", got, want)
	}
}

func TestNewBLEInjectorNilSenderPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("NewBLEInjector(nil) did not panic")
		}
	}()
	NewBLEInjector(nil)
}
