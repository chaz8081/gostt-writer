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

	// Simulate disconnect on the current connection
	adapter.latestConnection().SimulateDisconnect()

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

func TestCloseStopsReconnectLoop(t *testing.T) {
	adapter := newMockAdapter([]Device{
		{Name: "ToothPaste-S3", MAC: "AA:BB:CC:DD:EE:FF", RSSI: -45},
	})
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), zeroDelayOpts())

	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	// Simulate disconnect to start reconnect loop
	adapter.latestConnection().SimulateDisconnect()

	// Close immediately — should stop the reconnect loop without hanging
	time.Sleep(10 * time.Millisecond) // let goroutine start
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Allow time for goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// Verify the reconnecting flag was cleared
	if client.reconnecting.Load() {
		t.Error("reconnecting should be false after Close() stops the loop")
	}
}

func TestBackoffDelayOverflowProtection(t *testing.T) {
	// Attempt=100 would cause 1<<100 overflow without the cap
	got := backoffDelay(100, 30)
	want := 30 * time.Second
	if got != want {
		t.Errorf("backoffDelay(100, 30) = %v, want %v (capped at max)", got, want)
	}

	// Attempt=31 should also be capped to the shift limit
	got = backoffDelay(31, 60)
	if got <= 0 {
		t.Errorf("backoffDelay(31, 60) = %v, should be positive", got)
	}
	if got > 60*time.Second {
		t.Errorf("backoffDelay(31, 60) = %v, should not exceed 60s", got)
	}
}

func TestConcurrentDisconnectsDoNotStackReconnects(t *testing.T) {
	adapter := newMockAdapter([]Device{
		{Name: "ToothPaste-S3", MAC: "AA:BB:CC:DD:EE:FF", RSSI: -45},
	})
	client := mustNewClient(t, adapter, "AA:BB:CC:DD:EE:FF", makeTestKey(), zeroDelayOpts())

	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	conn := adapter.latestConnection()

	// Trigger disconnect — the handler calls setDisconnected + spawns reconnectLoop.
	// Only one reconnect goroutine should be spawned thanks to the atomic guard.
	conn.SimulateDisconnect()

	// Before the reconnect loop finishes, try to trigger again. The atomic
	// guard should prevent a second goroutine from being spawned.
	if client.reconnecting.CompareAndSwap(false, true) {
		// If we got here, the guard failed — a second goroutine could spawn.
		// Reset the flag so the test is fair, then fail.
		client.reconnecting.Store(false)
		t.Error("reconnecting guard should have prevented a second swap to true")
	}

	// Wait for reconnect to complete
	time.Sleep(100 * time.Millisecond)

	// Should be reconnected
	client.mu.Lock()
	connected := client.connected
	client.mu.Unlock()

	if !connected {
		t.Error("client should be reconnected")
	}

	// reconnecting flag should be cleared after successful reconnect
	if client.reconnecting.Load() {
		t.Error("reconnecting flag should be cleared after successful reconnect")
	}
}
