// Command gostt-guess is a talking guessing game: the player thinks of
// something from a pack, the device asks yes-or-no questions out loud,
// listens to the spoken answers, and guesses.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/chaz8081/gostt-writer/guess/internal/config"
	"github.com/chaz8081/gostt-writer/guess/internal/game"
)

var version = "dev"

const defaultConfig = "/etc/gostt-guess/config.yaml"

const usage = `gostt-guess %s

  gostt-guess [flags]         play
  gostt-guess pack [file]     check a pack and show how many questions it takes
  gostt-guess voice [flags]   make the voice clips ahead of time, or say one line
  gostt-guess check [flags]   test the models, microphone, speaker and buttons

Flags for every command:
`

// common holds the flags every command takes.
type common struct {
	fs      *flag.FlagSet
	cfgPath string
	pack    string
	debug   bool
}

func newCommon(name string) *common {
	c := &common{fs: flag.NewFlagSet(name, flag.ExitOnError)}
	c.fs.StringVar(&c.cfgPath, "config", defaultConfig, "settings file")
	c.fs.StringVar(&c.pack, "pack", "", "pack file, instead of the one in the settings")
	c.fs.BoolVar(&c.debug, "debug", false, "log every decision")
	c.fs.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, version)
		c.fs.PrintDefaults()
	}
	return c
}

// load parses the flags and reads the settings and the pack.
func (c *common) load(args []string) (config.Config, *game.Pack, error) {
	_ = c.fs.Parse(args)
	level := slog.LevelInfo
	if c.debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	cfg, err := config.Load(c.cfgPath, c.cfgPath != defaultConfig)
	if err != nil {
		return cfg, nil, err
	}
	if c.pack != "" {
		cfg.Game.Pack = c.pack
	}
	pack, err := game.LoadPack(cfg.Game.Pack)
	return cfg, pack, err
}

func rules(cfg config.Config) game.Rules {
	return game.Rules{
		MaxQuestions: cfg.Game.MaxQuestions, MaxGuesses: cfg.Game.MaxGuesses,
		Confidence: cfg.Game.Confidence, Mistakes: cfg.Game.Mistakes,
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	args := os.Args[1:]
	cmd := runCmd
	if len(args) > 0 {
		switch args[0] {
		case "pack":
			cmd, args = packCmd, args[1:]
		case "voice":
			cmd, args = voiceCmd, args[1:]
		case "check":
			cmd, args = checkCmd, args[1:]
		case "version":
			fmt.Println(version)
			return
		}
	}
	err := cmd(ctx, args)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// packCmd checks a pack and plays every item against itself.
func packCmd(_ context.Context, args []string) error {
	c := newCommon("pack")
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		args = append([]string{"-pack", args[0]}, args[1:]...)
	}
	cfg, pack, err := c.load(args)
	if err != nil {
		return err
	}
	r := rules(cfg)
	total, worst, missed := 0, 0, 0
	for i, it := range pack.Items {
		guessed, asked := game.Simulate(pack, r, i, nil)
		total += asked
		worst = max(worst, asked)
		if len(guessed) != 1 {
			missed++
			fmt.Printf("  %s: not guessed first time (%d guesses, %d questions)\n", it.Name, len(guessed), asked)
		}
	}
	lied, found := 0, 0
	for i := range pack.Items {
		for lie := 1; lie <= 4; lie++ {
			guessed, _ := game.Simulate(pack, r, i, map[int]bool{lie: true})
			lied++
			if len(guessed) > 0 && guessed[len(guessed)-1] == i {
				found++
			}
		}
	}
	fmt.Printf("%s: %d items, %d questions\n", cfg.Game.Pack, len(pack.Items), len(pack.Questions))
	fmt.Printf("questions per round: %.1f on average, %d at most\n", float64(total)/float64(len(pack.Items)), worst)
	fmt.Printf("guessed first time: %d of %d\n", len(pack.Items)-missed, len(pack.Items))
	fmt.Printf("still found after one wrong answer: %d of %d rounds\n", found, lied)
	if u := pack.Unused(); len(u) > 0 {
		fmt.Printf("questions that are true for no item (check the spelling): %v\n", u)
	}
	return nil
}
