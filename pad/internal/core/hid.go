package core

import (
	"fmt"
	"io"
)

// Report is one USB HID boot-keyboard input report: a modifier byte, a
// reserved byte, and up to six key codes. Only the first key slot is used.
type Report [8]byte

// Modifier bits of a keyboard report.
const (
	ModShift  byte = 0x02
	ModOption byte = 0x04 // Left Alt, which macOS treats as Option
)

// Key codes (USB HID usage page 0x07) used directly by the pad.
const (
	KeyEnter     byte = 0x28
	KeyBackspace byte = 0x2A
)

// releaseAll is the report that lifts every key.
var releaseAll = Report{}

// Tap returns the two reports for pressing and releasing one key.
func Tap(mod, key byte) []Report {
	return []Report{{mod, 0, key}, releaseAll}
}

// shifted maps the symbols typed with Shift on a US layout to the key that
// carries them.
var shifted = map[rune]byte{
	'!': 0x1E, '@': 0x1F, '#': 0x20, '$': 0x21, '%': 0x22, '^': 0x23, '&': 0x24, '*': 0x25, '(': 0x26, ')': 0x27,
	'_': 0x2D, '+': 0x2E, '{': 0x2F, '}': 0x30, '|': 0x31, ':': 0x33, '"': 0x34, '~': 0x35, '<': 0x36, '>': 0x37, '?': 0x38,
}

// unshifted maps the symbols typed without Shift on a US layout.
var unshifted = map[rune]byte{
	' ': 0x2C, '-': 0x2D, '=': 0x2E, '[': 0x2F, ']': 0x30, '\\': 0x31, ';': 0x33, '\'': 0x34, '`': 0x35, ',': 0x36, '.': 0x37, '/': 0x38,
	'\n': KeyEnter, '\t': 0x2B,
}

// substitutes maps characters a recogniser may emit that a US layout cannot
// type to the nearest character it can.
var substitutes = map[rune]rune{
	'‘': '\'', '’': '\'', '“': '"', '”': '"', '–': '-', '—': '-', ' ': ' ',
}

// EncodeRune returns the modifier and key code that type r on a US layout.
// ok is false when r cannot be typed.
func EncodeRune(r rune) (mod, key byte, ok bool) {
	if s, found := substitutes[r]; found {
		r = s
	}
	switch {
	case r >= 'a' && r <= 'z':
		return 0, 0x04 + byte(r-'a'), true
	case r >= 'A' && r <= 'Z':
		return ModShift, 0x04 + byte(r-'A'), true
	case r >= '1' && r <= '9':
		return 0, 0x1E + byte(r-'1'), true
	case r == '0':
		return 0, 0x27, true
	}
	if k, found := unshifted[r]; found {
		return 0, k, true
	}
	if k, found := shifted[r]; found {
		return ModShift, k, true
	}
	return 0, 0, false
}

// TypeString returns the reports that type s, and how many characters they
// produce on the host. Characters that cannot be typed are skipped and not
// counted.
func TypeString(s string) (reports []Report, typed int) {
	for _, r := range s {
		mod, key, ok := EncodeRune(r)
		if !ok {
			continue
		}
		reports = append(reports, Tap(mod, key)...)
		typed++
	}
	return reports, typed
}

// WriteReports sends reports to a HID gadget device such as /dev/hidg0, one
// write per report.
func WriteReports(w io.Writer, reports []Report) error {
	for _, r := range reports {
		if _, err := w.Write(r[:]); err != nil {
			return fmt.Errorf("pad: write hid report: %w", err)
		}
	}
	return nil
}
