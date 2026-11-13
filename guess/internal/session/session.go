// Package session runs the conversation: it speaks, listens, watches the
// buttons and plays rounds of the game. It reaches the speaker, microphone
// and buttons only through the IO interface, so it runs the same on the
// device and in tests.
package session

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/chaz8081/gostt-writer/guess/internal/game"
)

// IO is the device's mouth, ears and light.
type IO interface {
	// Say speaks text and returns when it has finished, or early with the
	// context's error when ctx is cancelled.
	Say(ctx context.Context, text string) error
	// Hear listens for one reply and returns the words heard, or "" if
	// nobody spoke within wait.
	Hear(ctx context.Context, wait time.Duration) (string, error)
	// Listening shows the player whether the device is listening.
	Listening(on bool)
}

// Options configure a Session.
type Options struct {
	Pack       *game.Pack
	Rules      game.Rules
	Collection *game.Collection
	// Buttons delivers presses: Yes, No, DontKnow, or Restart for the play
	// button. It may be nil when there are no buttons.
	Buttons <-chan game.Reply
	// Wait is how long to wait for a reply before asking again.
	Wait time.Duration
	// Tries is how many times a question is asked before the device stops
	// and waits for the play button.
	Tries int
	// StartNow plays the first round without waiting for the play button.
	StartNow bool
	// ExitWhenIdle makes Run return instead of waiting for the play button.
	ExitWhenIdle bool
	// Rand picks between equivalent lines; nil uses a random source.
	Rand *rand.Rand
}

// Session plays rounds until its context ends.
type Session struct {
	io  IO
	opt Options
}

// New returns a Session.
func New(io IO, opt Options) *Session {
	if opt.Wait <= 0 {
		opt.Wait = 8 * time.Second
	}
	if opt.Tries <= 0 {
		opt.Tries = 3
	}
	if opt.Rand == nil {
		opt.Rand = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1))
	}
	return &Session{io: io, opt: opt}
}

// How a round or a question ended, beyond a plain answer.
type outcome int

const (
	finished  outcome = iota // the round ran to its end
	restarted                // the player asked to start over
	stopped                  // the player asked to stop
	asleep                   // the player stopped answering
)

// Run plays rounds until ctx ends, or until the device goes idle when
// ExitWhenIdle is set.
func (s *Session) Run(ctx context.Context) error {
	play := s.opt.StartNow
	if !play {
		if err := s.say(ctx, lineHello); err != nil {
			return err
		}
	}
	first := true
	for {
		if !play {
			if s.opt.ExitWhenIdle {
				return nil
			}
			if err := s.waitForPlay(ctx); err != nil {
				return err
			}
		}
		how, err := s.round(ctx, first)
		if err != nil {
			return err
		}
		first = false
		switch how {
		case restarted:
			if err := s.say(ctx, lineRestart); err != nil {
				return err
			}
			play = true
			continue
		case stopped:
			play = false
			if err := s.say(ctx, lineBye); err != nil {
				return err
			}
			continue
		case asleep:
			play = false
			if err := s.say(ctx, lineSleep); err != nil {
				return err
			}
			continue
		}
		reply, how, err := s.ask(ctx, lineAgain)
		if err != nil {
			return err
		}
		play = how == restarted || (how == finished && reply == game.Yes)
		if !play {
			if err := s.say(ctx, lineBye); err != nil {
				return err
			}
		}
	}
}

// waitForPlay blocks until any button is pressed.
func (s *Session) waitForPlay(ctx context.Context) error {
	select {
	case <-s.opt.Buttons:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// round plays one game.
func (s *Session) round(ctx context.Context, explain bool) (outcome, error) {
	p := s.opt.Pack
	if err := s.say(ctx, lineThink(p)); err != nil {
		return 0, err
	}
	if explain {
		if err := s.say(ctx, lineRules); err != nil {
			return 0, err
		}
	}
	r := game.NewRound(p, s.opt.Rules)
	r.Pick = s.opt.Rand.IntN
	guesses := 0
	for {
		move := r.Next()
		switch move.Kind {
		case game.Ask:
			if r.Asked() == s.opt.Rules.MaxQuestions/2 && r.Asked() > 0 {
				if err := s.say(ctx, lineHalfway); err != nil {
					return 0, err
				}
			}
			reply, how, err := s.ask(ctx, p.Questions[move.Question].Ask)
			if err != nil || how != finished {
				return how, err
			}
			slog.Debug("answer", "question", p.Questions[move.Question].ID, "reply", reply.String())
			r.Answer(move.Question, reply)
			if s.opt.Rand.IntN(3) == 0 {
				if err := s.say(ctx, acknowledgements[s.opt.Rand.IntN(len(acknowledgements))]); err != nil {
					return 0, err
				}
			}
		case game.Guess:
			item := p.Items[move.Item]
			guesses++
			if guesses == s.opt.Rules.MaxGuesses && guesses > 1 {
				if err := s.say(ctx, lineLastChance); err != nil {
					return 0, err
				}
			}
			reply, how, err := s.ask(ctx, lineGuess(item))
			if err != nil || how != finished {
				return how, err
			}
			slog.Info("guess", "item", item.Name, "right", reply == game.Yes, "questions", r.Asked())
			if reply == game.Yes {
				return finished, s.celebrate(ctx, item)
			}
			r.WrongGuess(move.Item)
			if r.Next().Kind != game.GiveUp {
				if err := s.say(ctx, lineWrong); err != nil {
					return 0, err
				}
			}
		case game.GiveUp:
			return finished, s.say(ctx, lineGiveUp)
		}
	}
}

// celebrate announces a correct guess and updates the collection.
func (s *Session) celebrate(ctx context.Context, item game.Item) error {
	if err := s.say(ctx, lineWin); err != nil {
		return err
	}
	c := s.opt.Collection
	if c == nil {
		return nil
	}
	isNew, err := c.Add(item.Name)
	if err != nil {
		// The game goes on; the item is only not remembered after a restart.
		slog.Error("collection not saved", "error", err)
	}
	line := lineOld
	if isNew {
		line = lineNew
	}
	if err := s.say(ctx, line); err != nil {
		return err
	}
	found, total := c.Count(s.opt.Pack), len(s.opt.Pack.Items)
	if found == total {
		return s.say(ctx, lineAll)
	}
	return s.say(ctx, lineCount(found, total))
}

// ask speaks a question and returns the player's Yes, No or DontKnow. It
// asks again after silence or an unclear reply, and gives up after Tries.
func (s *Session) ask(ctx context.Context, question string) (game.Reply, outcome, error) {
	lines := []string{question}
	for try := 1; ; {
		reply, silent, err := s.exchange(ctx, lines)
		if err != nil {
			return 0, 0, err
		}
		switch reply {
		case game.Yes, game.No, game.DontKnow:
			return reply, finished, nil
		case game.Restart:
			return 0, restarted, nil
		case game.Quit:
			return 0, stopped, nil
		case game.Repeat:
			lines = []string{question}
			continue
		}
		if try++; try > s.opt.Tries {
			return 0, asleep, nil
		}
		if silent {
			lines = []string{lineNotHeard, question}
		} else {
			lines = []string{lineUnclear, question}
		}
	}
}

// exchange speaks the lines and then listens for one reply. A button press
// at any point, even while the device is still speaking, cuts it short and
// is the reply.
func (s *Session) exchange(ctx context.Context, lines []string) (reply game.Reply, silent bool, err error) {
	s.drainButtons()
	turn, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		heard string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		for _, line := range lines {
			if err := s.io.Say(turn, line); err != nil {
				done <- result{err: err}
				return
			}
		}
		s.io.Listening(true)
		heard, err := s.io.Hear(turn, s.opt.Wait)
		s.io.Listening(false)
		done <- result{heard, err}
	}()
	select {
	case b := <-s.opt.Buttons:
		cancel()
		<-done
		slog.Debug("button", "reply", b.String())
		return b, false, ctx.Err()
	case res := <-done:
		if res.err != nil {
			return 0, false, res.err
		}
		reply = game.ParseReply(res.heard)
		slog.Debug("heard", "text", res.heard, "reply", reply.String())
		return reply, res.heard == "", nil
	}
}

// drainButtons forgets presses made while nothing was being asked.
func (s *Session) drainButtons() {
	for {
		select {
		case <-s.opt.Buttons:
		default:
			return
		}
	}
}

func (s *Session) say(ctx context.Context, line string) error {
	return s.io.Say(ctx, line)
}
