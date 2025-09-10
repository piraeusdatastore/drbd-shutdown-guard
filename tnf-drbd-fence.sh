#!/bin/sh
set -e

export PATH="/run/drbd-shutdown-guard:$PATH"
exec /run/tnf-drbd-fence/tnf-drbd-fence.py
