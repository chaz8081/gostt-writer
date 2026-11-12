// Package pad holds the hardware-independent logic of the standalone
// dictation macro pad: the talk key's tap/hold behaviour, the record of what
// was typed (so a phrase can be undone exactly), and the USB HID keyboard
// reports that carry text and editing keys to the host.
//
// Nothing here touches GPIO, audio or a USB device. The caller feeds in key
// events and recognised text and writes the returned reports to the HID
// gadget. Shortcuts assume a macOS host with a US keyboard layout.
package pad
