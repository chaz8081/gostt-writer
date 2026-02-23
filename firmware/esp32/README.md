# GOSTT-KBD ESP32-S3 Firmware

Custom firmware that turns an ESP32-S3 into a BLE-to-USB-HID keyboard bridge. It receives AES-256-GCM encrypted text from the gostt-writer macOS app over BLE and types it as USB HID keystrokes on whatever device the ESP32-S3 is plugged into.

## Hardware

Any ESP32-S3 development board with **two USB-C ports** (one UART, one native USB-OTG) should work. The key requirement is the S3 variant — it has native USB-OTG for HID keyboard emulation.

**Tested with:** [Lonely Binary ESP32-S3 N16R8 Gold Edition](https://www.amazon.com/dp/B0FL149DGM) (16MB Flash, 8MB PSRAM, CH343 USB-UART, dual USB-C). A 3-pack runs ~$40.

The two USB-C ports serve different purposes:

| Port     | Purpose                              |
| -------- | ------------------------------------ |
| **UART** | Flashing firmware and serial monitor |
| **USB**  | USB HID keyboard output to target    |

The firmware uses:
- **GPIO 48** -- WS2812 addressable LED (standard on most ESP32-S3 dev boards)
- **GPIO 0** -- BOOT button (used for factory reset)

## Prerequisites

### 1. ESP-IDF (v6.1-dev)

ESP-IDF is included as a git submodule at `third_party/esp-idf`.

> **Note:** This project uses a development snapshot of ESP-IDF v6.1 (not a stable release). It is pinned to a tested commit via the git submodule. Do not switch to a different ESP-IDF version without testing.

```bash
# Initialize the submodule (first time only)
git submodule update --init --recursive third_party/esp-idf

# Install toolchain for ESP32-S3
third_party/esp-idf/install.sh esp32s3
```

> **Heads up:** The ESP-IDF submodule is ~1.8 GB and the toolchain install adds another ~1.5 GB. Expect 10-25 minutes for initial setup depending on your connection.

> You do **not** need to manually run `source export.sh` -- all `task fw-*` commands handle this automatically.
>
> To use a different ESP-IDF installation, set the `IDF_PATH` environment variable.

### 2. USB Serial Driver (macOS)

Most ESP32-S3 dev boards use a **CH34x** USB-UART chip that requires a third-party driver on macOS.

```bash
# Install the driver
brew install --cask wch-ch34x-usb-serial-driver

# Activate it
open /Applications/CH34xVCPDriver.app
```

After opening the app:
1. Go to **System Settings > General > Login Items & Extensions > Driver Extensions**
2. Enable the **CH34x** driver toggle
3. **Reboot**
4. Plug in the board -- verify with `ls /dev/cu.wchusbserial*`

You can also run `task fw-setup` which checks for both ESP-IDF and the serial driver.

### 3. Python 3.8+

ESP-IDF requires Python 3.8 or later for its build system and toolchain scripts. macOS includes Python 3 by default since Catalina. Verify with:

```bash
python3 --version
```

### 4. Task Runner

All commands use [Task](https://taskfile.dev):

```bash
brew install go-task
```

## Quick Start

```bash
# Plug the ESP32-S3 into your Mac via the UART port
task fw-build          # Build firmware
task fw-flash          # Flash to device
task fw-monitor        # Serial monitor (Ctrl+] to exit)
task fw-flash-monitor  # Flash + monitor in one step
```

## All Firmware Tasks

| Command               | Description                                        |
| --------------------- | -------------------------------------------------- |
| `task fw-setup`       | Check/install ESP-IDF and USB serial driver        |
| `task fw-build`       | Build firmware                                     |
| `task fw-flash`       | Flash to connected device                          |
| `task fw-monitor`     | Serial monitor                                     |
| `task fw-flash-monitor` | Flash and open serial monitor                    |
| `task fw-fullclean`   | Delete build directory and managed components      |
| `task fw-port`        | Print detected serial port                         |
| `task fw-test`        | Run host-side protobuf cross-validation tests      |

### Overrides

```bash
# Use a specific serial port
FW_PORT=/dev/cu.wchusbserial10 task fw-flash

# Use ESP-IDF from a different location (instead of the submodule)
IDF_PATH=/opt/esp-idf task fw-build
```

## Development Setup (Two Cables)

For active development, connect **both** USB-C cables simultaneously:

| Cable | Port | Purpose |
|-------|------|---------|
| Cable 1 | **UART** | Flashing, serial monitor, debug logs |
| Cable 2 | **USB** | HID keyboard output to target device |

This lets you flash, monitor logs, and test USB HID output at the same time. The UART connection stays on your Mac while the USB connection goes to whatever device you want to type on (phone, tablet, another PC).

## Pairing

1. Flash the firmware to the ESP32-S3 (via the **UART** port)
2. Plug the ESP32-S3 into the **target device** via the **USB** port (it enumerates as a USB HID keyboard)
3. On your Mac, run `task ble-pair` (or `gostt-writer --ble-pair`)
4. Pairing uses ECDH P-256 key exchange over BLE -- the shared secret is saved to your config
5. Set `inject.method: ble` in `~/.config/gostt-writer/config.yaml`
6. Verify pairing works — speak a test phrase. You should see the LED flash white briefly and text appear on the target device.

## LED Status

| LED State             | Color    | Meaning                              |
| --------------------- | -------- | ------------------------------------ |
| Slow blink            | Blue     | Advertising (waiting for BLE)        |
| Solid                 | Blue     | Connected (not yet paired)           |
| Solid                 | Green    | Paired and ready                     |
| Brief flash           | White    | Typing in progress                   |
| Triple flash          | Red      | Error                                |
| Alternating           | Red/Blue | Factory reset in progress            |

## Factory Reset

Hold the **BOOT** button for 5 seconds at startup. This erases all stored keys and mute configuration.

## Troubleshooting

### No serial port after plugging in

1. Make sure you're using the **UART** port (not the USB port)
2. Check the driver: `ls /dev/cu.wchusbserial*`
3. If missing, run `task fw-setup` or install the CH34x driver manually (see Prerequisites)
4. On macOS, you may need to approve the driver in System Settings and reboot

### Build fails with "idf.py not found"

The `task fw-*` commands source ESP-IDF automatically. If it still fails, verify the submodule is initialized:
```bash
git submodule update --init --recursive third_party/esp-idf
third_party/esp-idf/install.sh esp32s3
```

### Build fails with Python path mismatch

```
'python' is currently active while the project was configured with 'python3'
```

Run `task fw-fullclean` then rebuild. This happens when the build cache was created with a different Python path.

### LED stays off

Check USB connection and ensure firmware was flashed. Use `task fw-flash-monitor` to see boot output.

### BLE pairing fails

Make sure the ESP32-S3 is advertising (slow blue blink). Only one BLE connection is supported at a time.

### Typed text is garbled

Verify the target device recognizes the ESP32 as a USB HID keyboard. Try a different USB port or cable.
