// Package inject provides text injection into the active application
// using robotgo for keystroke simulation or clipboard paste.
package inject

import (
	"fmt"

	"github.com/go-vgo/robotgo"
)

// Injector handles typing or pasting text into the active application.
type Injector struct {
	method string // "type" or "paste"
}

// NewInjector creates an Injector with the given method.
// method must be "type" (keystroke simulation) or "paste" (clipboard).
func NewInjector(method string) *Injector {
	return &Injector{method: method}
}

// Inject sends text to the active application using the configured method.
func (inj *Injector) Inject(text string) error {
	if text == "" {
		return nil
	}

	switch inj.method {
	case "paste":
		return inj.paste(text)
	default: // "type"
		return inj.typeText(text)
	}
}

// typeText simulates individual keystrokes. Preserves clipboard contents
// but is slower for long text.
func (inj *Injector) typeText(text string) error {
	robotgo.Type(text)
	return nil
}

// paste copies text to clipboard and pastes it with Cmd+V.
// Faster for long text but overwrites the clipboard.
func (inj *Injector) paste(text string) error {
	// Save current clipboard
	prev, _ := robotgo.ReadAll()

	// Write text to clipboard
	if err := robotgo.WriteAll(text); err != nil {
		return fmt.Errorf("inject: write to clipboard: %w", err)
	}

	// Paste with Cmd+V
	if err := robotgo.KeyTap("v", "cmd"); err != nil {
		return fmt.Errorf("inject: key tap cmd+v: %w", err)
	}

	// Restore previous clipboard (best effort)
	_ = robotgo.WriteAll(prev)

	return nil
}
