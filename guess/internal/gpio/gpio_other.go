//go:build !linux

package gpio

import (
	"errors"
	"time"
)

// Panel is unavailable off Linux; the program still runs in text mode.
type Panel struct{}

// Open always fails off Linux.
func Open(string, map[string]int, int, time.Duration, func(string)) (*Panel, error) {
	return nil, errors.New("gpio: buttons need Linux; run with -buttons=false or -text")
}

// SetLight does nothing.
func (p *Panel) SetLight(bool) {}

// Close does nothing.
func (p *Panel) Close() {}
