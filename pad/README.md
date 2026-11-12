# gostt-pad

A standalone dictation macro pad. A Raspberry Pi listens to a microphone,
transcribes speech on the device, and types the words into the computer it is
plugged into as a USB keyboard. Nothing is installed on the computer and
nothing leaves the Pi.

Status: the software is tested on a Linux PC with real audio and the real
speech model, and the ARM64 build is checked under emulation. **It has not
yet run on a Raspberry Pi.** The parts that need hardware (microphone
capture, the USB keyboard gadget, the buttons) are listed under
[What has and has not been tested](#what-has-and-has-not-been-tested).

## What it does

| Button | Action |
|---|---|
| Talk | Short tap: dictation on, tap again for off. Long press: dictation only while held. |
| Undo | Deletes the last phrase the pad typed, exactly. |
| Delete word | Sends Option+Backspace. |
| New line | Sends Enter. |
| Cancel | Stops dictation and throws away what has not been typed yet. |

- Words are typed once they are stable, about 1.5 to 3 seconds behind
  speech. Text already typed is never rewritten.
- A pause of about 3 seconds starts a new phrase, which is what one press of
  Undo removes. Shorter pauses may or may not split, because the model
  delivers words in bursts. New lines can be undone too.
- After Delete word the pad no longer knows how much text is on screen, so
  Undo does nothing until you dictate again.
- The LED is lit while the pad is listening.
- Shortcuts are for macOS with a US keyboard layout.

## Hardware

- **Raspberry Pi 4, 5 or Zero 2 W** with **64-bit** Raspberry Pi OS (Lite is
  fine). The original Zero and Zero W cannot run it.
- **Microphone.** On a Pi 4 or 5, any USB microphone. On a Zero 2 W the only
  data USB port is used for the keyboard connection, so use an I2S
  microphone (see [I2S microphone](#i2s-microphone-zero-2-w)).
- **A USB cable to the computer**, from the Pi's USB-C port (Pi 4 and 5) or
  the micro-USB port marked USB, not PWR (Zero 2 W). This cable also powers
  the Pi.
- **Switches and an LED** (optional for first tests).

Because the USB cable to the computer is in use, reach the Pi over Wi-Fi
with SSH. Set up Wi-Fi and SSH in Raspberry Pi Imager before the first boot.

### Wiring

Each switch connects a GPIO pin to any ground pin. No resistors are needed;
the pad enables the Pi's internal pull-ups.

| Button | GPIO | Header pin | A nearby ground pin |
|---|---|---|---|
| Talk | 17 | 11 | 9 |
| Undo | 27 | 13 | 14 |
| Delete word | 22 | 15 | 14 |
| New line | 23 | 16 | 20 |
| Cancel | 24 | 18 | 20 |
| LED (through a 330 ohm resistor to ground) | 25 | 22 | 25 |

Pins can be changed in the settings file. Set a pin to `-1` to leave it out.

## Install

On the Pi:

```bash
sudo apt-get update && sudo apt-get install -y git
git clone -b claude/esp32-s3-voice-to-text-oliz70 https://github.com/chaz8081/gostt-writer.git
cd gostt-writer/pad
./deploy/install.sh
sudo reboot
```

The installer adds system packages, Go, the speech model (55 MB), the
program in `/opt/gostt-pad`, the settings file `/etc/gostt-pad/config.yaml`,
the USB keyboard setup, and a service that is installed but not started. It
adds one line to `config.txt` to put the USB port in device mode. It is safe
to run again.

On a Zero 2 W the build takes several minutes. If it runs out of memory,
run the installer on a Pi 4 or 5 instead and copy `/opt/gostt-pad`,
`/etc/gostt-pad`, `/usr/local/bin/gostt-pad` and the two
`/etc/systemd/system/gostt-pad*.service` files across; all three boards run
the same build.

## Test it, one part at a time

Do these in order. Each step tests one more thing, so a failure points at
that thing. Steps 1 to 3 need no USB connection to the computer and no
buttons.

### 1. Speed

```bash
gostt-pad bench ~/gostt-writer/internal/transcribe/testdata/*.wav
```

Expect the three sentences printed, with at most a word or two wrong, and a
last line reading `overall speed ...: fast enough for live dictation`. The
speed must be below 1.0. On a Zero 2 W, if it is above 0.7, try
`-threads 3`, or set `decoding: greedy_search` in the settings file.

### 2. Microphone

```bash
arecord -l                        # find your microphone
sudoedit /etc/gostt-pad/config.yaml   # if it is not the default device
gostt-pad check -dry-run -keys none
```

Speak while it records for three seconds. Expect
`ok    microphone level ...`. For a USB microphone the device is usually
`plughw:CARD=<name from arecord -l>,DEV=0` in place of `default` in
`audio.command`. If the check says the level is very quiet, raise
`audio.gain` as it suggests.

### 3. Dictation, shown on screen only

```bash
gostt-pad -dry-run -keys stdin
```

Type `t` and Enter to start, speak a sentence or two, and watch lines
beginning `screen:` grow as words are recognised. Then try:

| Type | Expect |
|---|---|
| `t` | `listening: false`, and the rest of what you said appears |
| `u` | the last phrase disappears |
| `n` | a `\n` is added |
| `h`, speak, `r` | hold-to-talk: text appears after `r` |
| `t`, speak, `c` | cancel: nothing more is typed |
| `q` | quit |

### 4. USB keyboard

Connect the Pi's USB-C (or the Zero's USB) port to the Mac. The Pi will
restart if it was powered from somewhere else. The Mac may open the Keyboard
Setup Assistant the first time; close it or choose ANSI.

```bash
gostt-pad check -keys none
```

Expect `ok    keyboard   /dev/hidg0 accepted a report`. Then click into a
text editor on the Mac and run:

```bash
gostt-pad -keys stdin
```

Use `t`, `u`, `n`, `w` as in step 3. This time the text appears on the Mac.
If characters are dropped or doubled, raise `hid.report_delay_ms` to 5 or 10.

### 5. Buttons

```bash
gostt-pad check -buttons 20s
```

Press each button once within 20 seconds. Expect a line per press and
`ok    buttons    all 5 buttons respond`. The LED lights while a button is
held. Then run the pad for real:

```bash
gostt-pad
```

### 6. Start at boot

Once the steps above work:

```bash
sudo systemctl enable --now gostt-pad
journalctl -u gostt-pad -f        # watch its log
```

Stop the service (`sudo systemctl stop gostt-pad`) before running
`gostt-pad` by hand again; only one copy can use the microphone and buttons.

## If something fails

| Symptom | Likely cause |
|---|---|
| `check` fails on keyboard: no such file `/dev/hidg0` | Not rebooted since installing, or `systemctl status gostt-pad-gadget` shows an error |
| `check` fails on keyboard: timeout | The cable is in the wrong port, is charge-only, or the computer is asleep |
| `check` fails on buttons: permission denied | Log out and in again so the new group membership applies |
| Microphone: `arecord: not found` or `No such device` | Wrong `-D` device in `audio.command`; see `arecord -l` |
| Words appear but trail far behind | Speed too close to 1.0; see step 1 |
| Last word of a sentence missing | Raise `model.tail_padding_ms` |
| Pi 5 restarts or shows a low-voltage warning | The computer's port supplies too little power; use a powered port or a Pi 4 |

Add `-debug` to any command to log every decision the pad makes.

## I2S microphone (Zero 2 W)

Not yet tested. An INMP441 wires as: VDD to 3V3 (pin 1), GND to ground
(pin 6), SCK to GPIO 18 (pin 12), WS to GPIO 19 (pin 35), SD to GPIO 20
(pin 38), L/R to ground. Add `dtoverlay=googlevoicehat-soundcard` to
`/boot/firmware/config.txt`, reboot, find the card with `arecord -l`, and
set:

```yaml
audio:
  command: "arecord -q -D plughw:CARD=sndrpigooglevoi,DEV=0 -f S16_LE -r 16000 -c 1 -t raw"
  gain: 8
```

These microphones are quiet; `gostt-pad check` reports the level and
suggests a gain. The default button pins do not overlap the I2S pins.

## What has and has not been tested

Tested on a Linux PC:

- The unit tests (`go test ./...`), covering the talk key, typing and undo
  logic, capitalisation, settings, audio decoding, and the stalled-host and
  unplugged-cable cases.
- The whole pipeline against the real model with the repository's benchmark
  recordings: toggle and hold dictation, undo by phrase, cancel, and silence
  typing nothing (`GOSTT_PAD_MODEL_DIR=<model folder> go test ./internal/asr/`).
- The `gostt-pad` program itself, fed a recording at speaking speed with
  scripted key presses, in `-dry-run` mode.
- The installer's download, settings and file-placement steps, into a
  scratch directory.
- An ARM64 build, run under QEMU emulating the Cortex-A53, A72 and A76 (the
  CPUs of the Zero 2 W, Pi 4 and Pi 5). It transcribes correctly. Emulated
  speed says nothing about real speed.

Not tested, because it needs the hardware:

- Speed on a real Pi. Step 1 measures it.
- Microphone capture through `arecord`, and the I2S microphone setup.
- The USB keyboard gadget script and typing into a real Mac.
- The GPIO buttons and LED.
- Building on the Pi with the installer's `go` and `build` steps.

## Not built yet

- Modes (prose, code, commands) and per-mode word lists.
- A screen showing words before they are typed.
- Layouts other than US, and hosts other than macOS.

## Layout

| Path | Contents |
|---|---|
| `cmd/gostt-pad` | The program: `run`, `check`, `bench` |
| `internal/core` | Talk key, typing and undo, keyboard reports, controller. No hardware. |
| `internal/asr` | The speech model, through sherpa-onnx |
| `internal/audio` | Microphone capture command, WAV reading |
| `internal/gpio` | Buttons and LED |
| `internal/config` | Settings file |
| `deploy` | Installer, USB gadget script, service files |

This is its own Go module so it builds on a Pi without the macOS app's
dependencies. The speech model is Kroko's English streaming Zipformer
(CC-BY-SA 4.0, by Banafo), run with sherpa-onnx (Apache-2.0).
