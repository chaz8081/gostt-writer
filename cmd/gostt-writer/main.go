package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chaz8081/gostt-writer/internal/audio"
	"github.com/chaz8081/gostt-writer/internal/ble"
	"github.com/chaz8081/gostt-writer/internal/config"
	"github.com/chaz8081/gostt-writer/internal/hotkey"
	"github.com/chaz8081/gostt-writer/internal/inject"
	"github.com/chaz8081/gostt-writer/internal/transcribe"
)

// version is set at build time via -ldflags.
var version = "dev"

const (
	minRecordingDuration = 0.5  // seconds
	maxRecordingDuration = 30.0 // seconds
)

func main() {
	// CLI flags
	configPath := flag.String("config", "", "path to config file (default: ~/.config/gostt-writer/config.yaml)")
	showVersion := flag.Bool("version", false, "print version and exit")
	blePair := flag.Bool("ble-pair", false, "scan and pair with an ESP32-S3 BLE device")
	flag.Parse()

	if *showVersion {
		fmt.Printf("gostt-writer %s\n", version)
		return
	}

	if *blePair {
		runBLEPairing()
		return
	}

	// Load configuration
	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "config validation: %v\n", err)
		os.Exit(1)
	}

	// Set up structured logging
	logLevel := config.ParseLogLevel(cfg.LogLevel)
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	printBanner(cfg)

	// Initialize transcriber
	slog.Info("Loading transcription model...", "backend", cfg.Transcribe.Backend)
	modelStart := time.Now()
	transcriber, err := transcribe.New(&cfg.Transcribe)
	if err != nil {
		slog.Error("Failed to load transcription model",
			"error", err,
			"backend", cfg.Transcribe.Backend,
			"hint", "Run 'task whisper-model' or 'task parakeet-model' to download models")
		os.Exit(1)
	}
	slog.Info("Model loaded", "backend", cfg.Transcribe.Backend, "elapsed", time.Since(modelStart).Round(time.Millisecond))

	// Initialize audio recorder
	recorder, err := audio.NewRecorder(cfg.Audio.SampleRate, cfg.Audio.Channels)
	if err != nil {
		if err := transcriber.Close(); err != nil {
			slog.Error("failed to close transcriber", "error", err)
		}
		slog.Error("Failed to initialize audio recorder",
			"error", err,
			"hint", "Ensure microphone access is granted in System Settings > Privacy & Security > Microphone")
		os.Exit(1)
	}
	slog.Info("Audio recorder ready")

	// Initialize text injector
	var injector inject.TextInjector
	switch cfg.Inject.Method {
	case "ble":
		key, err := hex.DecodeString(cfg.Inject.BLE.SharedSecret)
		if err != nil {
			slog.Error("Invalid BLE shared secret", "error", err)
			os.Exit(1)
		}
		bleAdapter := ble.NewCoreBluetoothAdapter()
		bleClient, err := ble.NewClient(bleAdapter, cfg.Inject.BLE.DeviceMAC, key, ble.ClientOptions{
			QueueSize:    cfg.Inject.BLE.QueueSize,
			ReconnectMax: cfg.Inject.BLE.ReconnectMax,
		})
		if err != nil {
			slog.Error("Invalid BLE configuration", "error", err)
			os.Exit(1)
		}
		if err := bleClient.Connect(); err != nil {
			slog.Error("BLE connection failed", "error", err,
				"hint", "Ensure ESP32-S3 is powered on and in range. Re-pair with: task ble-pair")
			os.Exit(1)
		}
		injector = inject.NewBLEInjector(bleClient)
		slog.Info("Text injector ready", "method", "ble", "device", cfg.Inject.BLE.DeviceMAC)
	default:
		injector = inject.NewInjector(cfg.Inject.Method)
		slog.Info("Text injector ready", "method", cfg.Inject.Method)
	}

	// Initialize hotkey listener
	listener := hotkey.NewListener(cfg.Hotkey.Keys, cfg.Hotkey.Mode)
	slog.Info("Hotkey listener ready",
		"keys", strings.Join(cfg.Hotkey.Keys, "+"),
		"mode", cfg.Hotkey.Mode)

	// Signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start hotkey listener in background
	go listener.Start()

	slog.Info("Ready! Press " + strings.Join(cfg.Hotkey.Keys, "+") + " to dictate. Ctrl+C to quit.")

	// Main event loop
	events := listener.Events()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// Hotkey channel closed, listener stopped
				slog.Info("Hotkey listener stopped")
				if err := recorder.Close(); err != nil {
					slog.Error("failed to close recorder", "error", err)
				}
				if err := transcriber.Close(); err != nil {
					slog.Error("failed to close transcriber", "error", err)
				}
				return
			}

			switch ev.Type {
			case hotkey.EventStart:
				if err := recorder.Start(); err != nil {
					slog.Error("Failed to start recording", "error", err)
					continue
				}
				slog.Info("Recording...")

			case hotkey.EventStop:
				samples := recorder.Stop()
				if samples == nil {
					continue
				}

				duration := float64(len(samples)) / float64(cfg.Audio.SampleRate)

				if duration < minRecordingDuration {
					slog.Info("Recording too short, skipping",
						"duration_s", fmt.Sprintf("%.1f", duration),
						"min_s", minRecordingDuration)
					continue
				}

				if duration > maxRecordingDuration {
					slog.Warn("Recording exceeds max duration, truncating",
						"duration_s", fmt.Sprintf("%.1f", duration),
						"max_s", maxRecordingDuration)
					maxSamples := int(maxRecordingDuration * float64(cfg.Audio.SampleRate))
					samples = samples[:maxSamples]
					duration = maxRecordingDuration
				}

				slog.Info("Captured audio, transcribing...",
					"duration_s", fmt.Sprintf("%.1f", duration))

				// Async transcription and injection
				go func(samples []float32) {
					start := time.Now()
					text, err := transcriber.Process(samples)
					if err != nil {
						slog.Error("Transcription failed", "error", err)
						return
					}

					elapsed := time.Since(start).Round(time.Millisecond)

					if text == "" {
						slog.Info("No speech detected", "elapsed", elapsed)
						return
					}

					slog.Info("Transcribed", "elapsed", elapsed, "text", text)

					if err := injector.Inject(text); err != nil {
						slog.Error("Text injection failed", "error", err)
						return
					}

					slog.Info("Text injected")
				}(samples)
			}

		case sig := <-sigCh:
			slog.Info("Shutting down...", "signal", sig)
			// Stop recording if active
			if recorder.IsRecording() {
				recorder.Stop()
			}
			if err := recorder.Close(); err != nil {
				slog.Error("failed to close recorder", "error", err)
			}
			if err := transcriber.Close(); err != nil {
				slog.Error("failed to close transcriber", "error", err)
			}
			if closer, ok := injector.(interface{ Close() error }); ok {
				if err := closer.Close(); err != nil {
					slog.Error("failed to close injector", "error", err)
				}
			}
			slog.Info("Goodbye!")
			// Exit directly to avoid gohook's C cleanup crash.
			// The OS reclaims the event hook on process exit.
			os.Exit(0)
		}
	}
}

// loadConfig loads the config from the specified path, or falls back to
// the default config path, or uses built-in defaults. On first run,
// it writes a default config file.
func loadConfig(path string) (*config.Config, error) {
	if path != "" {
		return config.Load(path)
	}

	// Try default config path
	defaultPath := config.DefaultConfigPath()
	if _, err := os.Stat(defaultPath); err == nil {
		cfg, err := config.Load(defaultPath)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", defaultPath, err)
		}
		slog.Info("Config loaded", "path", defaultPath)
		return cfg, nil
	}

	// No config file found; create default for next time
	if created, err := config.WriteDefault(); err != nil {
		slog.Warn("Could not write default config", "error", err)
	} else if created != "" {
		slog.Info("Created default config", "path", created)
	}

	return config.Default(), nil
}

// printBanner displays the startup configuration summary.
func printBanner(cfg *config.Config) {
	fmt.Println("=== gostt-writer ===")
	fmt.Printf("  Version: %s\n", version)
	fmt.Printf("  Backend: %s\n", cfg.Transcribe.Backend)
	switch cfg.Transcribe.Backend {
	case "parakeet":
		fmt.Printf("  Model:   %s\n", cfg.Transcribe.ParakeetModelDir)
	default:
		fmt.Printf("  Model:   %s\n", cfg.Transcribe.ModelPath)
	}
	fmt.Printf("  Hotkey:  %s (%s mode)\n", strings.Join(cfg.Hotkey.Keys, "+"), cfg.Hotkey.Mode)
	fmt.Printf("  Audio:   %dHz, %dch\n", cfg.Audio.SampleRate, cfg.Audio.Channels)
	fmt.Printf("  Inject:  %s\n", cfg.Inject.Method)
	fmt.Printf("  Log:     %s\n", cfg.LogLevel)
	fmt.Println("====================")
}

// runBLEPairing scans for ESP32-S3 devices and performs ECDH key exchange.
func runBLEPairing() {
	fmt.Println("=== BLE Pairing ===")

	adapter := ble.NewCoreBluetoothAdapter()

	fmt.Println("Scanning for ESP32-S3 devices (5 seconds)...")
	devices, err := ble.ScanForDevices(adapter, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scan failed: %v\n", err)
		os.Exit(1)
	}

	if len(devices) == 0 {
		fmt.Println("No devices found. Make sure your ESP32-S3 is powered on and in range.")
		os.Exit(1)
	}

	fmt.Printf("Found %d device(s):\n", len(devices))
	for i, d := range devices {
		fmt.Printf("  [%d] %s (%s) RSSI: %d\n", i+1, d.Name, d.MAC, d.RSSI)
	}

	// Use the first device (TODO: prompt user to pick when multiple)
	target := devices[0]
	fmt.Printf("\nPairing with %s (%s)...\n", target.Name, target.MAC)

	result, err := ble.Pair(adapter, target.MAC, ble.DefaultPairOptions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Pairing failed: %v\n", err)
		os.Exit(1)
	}

	secretHex := hex.EncodeToString(result.SharedSecret)
	fmt.Println("\nPairing successful!")
	fmt.Printf("  Device MAC:    %s\n", result.DeviceMAC)
	fmt.Printf("  Shared Secret: %s\n", secretHex)
	fmt.Println("\nAdd to your config (~/.config/gostt-writer/config.yaml):")
	fmt.Println("  inject:")
	fmt.Println("    method: ble")
	fmt.Println("    ble:")
	fmt.Printf("      device_mac: %q\n", result.DeviceMAC)
	fmt.Printf("      shared_secret: %q\n", secretHex)
}
