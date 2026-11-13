package game

// Simulate plays a round against a player who is thinking of the item at
// index target and answers from the pack: yes, no, or "don't know" for a
// maybe. The answers to the questions numbered in lies (counting from 1)
// are reversed. It returns the items guessed, in order, and the number of
// questions asked; the round was won if the last guess is target.
func Simulate(p *Pack, rules Rules, target int, lies map[int]bool) (guessed []int, asked int) {
	r := NewRound(p, rules)
	for {
		m := r.Next()
		switch m.Kind {
		case Ask:
			reply := DontKnow
			switch p.truth[target][m.Question] {
			case pYes:
				reply = Yes
			case pNo:
				reply = No
			}
			if lies[r.Asked()+1] && reply != DontKnow {
				reply = Yes + No - reply
			}
			r.Answer(m.Question, reply)
		case Guess:
			guessed = append(guessed, m.Item)
			if m.Item == target {
				return guessed, r.Asked()
			}
			r.WrongGuess(m.Item)
		case GiveUp:
			return guessed, r.Asked()
		}
	}
}
