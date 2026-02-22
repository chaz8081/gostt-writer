package ble

import (
	"context"
	"fmt"
	"time"

	blecrypto "github.com/chaz8081/gostt-writer/internal/ble/crypto"
	"github.com/chaz8081/gostt-writer/internal/ble/protocol"
)

// PairResult contains the data needed to save to config after pairing.
type PairResult struct {
	DeviceMAC    string
	SharedSecret []byte // 32-byte derived encryption key
}

// PairOptions configures pairing behavior.
type PairOptions struct {
	Timeout time.Duration // how long to wait for peer public key
}

// DefaultPairOptions returns sensible defaults for production use.
func DefaultPairOptions() PairOptions {
	return PairOptions{
		Timeout: 10 * time.Second,
	}
}

// ScanForDevices scans for ESP32 devices advertising the ToothPaste service.
func ScanForDevices(adapter Adapter, timeout time.Duration) ([]Device, error) {
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("ble: enable adapter: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	devices, err := adapter.Scan(ctx, ServiceUUID)
	if err != nil {
		return nil, fmt.Errorf("ble: scan: %w", err)
	}
	return devices, nil
}

// Pair performs the ECDH key exchange with the specified device.
func Pair(adapter Adapter, deviceMAC string, opts PairOptions) (*PairResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("ble: enable adapter: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	conn, err := adapter.Connect(ctx, deviceMAC)
	if err != nil {
		return nil, fmt.Errorf("ble: connect for pairing: %w", err)
	}
	defer func() { _ = conn.Disconnect() }()

	// Discover characteristics
	txChar, err := conn.DiscoverCharacteristic(ServiceUUID, TXCharUUID)
	if err != nil {
		return nil, fmt.Errorf("ble: discover TX char: %w", err)
	}
	respChar, err := conn.DiscoverCharacteristic(ServiceUUID, ResponseCharUUID)
	if err != nil {
		return nil, fmt.Errorf("ble: discover response char: %w", err)
	}

	// Subscribe to response notifications
	peerPubKeyCh := make(chan []byte, 1)
	if err := respChar.Subscribe(func(data []byte) {
		resp, err := protocol.UnmarshalResponsePacket(data)
		if err != nil {
			return
		}
		// The ESP32 sends its public key as challenge data in a PEER_STATUS response
		if resp.Type == protocol.ResponseTypePeerStatus && len(resp.Data) == 33 {
			peerPubKeyCh <- resp.Data
		}
	}); err != nil {
		return nil, fmt.Errorf("ble: subscribe to responses: %w", err)
	}

	// Generate our ECDH keypair
	privKey, pubKey, err := blecrypto.GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	// Send our compressed public key to the TX characteristic
	compressed := blecrypto.CompressPublicKey(pubKey)
	if err := txChar.Write(compressed); err != nil {
		return nil, fmt.Errorf("ble: write public key: %w", err)
	}

	// Wait for peer's public key (with timeout)
	select {
	case peerPubKeyBytes := <-peerPubKeyCh:
		peerPubKey, err := blecrypto.ParseCompressedPublicKey(peerPubKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("ble: parse peer public key: %w", err)
		}

		// Derive shared secret
		sharedSecret, err := blecrypto.DeriveSharedSecret(privKey, peerPubKey)
		if err != nil {
			return nil, err
		}

		// Derive encryption key
		encKey, err := blecrypto.DeriveEncryptionKey(sharedSecret)
		if err != nil {
			return nil, err
		}

		return &PairResult{
			DeviceMAC:    deviceMAC,
			SharedSecret: encKey,
		}, nil

	case <-time.After(opts.Timeout):
		return nil, fmt.Errorf("ble: pairing timed out waiting for peer public key")
	}
}
