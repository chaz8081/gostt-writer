package game

import "strings"

// Phrases are matched as whole words anywhere in what was heard. Longer
// phrases are checked before the single words inside them, so "i don't think
// so" is a no and not a yes.
var replyPhrases = []struct {
	reply   Reply
	phrases []string
}{
	{Restart, []string{"start over", "start again", "new game", "restart"}},
	{Quit, []string{"stop", "goodbye", "good bye", "bye", "i'm done", "im done", "quit", "turn off"}},
	{Repeat, []string{"repeat", "say that again", "say it again", "again", "what", "pardon"}},
	{DontKnow, []string{
		"don't know", "dont know", "do not know", "dunno", "don't no", "dont no", "not sure",
		"no idea", "maybe", "sometimes", "kind of", "sort of", "skip", "i forget", "i forgot",
	}},
	{No, []string{
		"don't think so", "dont think so", "do not think so", "not really", "uh uh", "nuh uh",
		"it doesn't", "it does not", "it isn't", "it is not", "it's not", "it can't", "it cannot",
		"no", "nope", "nah", "naw", "not", "never", "wrong", "false", "incorrect", "negative",
	}},
	{Yes, []string{
		"think so", "uh huh", "it does", "it is", "it can", "of course", "you got it", "that's it", "thats it",
		"yes", "yeah", "yep", "yup", "yea", "yeh", "ya", "yah", "yas", "yess", "sure", "correct", "right",
		"true", "definitely", "totally", "absolutely", "probably", "okay", "ok",
	}},
}

// Words a speech model often hears in place of a short answer, counted only
// when they are the whole reply.
var soundAlikes = map[string]Reply{
	"huh": Repeat, "know": No, "new": No, "now": No, "noe": No, "nor": No,
	"yet": Yes, "es": Yes, "s": Yes,
}

var yesWords = []string{"yes", "yeah", "yep", "yup", "yea", "yeh", "ya", "yah"}

// ParseReply works out what the player meant from the words heard. It
// returns Unclear for anything it cannot place, including a reply that
// sounds like both yes and no.
func ParseReply(heard string) Reply {
	text := normalize(heard)
	if text == "" {
		return Unclear
	}
	if r, ok := soundAlikes[text]; ok {
		return r
	}
	// A speech model often hears a lone short word as a longer one: "no"
	// as "nove", "yes" as "yet". To a yes-or-no question, one short word
	// that starts like an answer is taken as that answer.
	if !strings.Contains(text, " ") && len(text) <= 5 && !knownWord(text) {
		switch {
		case strings.HasPrefix(text, "no"), strings.HasPrefix(text, "na"):
			return No
		case strings.HasPrefix(text, "ye"), strings.HasPrefix(text, "ya"):
			return Yes
		}
	}
	padded := " " + text + " "
	for _, group := range replyPhrases {
		for _, phrase := range group.phrases {
			if !strings.Contains(padded, " "+phrase+" ") {
				continue
			}
			// A bare "no" next to a bare "yes" could mean either.
			if group.reply == No && !strings.Contains(phrase, " ") && containsAny(padded, yesWords) {
				return Unclear
			}
			return group.reply
		}
	}
	return Unclear
}

// knownWord reports whether a single word is one of the listed phrases.
func knownWord(word string) bool {
	for _, group := range replyPhrases {
		for _, phrase := range group.phrases {
			if phrase == word {
				return true
			}
		}
	}
	return false
}

func containsAny(padded string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(padded, " "+p+" ") {
			return true
		}
	}
	return false
}

// normalize lowercases, keeps letters and apostrophes, and joins words with
// single spaces.
func normalize(s string) string {
	var b strings.Builder
	space := true
	for _, c := range strings.ToLower(s) {
		switch {
		case c >= 'a' && c <= 'z', c == '\'':
			b.WriteRune(c)
			space = false
		case c == '’':
			b.WriteRune('\'')
			space = false
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
