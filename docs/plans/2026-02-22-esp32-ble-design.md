# V3 Design: ESP32-S3 BLE Output

**Date:** 2026-02-22
**Status:** Validated

## Summary

gostt-writer V3 adds BLE output as a new injection method. Transcribed text is streamed
over BLE to an ESP32-S3 running [ToothPaste](https://github.com/Brisk4t/ToothPaste)
firmware, which acts as a USB HID keyboard on any target device (iPad, locked-down work
computer, gaming console, etc.).

## Design Decisions

| # | Decision | Answer |
|---|----------|--------|
| 1 | Transport | BLE primary; USB serial deferred to post-V3 |
| 2 | Inject behavior | New `inject.method: ble` value, replaces local injection (not dual output) |
| 3 | Security | ECDH key exchange + AES-256-GCM (matching ToothPaste protocol) |
| 4 | ESP32 firmware | Reuse ToothPaste firmware as-is; gostt-writer speaks its protocol |
| 5 | Text chunking | App-side chunking for BLE MTU (~213 usable bytes per packet) |
| 6 | Pairing UX | CLI command: `task ble-pair` / `gostt-writer --ble-pair` |
| 7 | Pairing storage | `inject.ble` section in `~/.config/gostt-writer/config.yaml` |
| 8 | USB serial scope | Deferred to separate follow-up |
| 9 | BLE library | `tinygo-org/bluetooth` |
| 10 | Reconnection | Auto-reconnect with exponential backoff + bounded queue |
| 11 | Privacy framing | Distinguish local radio (BLE) vs internet; "zero internet connections" |

## Architecture

The existing pipeline -- hotkey, audio capture, transcription -- is unchanged. Only the
injection stage gains a new method.

```
Hotkey --> Recorder --> Transcriber --> Injector
(gohook)   (malgo)    (whisper/        |
                       parakeet)   type / paste / ble
                                            |
                                       BLE Client
                                    (tinygo/bluetooth)
                                            |
                                        ESP32-S3
                                     (ToothPaste fw)
                                            |
                                        USB HID
                                            |
                                      Target Device
                                   (iPad, PC, etc.)
```

### New packages

- `internal/ble/` -- BLE client: scanning, connecting, GATT operations, reconnection
- `internal/ble/crypto/` -- ECDH key exchange, AES-256-GCM, HKDF key derivation
- `internal/ble/protocol/` -- ToothPaste protobuf hand-encoding (DataPacket, KeyboardPacket, ResponsePacket)

The `internal/inject/` package gains a `BLEInjector` that implements the existing
injector interface, delegating to `internal/ble/`.

## BLE Client Lifecycle

### Startup

1. Read `inject.ble.device_mac` and `inject.ble.shared_secret` from config
2. If either is missing, exit with error: `"BLE not paired. Run: task ble-pair"`
3. Enable BLE adapter via `tinygo-org/bluetooth`
4. Filtered scan for the paired MAC address
5. Connect and discover the ToothPaste GATT service (`19b10000-e8f2-537e-4f6c-d104768a1214`)
6. Subscribe to Response characteristic for keepalive/status
7. Connection ready

### Reconnection

- On disconnect, a goroutine retries with exponential backoff: 1s, 2s, 4s, 8s, 16s, capped at 30s
- Terminal shows `[BLE] disconnected, reconnecting...` and `[BLE] reconnected`
- Bounded queue (default 64 entries) buffers transcriptions during disconnection
- On reconnect, queue flushes in order
- Queue overflow drops oldest entries with a warning

### Shutdown

- On SIGINT/SIGTERM, gracefully disconnect (GATT disconnect + adapter cleanup)
- Unsent queued transcriptions logged as warnings

### Thread safety

- `Send(text string) error` is safe for concurrent use
- Internal state protected by mutex
- Reconnection goroutine is sole writer of connection state

## Security

### Pairing (one-time)

1. Generate ECDH keypair: secp256r1 (P-256) via Go `crypto/ecdh`
2. Connect to ESP32, read Response characteristic -- firmware sends `ResponsePacket` with `peer_status: PEER_UNKNOWN` + challenge
3. Write 33-byte compressed public key to TX characteristic as AUTH packet
4. ESP32 responds with its compressed public key
5. Both sides compute shared secret via ECDH
6. Derive encryption key: `HKDF-SHA256(shared_secret, salt=nil, info="toothpaste", length=32)`
7. Save `device_mac` and `shared_secret` (hex) to config
8. ESP32 stores same key in NVS flash

### Per-message encryption

1. Generate random 12-byte IV (`crypto/rand`)
2. Build inner `EncryptedData` protobuf with `KeyboardPacket` (text + length)
3. Serialize inner protobuf
4. Encrypt: `ciphertext, tag = AES-256-GCM(key, iv, plaintext)`
5. Wrap in `DataPacket`: IV, tag, encrypted data, incrementing packet number
6. Write to TX characteristic

### Dependencies

All Go stdlib + `golang.org/x/crypto/hkdf`. No external crypto libraries.

## Protocol Encoding

Hand-encoded protobuf -- no `protoc` dependency. Three message types:

```
DataPacket:
  field 1: iv           (bytes)
  field 2: tag          (bytes)
  field 3: encrypted    (bytes)
  field 4: packet_num   (uint32)

KeyboardPacket (inside EncryptedData):
  field 1: message      (string)
  field 2: length       (uint32)

ResponsePacket:
  field 1: type         (enum: KEEPALIVE=0, PEER_STATUS=1)
  field 2: peer_status  (enum: PEER_UNKNOWN=0, PEER_KNOWN=1)
  field 3: data         (bytes)
```

### Text chunking

253 bytes max BLE write. After protobuf + AES-GCM overhead: **~213 usable bytes per packet**.

- UTF-8 aware: never split mid-character
- Prefer word boundary splits (space, punctuation)
- Each chunk independently encrypted with its own `DataPacket` and incrementing packet number
- Sequential sends with ~20ms inter-packet delay

## Config Schema

```yaml
inject:
  method: ble
  ble:
    device_mac: "AA:BB:CC:DD:EE:FF"      # written by ble-pair, not user
    shared_secret: "a1b2c3..."            # 64-char hex (32 bytes), written by ble-pair
    queue_size: 64                         # optional, default 64
    reconnect_max: 30                      # optional, max backoff seconds, default 30
```

## Pairing Command

Two entry points, same implementation:

- `task ble-pair` -- Taskfile convenience
- `gostt-writer --ble-pair` -- binary flag

User experience:

```
$ task ble-pair
Scanning for ToothPaste devices...
Found 1 device(s):
  [1] ToothPaste-S3  (AA:BB:CC:DD:EE:FF)  RSSI: -45

Select device [1]: 1
Pairing with ToothPaste-S3...
Key exchange complete.
Saved to ~/.config/gostt-writer/config.yaml

Ready! Set inject.method to "ble" in your config to use.
```

- Scan timeout: 10 seconds
- Multiple devices: user picks by number
- Single device: auto-select with confirmation
- Writes `device_mac` and `shared_secret` to config, preserving other fields

## Testing Strategy

### Unit tests (no hardware)

- `internal/ble/crypto/` -- ECDH, HKDF against known test vectors, AES-GCM round-trips
- `internal/ble/protocol/` -- Marshal/unmarshal against golden bytes, chunk splitting (ASCII, multi-byte UTF-8, empty, exact-max, one-over)
- `internal/ble/` -- Reconnection state machine (mock adapter), queue behavior (fill, overflow, flush), concurrent `Send()`
- `internal/inject/` -- `BLEInjector` interface compliance, delegation
- `internal/config/` -- New `inject.ble` section parsing, backward compat

### Integration tests (mocked BLE)

- `BLEAdapter` interface allows mock in tests
- `MockAdapter` simulates connect, disconnect, characteristic writes, notifications
- Full flow tested: pair, connect, encrypt, chunk, send, reconnect

### Manual testing checklist

- Pair with real ESP32-S3 running ToothPaste firmware
- Dictate text, verify it types on target device
- Walk out of range, verify reconnect + queue flush
- Re-pair to verify config overwrite
- Long text (>253 bytes) to verify chunking

## Privacy Updates

The README privacy section updates:

- "Zero network connections" becomes "zero internet connections"
- BLE documented as local radio (~10m range), not internet
- BLE traffic encrypted end-to-end (AES-256-GCM, pre-shared key)
- Pairing is explicit, user-initiated
- No BLE advertising/scanning during normal operation -- connects to single known MAC
- Shared secret stored locally, never transmitted over any network
