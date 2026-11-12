#!/bin/sh
# Installed as /usr/local/bin/gostt-pad: runs the real binary with its
# bundled libraries.
LD_LIBRARY_PATH=/opt/gostt-pad/lib exec /opt/gostt-pad/gostt-pad "$@"
