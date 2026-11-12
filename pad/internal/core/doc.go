// Package core holds the hardware-independent logic of the standalone
// dictation macro pad: the talk key's tap/hold behaviour, the decision of
// when recognised words are stable enough to type, the record of what was
// typed (so a phrase can be undone exactly), and the USB HID keyboard
// reports that carry text and editing keys to the host.
//
// Nothing here touches GPIO, audio devices or the speech model. Shortcuts
// assume a macOS host with a US keyboard layout.
package core
