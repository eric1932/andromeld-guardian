#!/system/bin/sh
MODDIR=${0%/*}
# Only restart our own worker through its control file; never signal AndroMeld.
# The user's protect/observe switch is kept.
if ! "$MODDIR/bin/guardian" restart >/dev/null 2>&1; then
  echo "Guardian restart is pending; the next start finishes it."
  exit 1
fi
