#!/bin/bash
# Installs gostt-guess on a Raspberry Pi 4, 5 or Zero 2 W running 64-bit
# Raspberry Pi OS. Run it from a checkout of the repository, on the Pi, as
# your normal user:
#
#   ./deploy/install.sh              everything
#   ./deploy/install.sh build voice  only the named steps
#
# Steps: deps go build models config voice service
# Safe to run again; each step skips what is already done.
set -euo pipefail

GO_VERSION=1.25.6
MODELS_URL=https://github.com/k2-fsa/sherpa-onnx/releases/download
ASR_MODEL=sherpa-onnx-streaming-zipformer-en-kroko-2025-08-06
VOICE_MODEL=vits-piper-en_US-amy-low

# DESTDIR lets the file-writing steps be tried without touching the system.
DESTDIR=${DESTDIR:-}
OPT=$DESTDIR/opt/gostt-guess
ETC=$DESTDIR/etc/gostt-guess
VAR=$DESTDIR/var/lib/gostt-guess
HERE=$(cd "$(dirname "$0")" && pwd)
SRC=$(dirname "$HERE")
SUDO=sudo
[ "$(id -u)" = 0 ] && SUDO=
RUN_USER=${SUDO_USER:-$(id -un)}

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
	say "Building gostt-guess (a few minutes on a Zero 2 W)"
	local jobs=()
	# With under 1 GB of memory, compile one package at a time.
	if [ "$(awk '/MemTotal/ {print $2}' /proc/meminfo)" -lt 1000000 ]; then jobs=(-p 1); fi
	local version
	version=$(git -C "$SRC" describe --always --dirty 2>/dev/null || echo dev)
	(cd "$SRC" && CGO_ENABLED=1 "$go" build "${jobs[@]}" -ldflags "-X main.version=$version" -o "$SRC/gostt-guess" ./cmd/gostt-guess)
	local libs
	libs=$(cd "$SRC" && "$go" list -m -f '{{.Dir}}' github.com/k2-fsa/sherpa-onnx-go-linux)/lib/aarch64-unknown-linux-gnu
	[ -f "$libs/libonnxruntime.so" ] || die "the speech libraries were not found in $libs"
	$SUDO mkdir -p "$OPT/lib" "$DESTDIR/usr/local/bin"
	$SUDO install -m 755 "$SRC/gostt-guess" "$OPT/gostt-guess"
	$SUDO install -m 644 "$libs"/*.so "$OPT/lib/"
	$SUDO install -m 755 "$HERE/gostt-guess-wrapper.sh" "$DESTDIR/usr/local/bin/gostt-guess"
	say "Installed $OPT/gostt-guess (run it as: gostt-guess)"
}

# fetch_model <release folder> <model name> <what it is>
fetch_model() {
	local kind=$1 name=$2 what=$3
	if [ -f "$OPT/models/$name/tokens.txt" ]; then
		say "The $what is already in $OPT/models"
		return
	fi
	say "Downloading the $what (60 to 70 MB)"
	local tmp
	tmp=$(mktemp -d)
	curl -fL --retry 3 -o "$tmp/model.tar.bz2" "$MODELS_URL/$kind/$name.tar.bz2"
	tar -C "$tmp" -xjf "$tmp/model.tar.bz2"
	[ -f "$tmp/$name/tokens.txt" ] || die "the downloaded archive does not contain $name/tokens.txt"
	$SUDO mkdir -p "$OPT/models"
	$SUDO rm -rf "$OPT/models/$name"
	$SUDO cp -r "$tmp/$name" "$OPT/models/"
	$SUDO chmod -R a+rX "$OPT/models"
	rm -rf "$tmp"
}

step_models() {
	fetch_model asr-models "$ASR_MODEL" "speech recognition model"
	fetch_model tts-models "$VOICE_MODEL" "voice"
}

step_config() {
	$SUDO mkdir -p "$ETC" "$VAR/clips"
	[ -n "$DESTDIR" ] || $SUDO chown -R "$RUN_USER" "$VAR"
	local f
	for f in config pack; do
		if [ -f "$ETC/$f.yaml" ]; then
			say "Keeping your existing $ETC/$f.yaml"
		elif [ $f = config ]; then
			say "Writing $ETC/config.yaml"
			$SUDO install -m 644 "$SRC/config.example.yaml" "$ETC/config.yaml"
		else
			say "Writing $ETC/pack.yaml (the sample animal pack)"
			$SUDO install -m 644 "$SRC/packs/animals.yaml" "$ETC/pack.yaml"
		fi
	done
}

step_voice() {
	say "Making the voice clips (once per pack; this can take a while on a Zero 2 W)"
	"$DESTDIR/usr/local/bin/gostt-guess" voice
}

step_service() {
	say "Installing the gostt-guess service (not started; see the README)"
	$SUDO mkdir -p "$DESTDIR/etc/systemd/system"
	sed "s/__USER__/$RUN_USER/" "$HERE/gostt-guess.service" | $SUDO tee "$DESTDIR/etc/systemd/system/gostt-guess.service" > /dev/null
	if [ -z "$DESTDIR" ]; then
		for group in audio gpio; do
			getent group "$group" > /dev/null && $SUDO usermod -aG "$group" "$RUN_USER"
		done
		$SUDO systemctl daemon-reload
	fi
}

all="deps go build models config voice service"
steps=("$@")
# shellcheck disable=SC2206
[ ${#steps[@]} -gt 0 ] || steps=($all)
for s in "${steps[@]}"; do
	case " $all " in
	*" $s "*) "step_$s" ;;
	*) die "unknown step '$s' (steps: $all)" ;;
	esac
done

say "Done."
echo "Log out and in again so the audio and gpio groups apply, then follow the"
echo "test steps in guess/README.md, starting with:  gostt-guess -text"
