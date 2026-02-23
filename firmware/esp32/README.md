# GOSTT-KBD ESP32-S3 Firmware

Custom firmware that turns an ESP32-S3 into a BLE-to-USB-HID keyboard bridge. It receives AES-256-GCM encrypted text from the gostt-writer macOS app over BLE and types it as USB HID keystrokes on whatever device the ESP32-S3 is plugged into.

## Hardware

Any ESP32-S3 development board with USB-OTG support. Tested with the **ESP32-S3-DevKitC-1**.

The board has two USB-C ports:

| Port     | Purpose                              |
| -------- | ------------------------------------ |
| **UART** | Flashing firmware and serial monitor |
| **USB**  | USB HID keyboard output to target    |

The firmware uses:
- **GPIO 48** -- WS2812 addressable LED (standard on most ESP32-S3 dev boards)
- **GPIO 0** -- BOOT button (used for factory reset)

## Prerequisites

### 1. ESP-IDF (v6.x)

ESP-IDF is the official development framework for ESP32 chips.

```bash
# Clone ESP-IDF
mkdir -p ~/github/espressif
git clone --recursive https://github.com/espressif/esp-idf.git ~/github/espressif/esp-idf

# Install toolchain for ESP32-S3
~/github/espressif/esp-idf/install.sh esp32s3
```

The Taskfile expects ESP-IDF at `~/github/espressif/esp-idf` by default. Override with the `IDF_PATH` environment variable if installed elsewhere.

> You do **not** need to manually run `source export.sh` -- all `task fw-*` commands handle this automatically.

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

### 3. Task Runner

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

# Use ESP-IDF from a different location
IDF_PATH=/opt/esp-idf task fw-build
```

## Pairing

1. Flash the firmware to the ESP32-S3 (via the **UART** port)
2. Plug the ESP32-S3 into the **target device** via the **USB** port (it enumerates as a USB HID keyboard)
3. On your Mac, run `task ble-pair` (or `gostt-writer --ble-pair`)
4. Pairing uses ECDH P-256 key exchange over BLE -- the shared secret is saved to your config
5. Set `inject.method: ble` in `~/.config/gostt-writer/config.yaml`

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

The `task fw-*` commands source ESP-IDF automatically. If it still fails, verify `IDF_PATH`:
```bash
ls ~/github/espressif/esp-idf/export.sh
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
