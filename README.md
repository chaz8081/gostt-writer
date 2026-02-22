# gostt-writer

Local real-time dictation for macOS. Press a hotkey, speak, and your words are typed into the active application. All processing happens on-device -- nothing is sent to the cloud. Choose between [whisper.cpp](https://github.com/ggerganov/whisper.cpp) (default) or [Parakeet TDT 0.6B v2](https://huggingface.co/FluidInference/parakeet-tdt-0.6b-v2-coreml) via CoreML for Apple Neural Engine acceleration.

## Prerequisites

- macOS on Apple Silicon (M1 or later)
- [Go 1.21+](https://go.dev/dl/)
- [Task](https://taskfile.dev/) (`brew install go-task`)
- [CMake](https://cmake.org/) (`brew install cmake`)
- Xcode Command Line Tools (`xcode-select --install`)

## Build & Install

Clone the repo and install:

```bash
git clone --recurse-submodules https://github.com/chaz8081/gostt-writer.git
cd gostt-writer
task install
```

On macOS, the installer prompts you to choose a transcription backend (whisper, parakeet, or both). On other platforms it defaults to whisper. The binary is installed to `/usr/local/bin` by default -- override with `task install INSTALL_DIR=~/bin`.

If you already cloned without `--recurse-submodules`:

```bash
git submodule update --init --recursive
task install
```

To build without installing (binary stays in `bin/`):

```bash
task
```

## macOS Permissions

gostt-writer needs two permissions to function:

**Accessibility** (for global hotkey and keystroke injection):

1. Open **System Settings > Privacy & Security > Accessibility**
2. Click **+** and add your **terminal app** (Terminal.app, iTerm2, Ghostty, etc.)
3. Toggle it on

**Microphone** (for audio capture):

1. Open **System Settings > Privacy & Security > Microphone**
2. Add your terminal app if it isn't already listed
3. Toggle it on

You may need to restart your terminal after granting permissions.

## Quick Start

```bash
gostt-writer
```

Or if running from the repo without installing:

```bash
task run
```

Then press `Ctrl+Shift+R`, speak, and release. Your words will be typed into whatever application has focus.

Press `Ctrl+C` to quit.

## Running in the Background with tmux

gostt-writer runs in the foreground by default. If you want it running persistently (surviving terminal closes, SSH disconnects, etc.), tmux is the simplest approach.

### Install tmux

```bash
brew install tmux
```

### Start gostt-writer in a tmux session

```bash
tmux new-session -d -s gostt 'cd /path/to/gostt-writer && ./bin/gostt-writer'
```

This starts gostt-writer in a detached tmux session named "gostt". It runs in the background immediately -- you can close your terminal and it keeps running.

Replace `/path/to/gostt-writer` with the actual path to your clone.

### View logs (attach to the session)

```bash
tmux attach -t gostt
```

You'll see the live log output. To detach without stopping gostt-writer, press `Ctrl+B` then `D`.

### Stop gostt-writer

Attach to the session and press `Ctrl+C`:

```bash
tmux attach -t gostt
# Now press Ctrl+C to stop
```

Or kill the session from outside:

```bash
tmux kill-session -t gostt
```

### Check if it's running

```bash
tmux has-session -t gostt 2>/dev/null && echo "running" || echo "not running"
```

### Restart after a rebuild

```bash
tmux kill-session -t gostt 2>/dev/null
task build
tmux new-session -d -s gostt 'cd /path/to/gostt-writer && ./bin/gostt-writer'
```

### tmux cheat sheet (just the basics)

| Action                  | Command                              |
| ----------------------- | ------------------------------------ |
| Start a new session     | `tmux new-session -d -s gostt '...'` |
| Attach to session       | `tmux attach -t gostt`               |
| Detach (while attached) | `Ctrl+B`, then `D`                   |
| List sessions           | `tmux ls`                            |
| Kill session            | `tmux kill-session -t gostt`         |

## Configuration

On first run, gostt-writer creates a default config at `~/.config/gostt-writer/config.yaml`. You can also specify a custom path:

```bash
./bin/gostt-writer --config /path/to/config.yaml
```

See [`config.example.yaml`](config.example.yaml) for all options with documentation. The key settings:

| Setting                         | Default                   | Description                                           |
| ------------------------------- | ------------------------- | ----------------------------------------------------- |
| `transcribe.backend`            | `whisper`                 | `whisper` or `parakeet`                               |
| `transcribe.model_path`         | `models/ggml-base.en.bin` | Path to whisper model                                 |
| `transcribe.parakeet_model_dir` | `models/parakeet-tdt-v2`  | Path to Parakeet CoreML models                        |
| `hotkey.keys`                   | `["ctrl", "shift", "r"]`  | Key combination                                       |
| `hotkey.mode`                   | `hold`                    | `hold` = push-to-talk, `toggle` = press to start/stop |
| `inject.method`                 | `type`                    | `type` = keystrokes, `paste` = clipboard + Cmd+V      |
| `log_level`                     | `info`                    | `debug`, `info`, `warn`, or `error`                   |

## How It Works

1. A global hotkey listener waits for your configured key combo
2. On press, audio is captured from your default microphone at 16kHz mono
3. On release, the audio is sent for local transcription (Metal-accelerated whisper or CoreML Neural Engine Parakeet)
4. The transcribed text is injected into the active application via keystroke simulation

Transcription happens asynchronously -- you can start speaking again while the previous result is being typed.

## Privacy

gostt-writer is designed to be fully offline. After installation, the application makes **zero network connections**. No audio, transcription results, configuration, or telemetry data ever leaves your machine.

### What we guarantee

- **No network access at runtime.** The application does not import Go's `net/http` or any networking package. There are no outbound connections, no DNS lookups, no listening sockets.
- **No telemetry or analytics.** No usage data, crash reports, or diagnostics are collected or transmitted.
- **Audio stays in memory.** Captured audio is held in RAM only, processed locally, and discarded. It is never written to disk or sent anywhere.
- **Minimal filesystem footprint.** The app reads its config from `~/.config/gostt-writer/config.yaml` and its models from the configured model directory. It writes only to the config directory (to create a default config on first run). Nothing else.
- **No environment variable harvesting.** The application does not read environment variables at runtime.
- **Dependencies are clean.** All third-party libraries (malgo, whisper.cpp, robotgo, gohook, yaml.v3) have been audited. None contain telemetry, analytics, or networking code. The whisper.cpp submodule includes an optional RPC backend (`ggml-rpc`) but it is **not compiled** -- the build explicitly excludes it.

### When the network is used

The **only** network access occurs during setup, when you explicitly download models:

- `task models` downloads whisper and/or Parakeet models from [HuggingFace](https://huggingface.co)
- This is manual and user-initiated -- it never happens automatically

After models are downloaded, you can run gostt-writer with no internet connection.

### Verifying this yourself

You can confirm the offline guarantee:

- **Block the binary with your firewall** (Little Snitch, Lulu, or macOS Application Firewall) -- gostt-writer will function identically with all network access blocked.
- **Search the source code** -- `grep -r 'net/http\|net.Dial\|http.Get\|http.Post' internal/ cmd/` returns zero results in application code.
- **Monitor with `nettop`** -- run `nettop -p $(pgrep gostt-writer)` while using the app. You will see zero network activity.

### One caveat: macOS system-level telemetry

Apple's CoreML and Metal frameworks may send anonymous performance data as part of macOS system-level analytics. This is controlled by **System Settings > Privacy & Security > Analytics & Improvements** and applies to any application using these frameworks. gostt-writer does not initiate or control this behavior.

## Backends

### Whisper (default)

[whisper.cpp](https://github.com/ggerganov/whisper.cpp) via Go bindings. Runs on CPU/GPU with Metal acceleration. Achieves ~26x real-time on M4 Max with the base.en model.

No extra setup needed -- `task` builds whisper.cpp and downloads the model automatically.

### Parakeet TDT (optional)

[Parakeet TDT 0.6B v2](https://huggingface.co/FluidInference/parakeet-tdt-0.6b-v2-coreml) via CoreML. Runs on the Apple Neural Engine. Achieves ~110x real-time on M4 Max with lower word error rate than whisper base.en.

To use Parakeet:

1. Download the CoreML models:
   ```bash
   task models
   ```
   Choose option **2** (parakeet) or **3** (both).
2. Switch the backend:
   ```bash
   task backend
   ```
   Or edit your config manually (`~/.config/gostt-writer/config.yaml`):
   ```yaml
   transcribe:
     backend: parakeet
   ```

## Tasks

Run `task --list` to see all available tasks:

| Task             | Description                                              |
| ---------------- | -------------------------------------------------------- |
| `task`           | Build everything (whisper.cpp + whisper model + binary)  |
| `task install`   | Build, download models, and install to /usr/local/bin    |
| `task models`    | Download transcription models (interactive)              |
| `task backend`   | Switch the active transcription backend in your config   |
| `task build`     | Build the gostt-writer binary                            |
| `task run`       | Build and run gostt-writer                               |
| `task test`      | Run all tests                                            |
| `task whisper`   | Build whisper.cpp static library (Metal + Accelerate)    |
| `task clean`     | Remove build artifacts                                   |

## Version

```bash
gostt-writer --version
```
