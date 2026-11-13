package game

import (
	"path/filepath"
	"strings"
	"testing"
)

const tinyPack = `
name: Tiny
thing: an animal
questions:
  - {id: flies, ask: "Can it fly?"}
  - {id: swims, ask: "Does it live in water?"}
  - {id: big, ask: "Is it bigger than a person?"}
  - {id: pet, ask: "Do people keep it as a pet?"}
items:
  - {name: Eagle, yes: [flies]}
  - {name: Shark, yes: [swims, big]}
  - {name: Goldfish, yes: [swims, pet]}
  - {name: Horse, yes: [big], maybe: [pet]}
  - {name: Cat, yes: [pet]}
  - {name: Parrot, yes: [flies, pet]}
`

func mustPack(t *testing.T, src string) *Pack {
	t.Helper()
	p, err := ParsePack([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func play(p *Pack, target int, lies map[int]bool) (guessed []int, asked int) {
	return Simulate(p, DefaultRules(), target, lies)
}

func TestEveryItemIsGuessedFirstTime(t *testing.T) {
	p := mustPack(t, tinyPack)
	for i, it := range p.Items {
		guessed, asked := play(p, i, nil)
		if len(guessed) != 1 || guessed[0] != i {
			t.Errorf("%s: guesses %v", it.Name, guessed)
		}
		if asked > len(p.Questions) {
			t.Errorf("%s: %d questions", it.Name, asked)
		}
	}
}

func TestOneWrongAnswerStillFindsTheItem(t *testing.T) {
	p, err := LoadPack(filepath.Join("..", "..", "packs", "animals.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	games, found := 0, 0
	for i := range p.Items {
		for lie := 1; lie <= 4; lie++ {
			guessed, _ := play(p, i, map[int]bool{lie: true})
			games++
			if len(guessed) > 0 && guessed[len(guessed)-1] == i {
				found++
			}
		}
	}
	t.Logf("with one wrong answer: found %d of %d", found, games)
	if found*100 < games*90 {
		t.Errorf("found only %d of %d", found, games)
	}
}

func TestSamplePack(t *testing.T) {
	p, err := LoadPack(filepath.Join("..", "..", "packs", "animals.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if u := p.Unused(); len(u) > 0 {
		t.Errorf("questions true for no item: %v", u)
	}
	total, worst := 0, 0
	for i, it := range p.Items {
		guessed, asked := play(p, i, nil)
		if len(guessed) != 1 {
			t.Errorf("%s: took %d guesses", it.Name, len(guessed))
		}
		total += asked
		worst = max(worst, asked)
	}
	t.Logf("%d items, %.1f questions on average, %d at most", len(p.Items), float64(total)/float64(len(p.Items)), worst)
	if worst > DefaultRules().MaxQuestions {
		t.Errorf("worst case %d questions", worst)
	}
}

func TestGivesUpAfterMaxGuesses(t *testing.T) {
	p := mustPack(t, tinyPack)
	r := NewRound(p, DefaultRules())
	guesses := 0
	for {
		m := r.Next()
		if m.Kind == GiveUp {
			break
		}
		if m.Kind == Ask {
			r.Answer(m.Question, No)
			continue
		}
		guesses++
		r.WrongGuess(m.Item)
	}
	if guesses != DefaultRules().MaxGuesses {
		t.Errorf("guessed %d times", guesses)
	}
}

func TestPackErrors(t *testing.T) {
	cases := map[string]string{
		"definite":       strings.Replace(tinyPack, "{name: Cat, yes: [pet]}", "{name: Cat, yes: [flies], maybe: [pet]}", 1),
		"unknown":        strings.Replace(tinyPack, "yes: [flies]}", "yes: [fly]}", 1),
		"listed twice":   strings.Replace(tinyPack, "name: Cat", "name: eagle", 1),
		"unknown field":  tinyPack + "extra: 1\n",
		"used twice":     strings.Replace(tinyPack, "id: big", "id: flies", 1),
		"lists question": strings.Replace(tinyPack, "yes: [big], maybe: [pet]", "yes: [big], maybe: [big]", 1),
	}
	for want, src := range cases {
		if _, err := ParsePack([]byte(src)); err == nil {
			t.Errorf("%s: no error", want)
		} else if !strings.Contains(err.Error(), strings.Fields(want)[0]) && want != "unknown field" {
			t.Errorf("%s: error %q", want, err)
		}
	}
}

func TestParseReply(t *testing.T) {
	cases := map[string]Reply{
		"Yes.": Yes, "YEAH": Yes, "yep!": Yes, "uh huh": Yes, "I think so": Yes, "yes it does": Yes,
		"yes yes yes": Yes, "it is": Yes, "yet": Yes, "um yeah": Yes,
		"no": No, "Nope": No, "no no no": No, "i don't think so": No, "not really": No, "know": No,
		"nove": No, "Noah.": No, "yell": Yes, "november": Unclear, "not": No, "never": No, "it doesn't": No, "it’s not": No, "no it is not": No, "uh uh": No,
		"I don't know": DontKnow, "i dont no": DontKnow, "maybe": DontKnow, "sometimes": DontKnow,
		"um not sure": DontKnow, "kind of": DontKnow,
		"what?": Repeat, "say that again": Repeat,
		"start over": Restart, "stop": Quit, "goodbye": Quit,
		"": Unclear, "banana": Unclear, "yes no": Unclear, "no yeah": Unclear, "the": Unclear,
	}
	for heard, want := range cases {
		if got := ParseReply(heard); got != want {
			t.Errorf("ParseReply(%q) = %v, want %v", heard, got, want)
		}
	}
}

func TestCollection(t *testing.T) {
	p := mustPack(t, tinyPack)
	path := filepath.Join(t.TempDir(), "sub", "found.json")
	c, err := OpenCollection(path)
	if err != nil {
		t.Fatal(err)
	}
	if isNew, err := c.Add("Cat"); !isNew || err != nil {
		t.Fatalf("first add: %v %v", isNew, err)
	}
	if isNew, _ := c.Add("Cat"); isNew {
		t.Error("second add counted as new")
	}
	_, _ = c.Add("Not in pack")
	c2, err := OpenCollection(path)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Count(p) != 1 || len(c2.Names()) != 2 {
		t.Errorf("reloaded: count %d names %v", c2.Count(p), c2.Names())
	}
}
