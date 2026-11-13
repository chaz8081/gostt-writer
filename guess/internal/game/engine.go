package game

import "math"

// Reply is what the player answered.
type Reply int

// The replies a player can give. Yes, No and DontKnow answer a question;
// the others steer the game.
const (
	Unclear Reply = iota
	Yes
	No
	DontKnow
	Repeat
	Restart
	Quit
)

func (r Reply) String() string {
	return [...]string{"unclear", "yes", "no", "don't know", "repeat", "restart", "quit"}[r]
}

// Rules tune how a round is played.
type Rules struct {
	MaxQuestions int     // questions asked before the device must guess
	MaxGuesses   int     // wrong guesses before the device gives up
	Confidence   float64 // guess as soon as one item is at least this likely
	Mistakes     float64 // chance that any one answer from the player is wrong
}

// DefaultRules suit a young player: answers are trusted, but not completely.
func DefaultRules() Rules {
	return Rules{MaxQuestions: 20, MaxGuesses: 3, Confidence: 0.8, Mistakes: 0.05}
}

// Move is what the device does next.
type Move struct {
	Kind     MoveKind
	Question int // index into Pack.Questions, for Ask
	Item     int // index into Pack.Items, for Guess
}

// MoveKind says whether a Move is a question, a guess or giving up.
type MoveKind int

// The kinds of move.
const (
	Ask MoveKind = iota
	Guess
	GiveUp
)

// Round is one game: the player has one item in mind and the device narrows
// it down. A wrong answer lowers an item's weight instead of removing it, so
// one mistake from the player does not lose the round.
type Round struct {
	pack    *Pack
	rules   Rules
	weight  []float64 // unnormalised likelihood per item; 0 once guessed wrong
	asked   []bool
	nAsked  int
	guesses int

	// Pick chooses between questions that are about equally useful, so
	// rounds do not all open the same way. It returns a number below n.
	// When nil, the most useful question is always asked.
	Pick func(n int) int
}

// NewRound starts a round with every item equally likely.
func NewRound(p *Pack, rules Rules) *Round {
	r := &Round{pack: p, rules: rules, weight: make([]float64, len(p.Items)), asked: make([]bool, len(p.Questions))}
	for i := range r.weight {
		r.weight[i] = 1
	}
	return r
}

// Asked returns how many questions have been answered so far.
func (r *Round) Asked() int { return r.nAsked }

// pYesFor is the chance the player says yes to question q if thinking of
// item i, allowing for mistakes.
func (r *Round) pYesFor(i, q int) float64 {
	t := r.pack.truth[i][q]
	return t*(1-r.rules.Mistakes) + (1-t)*r.rules.Mistakes
}

// Next decides the next move.
func (r *Round) Next() Move {
	best, bestP, total := -1, 0.0, 0.0
	for i, w := range r.weight {
		total += w
		if w > bestP {
			best, bestP = i, w
		}
	}
	if best < 0 || r.guesses >= r.rules.MaxGuesses {
		return Move{Kind: GiveUp}
	}
	if bestP/total >= r.rules.Confidence || r.nAsked >= r.rules.MaxQuestions {
		return Move{Kind: Guess, Item: best}
	}
	q, gain := r.bestQuestion(total)
	if q < 0 || gain < 1e-6 {
		return Move{Kind: Guess, Item: best}
	}
	return Move{Kind: Ask, Question: q}
}

// bestQuestion returns the unasked question whose answer is expected to
// reduce uncertainty the most, and by how many bits.
func (r *Round) bestQuestion(total float64) (int, float64) {
	now := entropy(r.weight, total)
	gains := make([]float64, len(r.pack.Questions))
	best, bestGain := -1, 0.0
	yes := make([]float64, len(r.weight))
	no := make([]float64, len(r.weight))
	for q := range r.pack.Questions {
		if r.asked[q] {
			continue
		}
		var sumYes, sumNo float64
		for i, w := range r.weight {
			p := r.pYesFor(i, q)
			yes[i], no[i] = w*p, w*(1-p)
			sumYes += yes[i]
			sumNo += no[i]
		}
		after := (sumYes*entropy(yes, sumYes) + sumNo*entropy(no, sumNo)) / total
		gains[q] = now - after
		if gains[q] > bestGain {
			best, bestGain = q, gains[q]
		}
	}
	if r.Pick == nil || best < 0 {
		return best, bestGain
	}
	var close []int
	for q, g := range gains {
		if g >= 0.9*bestGain && g > 0 {
			close = append(close, q)
		}
	}
	q := close[r.Pick(len(close))]
	return q, gains[q]
}

func entropy(w []float64, total float64) float64 {
	if total <= 0 {
		return 0
	}
	h := 0.0
	for _, x := range w {
		if x > 0 {
			p := x / total
			h -= p * math.Log2(p)
		}
	}
	return h
}

// Answer records the player's reply to a question. DontKnow uses up the
// question without changing what the device believes.
func (r *Round) Answer(q int, reply Reply) {
	if r.asked[q] {
		return
	}
	r.asked[q] = true
	r.nAsked++
	for i := range r.weight {
		switch reply {
		case Yes:
			r.weight[i] *= r.pYesFor(i, q)
		case No:
			r.weight[i] *= 1 - r.pYesFor(i, q)
		}
	}
}

// WrongGuess records that the item guessed was not the player's.
func (r *Round) WrongGuess(item int) {
	r.weight[item] = 0
	r.guesses++
}
