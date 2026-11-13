package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Collection remembers which items the device has guessed correctly, so the
// player can try to find them all. It is kept in a small JSON file.
type Collection struct {
	path  string
	Found map[string]bool `json:"found"`
}

// OpenCollection loads the collection at path, or starts an empty one if the
// file does not exist. An empty path keeps the collection in memory only.
func OpenCollection(path string) (*Collection, error) {
	c := &Collection{path: path, Found: map[string]bool{}}
	if path == "" {
		return c, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("game: reading collection: %w", err)
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("game: collection file %s is damaged: %w", path, err)
	}
	if c.Found == nil {
		c.Found = map[string]bool{}
	}
	return c, nil
}

// Add records a correct guess and reports whether it was new.
func (c *Collection) Add(name string) (isNew bool, err error) {
	if c.Found[name] {
		return false, nil
	}
	c.Found[name] = true
	return true, c.save()
}

// Count returns how many of the pack's items have been found.
func (c *Collection) Count(p *Pack) int {
	n := 0
	for _, it := range p.Items {
		if c.Found[it.Name] {
			n++
		}
	}
	return n
}

// Names returns the found items in alphabetical order.
func (c *Collection) Names() []string {
	out := make([]string, 0, len(c.Found))
	for name := range c.Found {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// save writes the file through a temporary file, so losing power mid-write
// does not lose the collection.
func (c *Collection) save() error {
	if c.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("game: saving collection: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("game: saving collection: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("game: saving collection: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("game: saving collection: %w", err)
	}
	return nil
}
