#!/bin/bash
# Makes the Pi appear to the connected computer as a USB keyboard, by
# creating a HID gadget through configfs. Run as root at boot (see
# gostt-pad-gadget.service). Needs the dwc2 overlay in peripheral mode, which
# deploy/install.sh adds to config.txt.
set -euo pipefail

G=/sys/kernel/config/usb_gadget/gosttpad

modprobe libcomposite
if [ -e "$G/UDC" ] && [ -n "$(cat "$G/UDC")" ]; then
	echo "usb gadget already active"
	exit 0
fi

mkdir -p "$G"
cd "$G"
echo 0x1d6b > idVendor   # Linux Foundation
echo 0x0104 > idProduct  # Multifunction Composite Gadget
echo 0x0100 > bcdDevice
echo 0x0200 > bcdUSB

mkdir -p strings/0x409
echo "gosttpad0001" > strings/0x409/serialnumber
echo "gostt-writer" > strings/0x409/manufacturer
echo "gostt-pad dictation keyboard" > strings/0x409/product

mkdir -p configs/c.1/strings/0x409
echo "Keyboard" > configs/c.1/strings/0x409/configuration
echo 250 > configs/c.1/MaxPower

mkdir -p functions/hid.usb0
echo 1 > functions/hid.usb0/protocol       # keyboard
echo 1 > functions/hid.usb0/subclass       # boot interface
echo 8 > functions/hid.usb0/report_length
# The standard boot keyboard report descriptor: one modifier byte, one
# reserved byte, six key codes, and a five-bit LED output report.
printf '\x05\x01\x09\x06\xa1\x01\x05\x07\x19\xe0\x29\xe7\x15\x00\x25\x01\x75\x01\x95\x08\x81\x02\x95\x01\x75\x08\x81\x03\x95\x05\x75\x01\x05\x08\x19\x01\x29\x05\x91\x02\x95\x01\x75\x03\x91\x03\x95\x06\x75\x08\x15\x00\x25\x65\x05\x07\x19\x00\x29\x65\x81\x00\xc0' \
	> functions/hid.usb0/report_desc

[ -e configs/c.1/hid.usb0 ] || ln -s functions/hid.usb0 configs/c.1/

# Bind to the USB device controller. It can take a moment to appear.
udc=
for _ in $(seq 1 50); do
	for path in /sys/class/udc/*; do
		[ -e "$path" ] && udc=$(basename "$path") && break
	done
	[ -n "$udc" ] && break
	sleep 0.2
done
if [ -z "$udc" ]; then
	echo "no USB device controller found. Is 'dtoverlay=dwc2,dr_mode=peripheral' in config.txt, and has the Pi been rebooted since?" >&2
	exit 1
fi
echo "$udc" > UDC

# Let the gostt-pad user write to the keyboard device.
for _ in $(seq 1 50); do
	[ -e /dev/hidg0 ] && break
	sleep 0.1
done
if [ -e /dev/hidg0 ]; then
	chgrp input /dev/hidg0
	chmod 660 /dev/hidg0
fi
echo "usb gadget active on $udc"
