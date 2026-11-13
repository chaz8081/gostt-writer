package session

import (
	"fmt"

	"github.com/chaz8081/gostt-writer/guess/internal/game"
)

// Everything the device says outside the pack's own questions. Each line is
// spoken as one clip, so the whole set can be made ahead of time.
const (
	lineHello      = "Hello! Press the big button when you want to play."
	lineRules      = "Answer yes, no, or I don't know. You can say it, or press a button."
	lineNotHeard   = "I did not hear you."
	lineUnclear    = "Sorry, I did not get that. Say yes, no, or I don't know."
	lineWrong      = "Hmm. Let me think some more."
	lineGiveUp     = "You stumped me! I give up. Well done."
	lineWin        = "Yes! I got it!"
	lineNew        = "That is a new one for your collection."
	lineOld        = "We have found that one before."
	lineAll        = "You have found every single one. Amazing!"
	lineAgain      = "Do you want to play again?"
	lineRestart    = "Okay, let's start over."
	lineBye        = "Okay. Press the big button when you want to play again. Bye for now!"
	lineSleep      = "I will wait here. Press the big button when you want to play."
	lineHalfway    = "I am getting closer."
	lineLastChance = "This is my last guess."
)

var acknowledgements = []string{"Okay.", "Hmm.", "Got it.", "Interesting."}

func lineThink(p *game.Pack) string {
	return fmt.Sprintf("Think of %s, and do not tell me what it is. I will try to guess it.", p.Thing)
}

func lineGuess(it game.Item) string {
	return fmt.Sprintf("I think I know. Is it %s?", it.Spoken())
}

func lineCount(found, total int) string {
	return fmt.Sprintf("You have found %d out of %d.", found, total)
}

// Lines returns every line the device can say with this pack, for making
// the voice clips ahead of time.
func Lines(p *game.Pack) []string {
	out := []string{
		lineHello, lineRules, lineNotHeard, lineUnclear, lineWrong, lineGiveUp, lineWin, lineNew, lineOld,
		lineAll, lineAgain, lineRestart, lineBye, lineSleep, lineHalfway, lineLastChance, lineThink(p),
	}
	out = append(out, acknowledgements...)
	for _, q := range p.Questions {
		out = append(out, q.Ask)
	}
	for _, it := range p.Items {
		out = append(out, lineGuess(it))
	}
	for n := 1; n < len(p.Items); n++ {
		out = append(out, lineCount(n, len(p.Items)))
	}
	return out
}
