package ble

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	blecrypto "github.com/chaz8081/gostt-writer/internal/ble/crypto"
	"github.com/chaz8081/gostt-writer/internal/ble/protocol"
)

// ClientOptions configures the BLE client behavior.
type ClientOptions struct {
	QueueSize    int // max queued messages during disconnect
	ReconnectMax int // max reconnect backoff in seconds
}

// DefaultClientOptions returns sensible defaults.
func DefaultClientOptions() ClientOptions {
	return ClientOptions{
		QueueSize:    64,
		ReconnectMax: 30,
	}
}

// Client manages the BLE connection to an ESP32-S3 running ToothPaste firmware.
type Client struct {
	adapter   Adapter
	deviceMAC string
	key       []byte // 32-byte AES encryption key

	mu        sync.Mutex
	conn      Connection
	txChar    Characteristic
	connected bool

	packetNum atomic.Uint32

	queue []string
	opts  ClientOptions
}

// NewClient creates a BLE client for the given paired device.
func NewClient(adapter Adapter, deviceMAC string, key []byte, opts ClientOptions) *Client {
	if opts.QueueSize <= 0 {
		opts.QueueSize = 64
	}
	if opts.ReconnectMax <= 0 {
		opts.ReconnectMax = 30
	}
	return &Client{
		adapter:   adapter,
		deviceMAC: deviceMAC,
		key:       key,
		opts:      opts,
	}
}

// Send encrypts and transmits text to the ESP32. If disconnected, the text
// is queued for delivery on reconnect. Safe for concurrent use.
func (c *Client) Send(text string) error {
	if text == "" {
		return nil
	}

	c.mu.Lock()
	if !c.connected {
		c.enqueue(text)
		c.mu.Unlock()
		return nil
	}
	txChar := c.txChar
	c.mu.Unlock()

	return c.sendChunked(txChar, text)
}

// sendChunked splits text into BLE-MTU-safe chunks, encrypts each, and writes.
func (c *Client) sendChunked(txChar Characteristic, text string) error {
	chunks := protocol.ChunkText(text, protocol.MaxPayloadBytes)
	for i, chunk := range chunks {
		if err := c.sendOne(txChar, chunk); err != nil {
			return err
		}
		// Small delay between chunks to avoid overwhelming the ESP32
		if i < len(chunks)-1 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	return nil
}

// sendOne encrypts and sends a single chunk.
func (c *Client) sendOne(txChar Characteristic, text string) error {
	// Build inner protobuf
	kbPacket := protocol.MarshalKeyboardPacket(text)
	encData := protocol.MarshalEncryptedData(kbPacket)

	// Encrypt
	iv, ciphertext, tag, err := blecrypto.Encrypt(c.key, encData)
	if err != nil {
		return fmt.Errorf("ble: encrypt: %w", err)
	}

	// Build outer DataPacket
	pktNum := c.packetNum.Add(1)
	dataPacket, err := protocol.MarshalDataPacket(iv, tag, ciphertext, pktNum)
	if err != nil {
		return fmt.Errorf("ble: marshal data packet: %w", err)
	}

	return txChar.Write(dataPacket)
}

// enqueue adds text to the send queue (caller must hold mu).
func (c *Client) enqueue(text string) {
	if len(c.queue) >= c.opts.QueueSize {
		// Drop oldest
		slog.Warn("[BLE] queue full, dropping oldest message")
		c.queue = c.queue[1:]
	}
	c.queue = append(c.queue, text)
}

// QueueLen returns the number of queued messages.
func (c *Client) QueueLen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.queue)
}

// setConnected sets the connection state (for testing and reconnection).
func (c *Client) setConnected(conn Connection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
	txChar, err := conn.DiscoverCharacteristic(ServiceUUID, TXCharUUID)
	if err != nil {
		slog.Error("[BLE] failed to discover TX characteristic", "error", err)
		return
	}
	c.txChar = txChar
	c.connected = true
}

// setDisconnected marks the client as disconnected.
func (c *Client) setDisconnected() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	c.conn = nil
	c.txChar = nil
}

// flushQueue sends all queued messages. Call after reconnection.
func (c *Client) flushQueue() {
	c.mu.Lock()
	if !c.connected || len(c.queue) == 0 {
		c.mu.Unlock()
		return
	}
	queued := make([]string, len(c.queue))
	copy(queued, c.queue)
	c.queue = c.queue[:0]
	txChar := c.txChar
	c.mu.Unlock()

	for _, text := range queued {
		if err := c.sendChunked(txChar, text); err != nil {
			slog.Error("[BLE] failed to flush queued message", "error", err)
		}
	}
}

// Close gracefully disconnects the BLE client.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.queue) > 0 {
		slog.Warn("[BLE] closing with unsent messages", "count", len(c.queue))
	}

	if c.conn != nil {
		c.conn.Disconnect()
	}
	c.connected = false
	return nil
}
