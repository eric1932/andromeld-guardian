#!/system/bin/sh
MODDIR=${0%/*}
# Only stop our own worker through its control file; never signal AndroMeld.
# On upgrade, stop() also resets to observation until the user opts in again.
if ! "$MODDIR/bin/guardian" stop >/dev/null 2>&1; then
  echo "Guardian stop is pending; no second worker was launched."
  exit 1
fi
"$MODDIR/bin/guardian" start >/dev/null 2>&1
