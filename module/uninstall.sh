#!/system/bin/sh
MODDIR=${0%/*}
# Stop automatic recovery only; leave the user's current AndroMeld session alone.
# Keep the small root-only state directory rather than race deletion with a tick.
# It contains only settings, certificate hash and classified events (no keys).
"$MODDIR/bin/guardian" stop >/dev/null 2>&1
