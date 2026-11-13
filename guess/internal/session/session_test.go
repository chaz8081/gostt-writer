package session

import (
	"context"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chaz8081/gostt-writer/guess/internal/game"
)

const pack = `
thing: an animal
questions:
  - {id: flies, ask: "Can it fly?"}
  - {id: swims, ask: "Does it live in water?"}
  - {id: big, ask: "Is it big?"}
  - {id: pet, ask: "Is it a pet?"}
items:
  - {name: Eagle, say: an eagle, yes: [flies]}
  - {name: Shark, say: a shark, yes: [swims, big]}
  - {name: Goldfish, say: a goldfish, yes: [swims, pet]}
  - {name: Horse, say: a horse, yes: [big]}
  - {name: Cat, say: a cat, yes: [pet]}
  - {name: Parrot, say: a parrot, yes: [flies, pet]}
`

// script is a fake player. It answers each line the device speaks using
// reply; "" means silence.
type script struct {
	t       *testing.T
	said    []string
	reply   func(question string) string
	onSay   func(line string)
	last    string
	lit     bool
	litHear bool
}

func (s *script) Say(ctx context.Context, text string) error {
	if s.onSay != nil {
		s.onSay(text)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.said = append(s.said, text)
	s.last = text
	return nil
}

func (s *script) Hear(ctx context.Context, wait time.Duration) (string, error) {
	s.litHear = s.lit
	return s.reply(s.last), ctx.Err()
}

func (s *script) Listening(on bool) { s.lit = on }

// truthful answers for target from the pack text above.
func truthful(target string, facts map[string]string) func(string) string {
	return func(q string) string {
		switch {
		case strings.HasPrefix(q, "I think I know"):
			if strings.Contains(q, strings.ToLower(target)) {
				return "yes"
			}
			return "no"
		case q == lineAgain:
			return "no"
		}
		if a, ok := facts[q]; ok {
			return a
		}
		return "no"
	}
}

func newSession(t *testing.T, io IO, buttons <-chan game.Reply) (*Session, *game.Collection) {
	t.Helper()
	p, err := game.ParsePack([]byte(pack))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := game.OpenCollection("")
	return New(io, Options{
		Pack: p, Rules: game.DefaultRules(), Collection: c, Buttons: buttons,
		StartNow: true, ExitWhenIdle: true, Rand: rand.New(rand.NewPCG(1, 2)),
	}), c
}

func TestRoundGuessesAndCollects(t *testing.T) {
	io := &script{t: t, reply: truthful("Parrot", map[string]string{"Can it fly?": "yeah", "Is it a pet?": "uh huh"})}
	s, c := newSession(t, io, nil)
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{lineRules, "I think I know. Is it a parrot?", lineWin, lineNew, lineCount(1, 6), lineAgain, lineBye} {
		if !slices.Contains(io.said, want) {
			t.Errorf("never said %q\nsaid: %q", want, io.said)
		}
	}
	if !c.Found["Parrot"] {
		t.Error("Parrot not in collection")
	}
	if !io.litHear {
		t.Error("light was off while listening")
	}
	if io.lit {
		t.Error("light left on")
	}
}

func TestEveryLineIsInLines(t *testing.T) {
	p, _ := game.ParsePack([]byte(pack))
	known := Lines(p)
	for _, target := range []string{"Parrot", "Nothing"} {
		io := &script{t: t}
		n := 0
		answer := truthful(target, map[string]string{"Can it fly?": "yes", "Is it a pet?": "yes"})
		io.reply = func(q string) string {
			n++
			if n == 2 {
				return "" // silence once
			}
			if n == 4 {
				return "purple" // nonsense once
			}
			return answer(q)
		}
		s, _ := newSession(t, io, nil)
		if err := s.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, line := range io.said {
			if !slices.Contains(known, line) {
				t.Errorf("%q is spoken but missing from Lines", line)
			}
		}
		if target == "Nothing" && !slices.Contains(io.said, lineGiveUp) {
			t.Errorf("did not give up: %q", io.said)
		}
		if !slices.Contains(io.said, lineNotHeard) || !slices.Contains(io.said, lineUnclear) {
			t.Errorf("did not ask again after silence and nonsense: %q", io.said)
		}
	}
}

func TestSilenceSendsTheDeviceToSleep(t *testing.T) {
	io := &script{t: t, reply: func(string) string { return "" }}
	s, _ := newSession(t, io, nil)
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := io.said[len(io.said)-1]; got != lineSleep {
		t.Errorf("last line %q", got)
	}
	asks := 0
	for _, line := range io.said {
		if line == io.said[2] {
			asks++
		}
	}
	if asks != 3 {
		t.Errorf("first question asked %d times, want 3", asks)
	}
}

func TestVoiceCommands(t *testing.T) {
	n := 0
	io := &script{t: t}
	io.reply = func(q string) string {
		n++
		switch n {
		case 1:
			return "what?"
		case 2:
			return "start over"
		default:
			return "stop"
		}
	}
	s, _ := newSession(t, io, nil)
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := io.said[2:5]; got[0] != got[1] || !strings.HasSuffix(got[0], "?") || got[2] != lineRestart {
		t.Errorf("said %q, want a question, the same question, then the restart line", got)
	}
	if got := io.said[len(io.said)-1]; got != lineBye {
		t.Errorf("last line %q", got)
	}
}

func TestButtonAnswersAndInterruptsSpeech(t *testing.T) {
	buttons := make(chan game.Reply, 1)
	io := &script{t: t, reply: func(q string) string {
		<-time.After(5 * time.Second) // nobody speaks; only buttons answer
		return ""
	}}
	target := truthful("Shark", map[string]string{"Does it live in water?": "yes", "Is it big?": "yes"})
	io.onSay = func(line string) {
		// Press while the device is still speaking the question.
		switch game.ParseReply(target(line)) {
		case game.Yes:
			if strings.HasSuffix(line, "?") {
				buttons <- game.Yes
			}
		case game.No:
			if strings.HasSuffix(line, "?") {
				buttons <- game.No
			}
		}
		if strings.HasSuffix(line, "?") {
			time.Sleep(10 * time.Millisecond)
		}
	}
	s, c := newSession(t, io, buttons)
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("buttons did not answer; the session waited for speech")
	}
	if !c.Found["Shark"] {
		t.Errorf("Shark not found; said %q", io.said)
	}
}

func TestIdleWaitsForPlayButtonAndStopsWithContext(t *testing.T) {
	buttons := make(chan game.Reply, 1)
	io := &script{t: t, reply: func(string) string { return "stop" }}
	p, _ := game.ParsePack([]byte(pack))
	s := New(io, Options{Pack: p, Rules: game.DefaultRules(), Buttons: buttons})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(20 * time.Millisecond)
	buttons <- game.Restart
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("Run returned %v", err)
	}
	if io.said[0] != lineHello || !slices.Contains(io.said, lineBye) {
		t.Errorf("said %q", io.said)
	}
}
