//go:build linux

// Package gpio reads the device's buttons and drives its light through the
// Linux GPIO character device, which works the same on a Raspberry Pi 4, 5
// and Zero 2 W.
package gpio

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

// FindChip returns the GPIO chip that carries the 40-pin header: the one the
// Pi's pin controller registers (pinctrl-bcm2835 or pinctrl-bcm2711 on the
// Pi 4 and Zero 2 W, pinctrl-rp1 on the Pi 5).
func FindChip() (string, error) {
	var seen []string
	for _, name := range gpiocdev.Chips() {
		c, err := gpiocdev.NewChip(name)
		if err != nil {
			continue
		}
		label := c.Label
		_ = c.Close()
		seen = append(seen, name+" ("+label+")")
		if strings.HasPrefix(label, "pinctrl-bcm2") || strings.HasPrefix(label, "pinctrl-rp1") {
			return name, nil
		}
	}
	if len(seen) == 0 {
		return "", errors.New("gpio: no GPIO chips found; is this a Raspberry Pi, and may this user open /dev/gpiochip*?")
	}
	return "", fmt.Errorf("gpio: none of the GPIO chips is the Pi header (found %s); set gpio.chip", strings.Join(seen, ", "))
}

// Panel is the set of requested button lines and the light.
type Panel struct {
	lines []*gpiocdev.Line
	led   *gpiocdev.Line
}

// Open requests the button pins as pulled-up inputs and calls pressed with
// a button's name each time it is pushed down. A button connects its pin to
// ground. buttons maps a name to its BCM GPIO number; led is the light's
// GPIO or -1. An empty chip is found automatically.
func Open(chip string, buttons map[string]int, led int, debounce time.Duration, pressed func(name string)) (*Panel, error) {
	if chip == "" {
		var err error
		if chip, err = FindChip(); err != nil {
			return nil, err
		}
	}
	p := &Panel{}
	for name, pin := range buttons {
		if pin < 0 {
			continue
		}
		line, err := gpiocdev.RequestLine(chip, pin,
			gpiocdev.WithConsumer("gostt-guess"),
			gpiocdev.AsInput,
			gpiocdev.WithPullUp,
			gpiocdev.AsActiveLow,
			gpiocdev.WithRisingEdge,
			gpiocdev.WithDebounce(debounce),
			gpiocdev.WithEventHandler(func(gpiocdev.LineEvent) { pressed(name) }))
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("gpio: %s button on GPIO %d: %w", name, pin, err)
		}
		p.lines = append(p.lines, line)
	}
	if led >= 0 {
		line, err := gpiocdev.RequestLine(chip, led, gpiocdev.WithConsumer("gostt-guess"), gpiocdev.AsOutput(0))
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("gpio: light on GPIO %d: %w", led, err)
		}
		p.led = line
	}
	return p, nil
}

// SetLight turns the light on or off. It does nothing when there is none.
func (p *Panel) SetLight(on bool) {
	if p == nil || p.led == nil {
		return
	}
	v := 0
	if on {
		v = 1
	}
	_ = p.led.SetValue(v)
}

// Close releases the pins, leaving the light off.
func (p *Panel) Close() {
	if p == nil {
		return
	}
	p.SetLight(false)
	for _, l := range p.lines {
		_ = l.Close()
	}
	if p.led != nil {
		_ = p.led.Close()
	}
}
