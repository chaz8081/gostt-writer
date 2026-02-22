package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chaz8081/gostt-writer/internal/audio"
	"github.com/chaz8081/gostt-writer/internal/config"
	"github.com/chaz8081/gostt-writer/internal/hotkey"
	"github.com/chaz8081/gostt-writer/internal/inject"
	"github.com/chaz8081/gostt-writer/internal/transcribe"
)

func main() {
	// CLI flags
	configPath := flag.String("config", "", "path to config file (default: ~/.config/gostt-writer/config.yaml)")
	flag.Parse()

	// Load configuration
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("config validation: %v", err)
	}

	printBanner(cfg)

	// Initialize whisper transcriber
	log.Println("Loading whisper model...")
	modelStart := time.Now()
	transcriber, err := transcribe.NewTranscriber(cfg.ModelPath)
	if err != nil {
		log.Fatalf("Failed to load whisper model: %v\n\nCheck that the model file exists at: %s\nRun 'make model' to download it.", err, cfg.ModelPath)
	}
	log.Printf("Model loaded in %s", time.Since(modelStart).Round(time.Millisecond))

	// Initialize audio recorder
	recorder, err := audio.NewRecorder(cfg.Audio.SampleRate, cfg.Audio.Channels)
	if err != nil {
		transcriber.Close()
		log.Fatalf("Failed to initialize audio recorder: %v\n\nEnsure microphone access is granted in System Settings > Privacy & Security > Microphone.", err)
	}
	log.Println("Audio recorder ready")

	// Initialize text injector
	injector := inject.NewInjector(cfg.Inject.Method)
	log.Printf("Text injector ready (method: %s)", cfg.Inject.Method)

	// Initialize hotkey listener
	listener := hotkey.NewListener(cfg.Hotkey.Keys, cfg.Hotkey.Mode)
	log.Printf("Hotkey listener ready (%s, mode: %s)", strings.Join(cfg.Hotkey.Keys, "+"), cfg.Hotkey.Mode)

	// Signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start hotkey listener in background
	go listener.Start()

	log.Println("Ready! Press", strings.Join(cfg.Hotkey.Keys, "+"), "to dictate. Ctrl+C to quit.")

	// Main event loop
	events := listener.Events()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// Hotkey channel closed, listener stopped
				log.Println("Hotkey listener stopped")
				recorder.Close()
				transcriber.Close()
				return
			}

			switch ev.Type {
			case hotkey.EventStart:
				if err := recorder.Start(); err != nil {
					log.Printf("ERROR: failed to start recording: %v", err)
					continue
				}
				log.Println("Recording...")

			case hotkey.EventStop:
				samples := recorder.Stop()
				if samples == nil {
					continue
				}

				duration := float64(len(samples)) / float64(cfg.Audio.SampleRate)
				if duration < 0.3 {
					log.Printf("Recording too short (%.1fs), skipping", duration)
					continue
				}

				log.Printf("Captured %.1fs of audio, transcribing...", duration)

				// Async transcription and injection
				go func(samples []float32) {
					start := time.Now()
					text, err := transcriber.Process(samples)
					if err != nil {
						log.Printf("ERROR: transcription failed: %v", err)
						return
					}

					elapsed := time.Since(start).Round(time.Millisecond)

					if text == "" {
						log.Printf("No speech detected (%s)", elapsed)
						return
					}

					log.Printf("Transcribed in %s: %q", elapsed, text)

					if err := injector.Inject(text); err != nil {
						log.Printf("ERROR: text injection failed: %v", err)
						return
					}

					log.Println("Text injected")
				}(samples)
			}

		case sig := <-sigCh:
			log.Printf("Received %s, shutting down...", sig)
			// Stop recording if active
			if recorder.IsRecording() {
				recorder.Stop()
			}
			recorder.Close()
			transcriber.Close()
			log.Println("Goodbye!")
			// Exit directly to avoid gohook's C cleanup crash.
			// The OS reclaims the event hook on process exit.
			os.Exit(0)
		}
	}
}

// loadConfig loads the config from the specified path, or falls back to
// the default config path, or uses built-in defaults.
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
		log.Printf("Config loaded from %s", defaultPath)
		return cfg, nil
	}

	// No config file, use defaults
	log.Println("No config file found, using defaults")
	return config.Default(), nil
}

// printBanner displays the startup configuration summary.
func printBanner(cfg *config.Config) {
	fmt.Println("=== gostt-writer ===")
	fmt.Printf("  Model:   %s\n", cfg.ModelPath)
	fmt.Printf("  Hotkey:  %s (%s mode)\n", strings.Join(cfg.Hotkey.Keys, "+"), cfg.Hotkey.Mode)
	fmt.Printf("  Audio:   %dHz, %dch\n", cfg.Audio.SampleRate, cfg.Audio.Channels)
	fmt.Printf("  Inject:  %s\n", cfg.Inject.Method)
	fmt.Printf("  Log:     %s\n", cfg.LogLevel)
	fmt.Println("====================")
}
