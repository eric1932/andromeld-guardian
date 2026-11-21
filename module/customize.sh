#!/system/bin/sh
# Sourced by the KernelSU installer. No device changes beyond our module files.
[ "$KSU" = true ] || abort "KernelSU is required."
[ "$ARCH" = arm64 ] || abort "This build supports Android arm64 only."
[ "$API" -ge 31 ] || abort "Android 12+ is required."
set_perm "$MODPATH/bin/guardian" 0 0 0755
for script in service.sh hotinstall.sh uninstall.sh; do
  set_perm "$MODPATH/$script" 0 0 0755
done
# The protect/observe switch lives in /data/adb/andromeld_guardian and is kept
# across upgrades; installing never turns protection on or off.
# Do NOT chmod/chcon webroot: KernelSU sets its own WebView context.
ui_print "AndroMeld Guardian: first install observes; an upgrade keeps your protection switch."
ui_print "No pairing access, system policy, or AndroMeld private files are modified."
# On supported managers this applies only this script/WebUI module immediately.
export MODULE_HOT_INSTALL_REQUEST=true
export MODULE_HOT_RUN_SCRIPT=hotinstall.sh
