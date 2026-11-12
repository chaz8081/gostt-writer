//go:build linux

// Package gpio reads the pad's buttons and drives its LED through the Linux
// GPIO character device, which works the same on a Raspberry Pi 4, 5 and
// Zero 2 W.
package gpio

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/warthog618/go-gpiocdev"

	"github.com/chaz8081/gostt-writer/pad/internal/core"
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

// Pad is the set of requested button lines and the LED.
type Pad struct {
	lines []*gpiocdev.Line
	led   *gpiocdev.Line
}

// Open requests the button pins as pulled-up inputs and sends an event to
// events for every press and release. A button connects its pin to ground.
// buttons maps a key to its BCM GPIO number; led is the LED's GPIO or -1.
func Open(chip string, buttons map[core.Key]int, led int, debounce time.Duration, events chan<- core.KeyEvent) (*Pad, error) {
	p := &Pad{}
	for key, pin := range buttons {
		line, err := gpiocdev.RequestLine(chip, pin,
			gpiocdev.WithConsumer("gostt-pad"),
			gpiocdev.AsInput,
			gpiocdev.WithPullUp,
			gpiocdev.AsActiveLow,
			gpiocdev.WithBothEdges,
			gpiocdev.WithDebounce(debounce),
			gpiocdev.WithEventHandler(func(evt gpiocdev.LineEvent) {
				events <- core.KeyEvent{Key: key, Down: evt.Type == gpiocdev.LineEventRisingEdge, At: time.Now()}
			}))
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("gpio: %s button on GPIO %d: %w", key, pin, err)
		}
		p.lines = append(p.lines, line)
	}
	if led >= 0 {
		line, err := gpiocdev.RequestLine(chip, led, gpiocdev.WithConsumer("gostt-pad"), gpiocdev.AsOutput(0))
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("gpio: LED on GPIO %d: %w", led, err)
		}
		p.led = line
	}
	return p, nil
}

// SetLED turns the LED on or off. It does nothing when there is no LED.
func (p *Pad) SetLED(on bool) {
	if p == nil || p.led == nil {
		return
	}
	v := 0
	if on {
		v = 1
	}
	_ = p.led.SetValue(v)
}

// Close releases the pins, leaving the LED off.
func (p *Pad) Close() {
	if p == nil {
		return
	}
	p.SetLED(false)
	for _, l := range p.lines {
		_ = l.Close()
	}
	if p.led != nil {
		_ = p.led.Close()
	}
}
