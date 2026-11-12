#!/bin/bash
# Installs gostt-pad on a Raspberry Pi 4, 5 or Zero 2 W running 64-bit
# Raspberry Pi OS. Run it from a checkout of the repository, on the Pi, as
# your normal user:
#
#   ./deploy/install.sh              everything
#   ./deploy/install.sh build model  only the named steps
#
# Steps: deps go build model config gadget service
# Safe to run again; each step skips what is already done.
set -euo pipefail

GO_VERSION=1.25.6
MODEL=sherpa-onnx-streaming-zipformer-en-kroko-2025-08-06
MODEL_URL=https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/$MODEL.tar.bz2

# DESTDIR lets the file-writing steps be tried without touching the system.
DESTDIR=${DESTDIR:-}
OPT=$DESTDIR/opt/gostt-pad
ETC=$DESTDIR/etc/gostt-pad
HERE=$(cd "$(dirname "$0")" && pwd)
PAD=$(dirname "$HERE")
SUDO=sudo
[ "$(id -u)" = 0 ] && SUDO=
PAD_USER=${SUDO_USER:-$(id -un)}

say() { printf '\n==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

need_arm64() {
	[ "$(uname -m)" = aarch64 ] || die "this needs 64-bit Raspberry Pi OS (uname -m says $(uname -m), not aarch64). \
Reflash the card with the 64-bit Lite image; the original Pi Zero and Zero W cannot run it, the Zero 2 W can."
}

step_deps() {
	say "Installing system packages"
	$SUDO apt-get update -q
	$SUDO apt-get install -y -q alsa-utils curl bzip2 gcc libc6-dev ca-certificates
}

go_bin() {
	if [ -x /usr/local/go/bin/go ]; then echo /usr/local/go/bin/go; else command -v go || true; fi
}

step_go() {
	need_arm64
	local go have
	go=$(go_bin)
	if [ -n "$go" ]; then
		have=$("$go" env GOVERSION | sed 's/^go//')
		if [ "$(printf '%s\n' 1.24 "$have" | sort -V | head -n 1)" = 1.24 ]; then
			say "Go $have is already installed"
			return
		fi
	fi
	say "Installing Go $GO_VERSION to /usr/local/go"
	local tmp
	tmp=$(mktemp -d)
	curl -fL --retry 3 -o "$tmp/go.tgz" "https://go.dev/dl/go$GO_VERSION.linux-arm64.tar.gz"
	$SUDO rm -rf /usr/local/go
	$SUDO tar -C /usr/local -xzf "$tmp/go.tgz"
	rm -rf "$tmp"
}

step_build() {
	need_arm64
	local go
	go=$(go_bin)
	[ -n "$go" ] || die "Go is not installed; run: $0 go"
	say "Building gostt-pad (a few minutes on a Zero 2 W)"
	local jobs=()
	# With under 1 GB of memory, compile one package at a time.
	if [ "$(awk '/MemTotal/ {print $2}' /proc/meminfo)" -lt 1000000 ]; then jobs=(-p 1); fi
	local version
	version=$(git -C "$PAD" describe --always --dirty 2>/dev/null || echo dev)
	(cd "$PAD" && CGO_ENABLED=1 "$go" build "${jobs[@]}" -ldflags "-X main.version=$version" -o "$PAD/gostt-pad" ./cmd/gostt-pad)
	local libs
	libs=$(cd "$PAD" && "$go" list -m -f '{{.Dir}}' github.com/k2-fsa/sherpa-onnx-go-linux)/lib/aarch64-unknown-linux-gnu
	[ -f "$libs/libonnxruntime.so" ] || die "the speech libraries were not found in $libs"
	$SUDO mkdir -p "$OPT/lib"
	$SUDO install -m 755 "$PAD/gostt-pad" "$OPT/gostt-pad"
	$SUDO install -m 644 "$libs"/*.so "$OPT/lib/"
	$SUDO install -m 755 "$HERE/gostt-pad-wrapper.sh" "$DESTDIR/usr/local/bin/gostt-pad"
	say "Installed $OPT/gostt-pad (run it as: gostt-pad)"
}

step_model() {
	if [ -f "$OPT/models/$MODEL/tokens.txt" ]; then
		say "The speech model is already in $OPT/models"
		return
	fi
	say "Downloading the speech model (about 55 MB)"
	local tmp
	tmp=$(mktemp -d)
	curl -fL --retry 3 -o "$tmp/model.tar.bz2" "$MODEL_URL"
	tar -C "$tmp" -xjf "$tmp/model.tar.bz2"
	[ -f "$tmp/$MODEL/tokens.txt" ] || die "the downloaded model archive does not contain $MODEL/tokens.txt"
	$SUDO mkdir -p "$OPT/models"
	$SUDO rm -rf "$OPT/models/$MODEL"
	$SUDO cp -r "$tmp/$MODEL" "$OPT/models/"
	$SUDO chmod -R a+rX "$OPT/models"
	rm -rf "$tmp"
}

step_config() {
	if [ -f "$ETC/config.yaml" ]; then
		say "Keeping your existing $ETC/config.yaml"
		return
	fi
	say "Writing $ETC/config.yaml"
	$SUDO mkdir -p "$ETC"
	$SUDO install -m 644 "$PAD/config.example.yaml" "$ETC/config.yaml"
}

step_gadget() {
	say "Setting up the USB keyboard gadget"
	$SUDO mkdir -p "$OPT"
	$SUDO install -m 755 "$HERE/usb-gadget.sh" "$OPT/usb-gadget.sh"
	$SUDO install -m 644 "$HERE/gostt-pad-gadget.service" "$DESTDIR/etc/systemd/system/gostt-pad-gadget.service"
	local boot=$DESTDIR/boot/firmware/config.txt
	[ -f "$boot" ] || boot=$DESTDIR/boot/config.txt
	[ -f "$boot" ] || die "config.txt was not found under /boot/firmware or /boot"
	if grep -Eq '^\s*dtoverlay=dwc2,dr_mode=peripheral' "$boot"; then
		echo "config.txt already enables USB device mode"
	else
		printf '\n# gostt-pad: let the USB-C / USB data port act as a keyboard\n[all]\ndtoverlay=dwc2,dr_mode=peripheral\n' | $SUDO tee -a "$boot" > /dev/null
		echo "added dtoverlay=dwc2,dr_mode=peripheral to $boot"
		REBOOT=1
	fi
	if [ -z "$DESTDIR" ]; then
		$SUDO systemctl daemon-reload
		$SUDO systemctl enable gostt-pad-gadget.service
	fi
}

step_service() {
	say "Installing the gostt-pad service (not started; see the README)"
	sed "s/__USER__/$PAD_USER/" "$HERE/gostt-pad.service" | $SUDO tee "$DESTDIR/etc/systemd/system/gostt-pad.service" > /dev/null
	if [ -z "$DESTDIR" ]; then
		for group in input audio gpio; do
			getent group "$group" > /dev/null && $SUDO usermod -aG "$group" "$PAD_USER"
		done
		$SUDO systemctl daemon-reload
	fi
}

REBOOT=
steps=("$@")
[ ${#steps[@]} -gt 0 ] || steps=(deps go build model config gadget service)
for s in "${steps[@]}"; do
	case $s in
	deps | go | build | model | config | gadget | service) "step_$s" ;;
	*) die "unknown step '$s' (steps: deps go build model config gadget service)" ;;
	esac
done

say "Done."
if [ -n "$REBOOT" ]; then
	echo "Reboot now so the USB keyboard mode takes effect:  sudo reboot"
fi
echo "Then follow the test steps in pad/README.md, starting with:  gostt-pad check -dry-run -keys none"
