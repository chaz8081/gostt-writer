package ble

import (
	"testing"
	"time"
)

func TestReconnectBackoff(t *testing.T) {
	delays := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second, // capped
		30 * time.Second, // still capped
	}

	for i, want := range delays {
		got := backoffDelay(i, 30)
		if got != want {
			t.Errorf("backoffDelay(%d, 30) = %v, want %v", i, got, want)
		}
	}
}

func TestClientConnectAndReconnect(t *testing.T) {
	adapter := newMockAdapter([]Device{
		{Name: "ToothPaste-S3", MAC: "AA:BB:CC:DD:EE:FF", RSSI: -45},
	})
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), zeroDelayOpts())

	// Connect
	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	client.mu.Lock()
	if !client.connected {
		t.Error("client should be connected after Connect()")
	}
	client.mu.Unlock()

	// Simulate disconnect
	adapter.connection.SimulateDisconnect()

	// Give reconnect goroutine a moment — the reconnect loop attempts
	// immediately on the first try, so 100ms is plenty for the mock.
	time.Sleep(100 * time.Millisecond)

	// The test mock reconnects immediately, so we should be reconnected
	client.mu.Lock()
	connected := client.connected
	client.mu.Unlock()

	if !connected {
		t.Error("client should be reconnected after SimulateDisconnect")
	}
}
