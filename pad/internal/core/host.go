package core

import "sync"

// HostSim plays keyboard reports back the way a plain macOS text field
// would, so the pad can be tried without a computer attached and tests can
// check what ends up on screen. Write accepts whole reports.
type HostSim struct {
	mu   sync.Mutex
	text []rune
	keys map[[2]byte]rune

	// OnChange, if set, is called with the full text after each change.
	OnChange func(text string)
}

// Write applies each 8-byte report in p.
func (h *HostSim) Write(p []byte) (int, error) {
	h.mu.Lock()
	if h.keys == nil {
		h.keys = map[[2]byte]rune{}
		for r := rune(0); r < 128; r++ {
			if mod, key, ok := EncodeRune(r); ok {
				h.keys[[2]byte{mod, key}] = r
			}
		}
	}
	changed := false
	for i := 0; i+len(Report{}) <= len(p); i += len(Report{}) {
		changed = h.apply(p[i], p[i+2]) || changed
	}
	text := string(h.text)
	h.mu.Unlock()
	if changed && h.OnChange != nil {
		h.OnChange(text)
	}
	return len(p), nil
}

// Text returns what is on the simulated screen.
func (h *HostSim) Text() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return string(h.text)
}

func (h *HostSim) apply(mod, key byte) bool {
	switch {
	case key == 0:
		return false
	case key == KeyBackspace && mod == 0:
		if len(h.text) > 0 {
			h.text = h.text[:len(h.text)-1]
		}
	case key == KeyBackspace && mod == ModOption:
		for len(h.text) > 0 && h.text[len(h.text)-1] == ' ' {
			h.text = h.text[:len(h.text)-1]
		}
		for len(h.text) > 0 && h.text[len(h.text)-1] != ' ' && h.text[len(h.text)-1] != '\n' {
			h.text = h.text[:len(h.text)-1]
		}
	default:
		c, ok := h.keys[[2]byte{mod, key}]
		if !ok {
			return false
		}
		h.text = append(h.text, c)
	}
	return true
}
