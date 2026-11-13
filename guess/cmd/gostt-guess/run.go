package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/chaz8081/gostt-writer/guess/internal/audio"
	"github.com/chaz8081/gostt-writer/guess/internal/config"
	"github.com/chaz8081/gostt-writer/guess/internal/game"
	"github.com/chaz8081/gostt-writer/guess/internal/gpio"
	"github.com/chaz8081/gostt-writer/guess/internal/session"
	"github.com/chaz8081/gostt-writer/guess/internal/speech"
)

// textIO plays the game on the terminal: lines are printed and replies are
// typed. It needs no models, microphone or speaker.
type textIO struct {
	in *bufio.Reader
}

func (t *textIO) Say(ctx context.Context, text string) error {
	fmt.Println("device:", text)
	return ctx.Err()
}

func (t *textIO) Hear(ctx context.Context, _ time.Duration) (string, error) {
	fmt.Print("you: ")
	line, err := t.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
		return "", io.EOF
	}
	return strings.TrimSpace(line), ctx.Err()
}

func (t *textIO) Listening(bool) {}

// runCmd plays the game.
func runCmd(ctx context.Context, args []string) error {
	c := newCommon("gostt-guess")
	text := c.fs.Bool("text", false, "play on the terminal by typing, without models or audio")
	buttons := c.fs.Bool("buttons", true, "use the buttons and light; without them a game starts at once")
	cfg, pack, err := c.load(args)
	if err != nil {
		return err
	}
	collectionPath := cfg.Game.Collection
	if *text {
		collectionPath = "" // trying a pack should not touch the real collection
	}
	collection, err := game.OpenCollection(collectionPath)
	if err != nil {
		return err
	}
	opt := session.Options{
		Pack: pack, Rules: rules(cfg), Collection: collection,
		Wait: time.Duration(cfg.Listen.WaitMs) * time.Millisecond, Tries: cfg.Listen.Tries,
	}
	if *text {
		opt.StartNow, opt.ExitWhenIdle = true, true
		fmt.Println("Type yes, no, or don't know. Also: repeat, start over, stop.")
		return session.New(&textIO{in: bufio.NewReader(os.Stdin)}, opt).Run(ctx)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dev, closeDev, err := openDevice(ctx, cancel, cfg)
	if err != nil {
		return err
	}
	defer closeDev()
	if *buttons {
		presses := make(chan game.Reply, 4)
		byName := map[string]game.Reply{"yes": game.Yes, "no": game.No, "not_sure": game.DontKnow, "play": game.Restart}
		panel, err := gpio.Open(cfg.GPIO.Chip, cfg.GPIO.Buttons(), cfg.GPIO.Light,
			time.Duration(cfg.GPIO.DebounceMs)*time.Millisecond, func(name string) {
				select {
				case presses <- byName[name]:
				default: // nobody is waiting for this many presses
				}
			})
		if err != nil {
			return err
		}
		defer panel.Close()
		dev.Light = panel.SetLight
		opt.Buttons = presses
	} else {
		opt.StartNow, opt.ExitWhenIdle = true, true
	}
	slog.Info("ready", "pack", pack.Name, "items", len(pack.Items), "found", collection.Count(pack))
	return session.New(dev, opt).Run(ctx)
}

// openDevice loads the models and starts the microphone. A microphone that
// stops ends the program through cancel, so the service restarts it.
func openDevice(ctx context.Context, cancel context.CancelFunc, cfg config.Config) (*speech.Device, func(), error) {
	asr, err := speech.NewASR(speech.ASRConfig{
		Dir: cfg.Hear.Dir, Threads: cfg.Hear.Threads, Decoding: cfg.Hear.Decoding,
		TailPadding: time.Duration(cfg.Hear.TailPaddingMs) * time.Millisecond,
	})
	if err != nil {
		return nil, nil, err
	}
	voice, err := speech.NewVoice(speech.VoiceConfig{Dir: cfg.Voice.Dir, Threads: cfg.Voice.Threads, Speed: cfg.Voice.Speed, Cache: cfg.Voice.Cache})
	if err != nil {
		asr.Close()
		return nil, nil, err
	}
	mic := startMic(ctx, cancel, cfg)
	dev := &speech.Device{
		Voice: voice, Play: cfg.Audio.Play, Volume: cfg.Audio.Volume,
		Listener: &speech.Listener{
			Rec: asr, Mic: mic, Level: cfg.Listen.Level,
			Quiet:     time.Duration(cfg.Listen.QuietMs) * time.Millisecond,
			MaxSpeech: time.Duration(cfg.Listen.MaxSpeechMs) * time.Millisecond,
			Skip:      time.Duration(cfg.Listen.SkipMs) * time.Millisecond,
			Enough:    func(text string) bool { return game.ParseReply(text) != game.Unclear },
		},
	}
	return dev, func() { voice.Close(); asr.Close() }, nil
}

// startMic runs the capture command and returns its audio. The microphone
// runs all the time; when nobody is listening the oldest audio is dropped,
// so the capture command is never held up.
func startMic(ctx context.Context, cancel context.CancelFunc, cfg config.Config) <-chan []float32 {
	raw := make(chan []float32)
	mic := make(chan []float32, 20)
	go func() {
		if err := audio.RunCommand(ctx, cfg.Audio.Capture, cfg.Audio.Gain, raw); err != nil {
			slog.Error("microphone stopped", "error", err)
		}
		cancel()
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case block := <-raw:
				select {
				case mic <- block:
				default:
					select {
					case <-mic:
					default:
					}
					mic <- block
				}
			}
		}
	}()
	return mic
}
