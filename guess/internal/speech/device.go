package speech

import (
	"context"
	"time"

	"github.com/chaz8081/gostt-writer/guess/internal/audio"
)

// Device speaks through a playback command and hears through a Listener.
// With Light it implements the session's IO.
type Device struct {
	Voice    *Voice
	Listener *Listener
	Play     string     // playback command; see audio.PlayCommand
	Volume   float32    // 1 plays clips as made; higher is louder
	Light    func(bool) // may be nil
}

// Say speaks one line.
func (d *Device) Say(ctx context.Context, text string) error {
	samples, rate, err := d.Voice.Clip(text)
	if err != nil {
		return err
	}
	return audio.PlayCommand(ctx, d.Play, samples, rate, d.Volume)
}

// Hear listens for one reply.
func (d *Device) Hear(ctx context.Context, wait time.Duration) (string, error) {
	return d.Listener.Hear(ctx, wait)
}

// Listening turns the light on while the device listens.
func (d *Device) Listening(on bool) {
	if d.Light != nil {
		d.Light(on)
	}
}
