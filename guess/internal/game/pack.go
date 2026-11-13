// Package game is the guessing game itself: the pack of things that can be
// guessed, the engine that picks questions, and the parsing of spoken
// replies. It has no hardware or audio dependencies.
package game

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Question is one yes/no question the device can ask.
type Question struct {
	ID  string `yaml:"id"`
	Ask string `yaml:"ask"`
}

// Item is one thing the player can think of. A question listed under Yes is
// true for the item, under Maybe is true only sometimes or arguable, and any
// other question is false.
type Item struct {
	Name  string   `yaml:"name"`
	Say   string   `yaml:"say"` // how to pronounce the name, if spelling it is not enough
	Yes   []string `yaml:"yes"`
	Maybe []string `yaml:"maybe"`
}

// Spoken returns the text to speak for the item's name.
func (it Item) Spoken() string {
	if it.Say != "" {
		return it.Say
	}
	return it.Name
}

// Pack is a set of items and the questions that tell them apart.
type Pack struct {
	Name      string     `yaml:"name"`
	Thing     string     `yaml:"thing"` // what one item is called with its article, e.g. "an animal"
	Questions []Question `yaml:"questions"`
	Items     []Item     `yaml:"items"`

	// truth[item][question] is the chance that a careful player answers yes.
	truth [][]float64
}

// The chance of a yes for each kind of entry.
const (
	pYes   = 1.0
	pMaybe = 0.5
	pNo    = 0.0
)

// LoadPack reads and checks a pack file.
func LoadPack(path string) (*Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("game: reading pack: %w", err)
	}
	return ParsePack(data)
}

// ParsePack checks pack data and prepares it for play.
func ParsePack(data []byte) (*Pack, error) {
	var p Pack
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("game: parsing pack: %w", err)
	}
	if err := p.prepare(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *Pack) prepare() error {
	if p.Thing == "" {
		p.Thing = "something"
	}
	if len(p.Questions) == 0 || len(p.Items) < 2 {
		return fmt.Errorf("game: a pack needs questions and at least two items")
	}
	index := make(map[string]int, len(p.Questions))
	for i, q := range p.Questions {
		if q.ID == "" || q.Ask == "" {
			return fmt.Errorf("game: question %d needs both id and ask", i+1)
		}
		if _, dup := index[q.ID]; dup {
			return fmt.Errorf("game: question id %q is used twice", q.ID)
		}
		index[q.ID] = i
	}
	names := make(map[string]bool, len(p.Items))
	p.truth = make([][]float64, len(p.Items))
	for i, it := range p.Items {
		if it.Name == "" {
			return fmt.Errorf("game: item %d has no name", i+1)
		}
		if names[strings.ToLower(it.Name)] {
			return fmt.Errorf("game: item %q is listed twice", it.Name)
		}
		names[strings.ToLower(it.Name)] = true
		row := make([]float64, len(p.Questions))
		set := func(ids []string, v float64) error {
			for _, id := range ids {
				qi, ok := index[id]
				if !ok {
					return fmt.Errorf("game: item %q uses unknown question id %q", it.Name, id)
				}
				if row[qi] != pNo {
					return fmt.Errorf("game: item %q lists question %q twice", it.Name, id)
				}
				row[qi] = v
			}
			return nil
		}
		if err := set(it.Yes, pYes); err != nil {
			return err
		}
		if err := set(it.Maybe, pMaybe); err != nil {
			return err
		}
		p.truth[i] = row
	}
	// Two items that differ only on "maybe" questions cannot be told apart.
	for i := range p.Items {
		for j := i + 1; j < len(p.Items); j++ {
			apart := false
			for q := range p.Questions {
				if a, b := p.truth[i][q], p.truth[j][q]; a != b && a != pMaybe && b != pMaybe {
					apart = true
					break
				}
			}
			if !apart {
				return fmt.Errorf("game: no question has a definite yes for one of %q and %q and a definite no for the other; add a question that tells them apart", p.Items[i].Name, p.Items[j].Name)
			}
		}
	}
	return nil
}

// Unused returns the ids of questions that are not true for any item, which
// are usually spelling mistakes in the pack.
func (p *Pack) Unused() []string {
	var out []string
	for qi, q := range p.Questions {
		used := false
		for i := range p.Items {
			if p.truth[i][qi] != pNo {
				used = true
				break
			}
		}
		if !used {
			out = append(out, q.ID)
		}
	}
	sort.Strings(out)
	return out
}
