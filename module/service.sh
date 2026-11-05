#!/system/bin/sh
MODDIR=${0%/*}
# Non-blocking late_start stage. No exact alarm, global whitelist or wakelock.
(
  until [ "$(/system/bin/getprop sys.boot_completed)" = 1 ]; do
    [ -e "$MODDIR/disable" ] && exit 0
    [ -e "$MODDIR/remove" ] && exit 0
    sleep 5
  done
  "$MODDIR/bin/guardian" start >/dev/null 2>&1
) </dev/null >/dev/null 2>&1 &
