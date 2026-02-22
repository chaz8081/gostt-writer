# ESP32-S3 Firmware Design: GOSTT-KBD

**Date:** 2026-02-22
**Status:** Validated

## Summary

Custom ESP-IDF firmware for ESP32-S3 that acts as an encrypted BLE-to-USB-HID bridge.
Receives AES-256-GCM encrypted text from the gostt-writer macOS app over BLE, decrypts it,
and types it as USB HID keystrokes on any target device (iPad, locked-down work computer,
gaming console, etc.). Also supports a configurable mute toggle via USB HID Consumer Control.

Written from scratch — no ToothPaste code. Wire-compatible with gostt-writer's existing
BLE protocol.

## Design Decisions

| # | Decision | Answer |
|---|----------|--------|
| 1 | Firmware origin | Custom from scratch (not ToothPaste fork) |
| 2 | Feature scope | Text injection + flexible audio mute |
| 3 | Framework | ESP-IDF (native C) |
| 4 | Build system | CMake + idf.py |
| 5 | Repo location | `firmware/esp32/` in monorepo |
| 6 | Mute strategy | Flexible: default USB HID Consumer Control 0xE2, reconfigurable over BLE to shortcuts/macros, persisted in NVS |
| 7 | Key storage | NVS (Non-Volatile Storage) |
| 8 | Status LED | WS2812 on GPIO 48 (configurable), 5 states |
| 9 | BLE device name | GOSTT-KBD |
| 10 | USB device | Composite HID: Keyboard + Consumer Control via TinyUSB |
| 11 | Protobuf | Hand-written C decoder, no protoc |
| 12 | Factory reset | BOOT button hold 5s, clears NVS |

## Architecture

Three main subsystems running on ESP-IDF (FreeRTOS):

```
                    gostt-writer (macOS)
                          |
                     BLE (encrypted)
                          |
                    ┌─────┴─────┐
                    │ BLE Server │  ← GATT service, advertising, connections
                    └─────┬─────┘
                          |
                   ┌──────┴──────┐
                   │Crypto Engine│  ← ECDH pairing, AES-GCM decrypt
                   └──────┬──────┘
                          |
                   ┌──────┴──────┐
                   │ USB HID Out │  ← Keyboard + Consumer Control
                   └──────┬──────┘
                          |
                     USB cable
                          |
                    Target Device
                 (iPad, PC, etc.)
```

**Data flow:** BLE write → protobuf decode (DataPacket) → AES-GCM decrypt → protobuf
decode (EncryptedData/KeyboardPacket or Command) → extract text or command → execute
(type keystrokes or trigger mute action).

## BLE Server

### Advertising

Advertises continuously as "GOSTT-KBD" with service UUID
`19b10000-e8f2-537e-4f6c-d104768a1214`. Advertising restarts automatically on disconnect.

### GATT Service

One primary service with three characteristics matching the gostt-writer app's expectations:

| Characteristic | UUID | Properties | Purpose |
|---|---|---|---|
| TX | `6856e119-2c7b-455a-bf42-cf7ddd2c5907` | Write | App writes encrypted DataPackets here |
| Response | `6856e119-2c7b-455a-bf42-cf7ddd2c5908` | Notify | Firmware sends ResponsePackets (pairing pubkey, keepalives) |
| MAC | `19b10002-e8f2-537e-4f6c-d104768a1214` | Read | Returns device BLE MAC address (6 bytes) |

### Connection Handling

- Single connection only (reject additional connections while one is active)
- On connect: stop advertising, set LED solid blue
- On disconnect: clear session state (keep persisted shared key), restart advertising
- MTU negotiation: request 256 bytes (ESP-IDF NimBLE default supports this)

### Keepalives

Firmware sends `ResponsePacket(type=Keepalive)` on Response characteristic every 5 seconds
while connected. Lets the app detect stale connections.

## Crypto Engine

### Library

mbedtls — ships with ESP-IDF, no extra dependencies.

### Pairing (device side)

1. App writes 33-byte compressed ECDH public key to TX characteristic (unencrypted, raw
   key — firmware detects this is not a DataPacket by size/framing)
2. Firmware generates ECDH keypair on secp256r1 (P-256) via `mbedtls_ecdh_gen_public`
3. Computes shared secret via `mbedtls_ecdh_compute_shared`
4. Derives 32-byte AES key: `HKDF-SHA256(secret, salt=nil, info="toothpaste")`
5. Stores AES key (32 bytes) + app's compressed public key (33 bytes) in NVS
6. Sends `ResponsePacket(type=PeerStatus, status=Known, data=own_compressed_pubkey)` as
   notification on Response characteristic
7. LED turns solid green

### Decryption (normal operation)

1. Receive DataPacket on TX characteristic
2. Decode protobuf: extract `iv` (12 bytes), `tag` (16 bytes), `encrypted_data` (variable),
   `packet_num` (uint32)
3. AES-256-GCM decrypt using stored key, iv, tag, empty AAD
4. Result is serialized `EncryptedData` protobuf
5. Decode inner protobuf to get plaintext text or command

### Protobuf Wire Format

Hand-written C decoder matching the Go app's hand-written encoder. No protoc dependency.
~150-200 lines of C. Four message types:

```
DataPacket (outer envelope):
  field 1: iv             (bytes, 12 bytes)
  field 2: tag            (bytes, 16 bytes)
  field 3: encrypted_data (bytes, variable)
  field 4: packet_num     (uint32)

EncryptedData (inner wrapper, after decryption):
  field 1: keyboard_packet (bytes)

KeyboardPacket (text payload):
  field 1: message         (string)
  field 2: length          (uint32)

ResponsePacket (firmware → app):
  field 1: type            (uint32: 0=Keepalive, 1=PeerStatus)
  field 2: peer_status     (uint32: 0=Unknown, 1=Known)
  field 3: data            (bytes, 33-byte compressed pubkey during pairing)
```

### Command Extension

To support mute and future commands, the encrypted payload includes a command type
discriminator. The `EncryptedData` inner message is extended:

```
EncryptedData:
  field 1: keyboard_packet (bytes)   — present for text (type 0)
  field 2: command_type    (uint32)  — 0=text, 1=mute_toggle, 2=configure_mute
  field 3: command_data    (bytes)   — mute config payload for type 2
```

When `command_type` is 0 or absent, `keyboard_packet` is decoded and typed as text.
When `command_type` is 1, the configured mute action executes.
When `command_type` is 2, `command_data` contains a new mute action definition to persist.

### Anti-replay (stretch goal)

Track `packet_num` and reject packets with numbers ≤ last seen. Not critical for V1
since this is a local radio link, but easy to add later.

## USB HID Output

### USB Stack

ESP-IDF's TinyUSB integration (`tinyusb` component). The ESP32-S3's native USB-OTG
peripheral handles this in hardware — no external USB chip needed.

### Composite Device

Two HID interfaces on one USB device:

1. **HID Keyboard** — Standard 6-key rollover boot keyboard. Types decrypted text
   character by character.
2. **HID Consumer Control** — Single 16-bit usage code. Executes mute toggle and
   any future media actions.

### USB Device Descriptor

- Vendor: "gostt-writer"
- Product: "GOSTT-KBD"
- USB VID/PID: ESP-IDF defaults (Espressif test VID 0x303A) for development

### Text-to-Keystroke Conversion

- ASCII printable (0x20–0x7E): lookup table → USB HID keycodes
- Shift modifier applied automatically for uppercase and symbols
- `\n` → Enter, `\t` → Tab
- Non-ASCII characters: skipped in V1 (UTF-8/Unicode requires OS-specific IME input)

### Typing Cadence

- 5ms press duration (key-down to key-up)
- 2ms inter-key gap
- ~100 chars/sec throughput — well above real-time transcription speed
- Configurable via NVS if a target device needs slower input

## Flexible Mute System

### BLE Command Protocol

The encrypted payload distinguishes text from commands via `command_type`:

| Type | Name | Behavior |
|------|------|----------|
| 0 | Text | Decode KeyboardPacket, type as keystrokes |
| 1 | Mute toggle | Execute configured mute action |
| 2 | Configure mute | Parse command_data as new mute config, store in NVS |

### Mute Action Types

The mute action is stored in NVS as a small config blob. Three action types:

1. **Consumer Control key** — Single HID consumer usage ID. Default: 0xE2 (Mute).
   Also supports 0xE9 (Volume Up), 0xEA (Volume Down), etc.
2. **Keyboard shortcut** — Modifier mask + keycode. Examples: Cmd+Shift+M (Zoom),
   Win+Alt+K (Teams), Ctrl+D (Google Meet).
3. **Multi-step macro** — Sequence of HID actions with configurable delays between
   steps. Supports push-to-talk style workflows.

### Configuration Over BLE

A Type 2 command pushes a new mute action definition to the firmware, which persists
it to NVS. No reflashing required. The gostt-writer app can expose this via a
`task ble-configure-mute` command or config YAML option.

### Default

USB Consumer Control Mute (0xE2) out of the box. Works on most platforms as a
starting point. Behavior note: on some platforms this mutes speakers rather than
microphone — users may need to adjust OS settings or reconfigure to a keyboard shortcut.

## LED Status & State Machine

### Hardware

WS2812 (NeoPixel) RGB LED controlled via ESP-IDF's RMT peripheral driver. Default
GPIO 48 (configurable via `menuconfig` for boards that differ).

### States

| State | LED | Description |
|-------|-----|-------------|
| ADVERTISING | Slow blue blink (1Hz) | Waiting for BLE connection |
| CONNECTED | Solid blue | BLE connected, not yet paired |
| PAIRED | Solid green | Paired and ready to receive |
| TYPING | Brief white flash per packet | Actively typing received text |
| ERROR | Red flash (3x then resume) | Decrypt failure, USB error, etc. |

### Transitions

```
Boot → check NVS for stored key
  ├── key found → ADVERTISING (goes to PAIRED on connect)
  └── no key   → ADVERTISING (goes to CONNECTED on connect)

ADVERTISING → BLE connect → CONNECTED or PAIRED (depends on stored key)
CONNECTED   → successful pairing → PAIRED
PAIRED      → receive packet → TYPING → done → PAIRED
Any state   → decrypt failure → ERROR → previous state
Any connected state → BLE disconnect → ADVERTISING
```

### Factory Reset

Hold BOOT button for 5 seconds → clears NVS (erases stored keys and mute config) →
flashes red/blue alternating → reboots into ADVERTISING state.

## Project Structure

```
firmware/esp32/
├── CMakeLists.txt              # Top-level ESP-IDF project file
├── sdkconfig.defaults          # Default config (USB-OTG, NimBLE, TinyUSB, etc.)
├── partitions.csv              # Partition table (includes NVS partition)
├── main/
│   ├── CMakeLists.txt          # Component registration
│   ├── main.c                  # Entry point, init subsystems, state machine loop
│   ├── ble_server.c/.h         # BLE GATT server, advertising, connection handling
│   ├── crypto.c/.h             # ECDH, HKDF, AES-GCM decrypt, key storage (NVS)
│   ├── proto.c/.h              # Protobuf decode/encode (DataPacket, ResponsePacket, etc.)
│   ├── usb_hid.c/.h            # TinyUSB composite device, keyboard + consumer control
│   ├── mute.c/.h               # Mute action config, storage, execution
│   ├── led.c/.h                # WS2812 LED control, status patterns
│   └── config.h                # Pin definitions, timing constants, default values
```

Estimated total: ~800-1000 lines of C.

## Taskfile Integration

New tasks in the root `Taskfile.yml`:

- `task fw-build` — build firmware (`idf.py build` in `firmware/esp32/`)
- `task fw-flash` — flash to connected ESP32-S3
- `task fw-monitor` — serial monitor for debug output

## Testing Strategy

### Unit tests (host-side, no hardware)

The protobuf encoder/decoder and crypto functions can be tested on the host using
ESP-IDF's host test framework or a simple C test harness:

- Protobuf encode/decode against known golden bytes (cross-validated with Go tests)
- AES-GCM decrypt with test vectors from the Go crypto tests
- HID keycode lookup table coverage

### Integration tests (with hardware)

- Pair with gostt-writer app, verify key exchange completes
- Send encrypted text, verify keystrokes appear on target device
- Walk out of BLE range, verify reconnect works
- Factory reset, verify re-pairing works
- Long text (>253 bytes, multiple BLE packets) verify all chunks typed correctly
- Mute toggle, verify HID consumer report sent
- Reconfigure mute over BLE, verify new action persists across reboot

### Cross-validation with Go tests

The Go protocol tests (`internal/ble/protocol/proto_test.go`) produce known-good
encoded bytes. The C decoder must produce identical decoded output from those same
bytes, and vice versa. This ensures wire compatibility.
