#!/bin/sh
# Installed as /usr/local/bin/gostt-guess: runs the real binary with its
# bundled libraries.
LD_LIBRARY_PATH=/opt/gostt-guess/lib exec /opt/gostt-guess/gostt-guess "$@"
