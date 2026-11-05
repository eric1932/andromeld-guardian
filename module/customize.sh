#!/system/bin/sh
# Sourced by the KernelSU installer. No device changes beyond our module files.
[ "$KSU" = true ] || abort "KernelSU is required."
[ "$ARCH" = arm64 ] || abort "This build supports Android arm64 only."
[ "$API" -ge 31 ] || abort "Android 12+ is required."
set_perm "$MODPATH/bin/guardian" 0 0 0755
for script in service.sh hotinstall.sh uninstall.sh; do
  set_perm "$MODPATH/$script" 0 0 0755
done
# A marker in the NEW module directory revokes old repair authorization in BOTH
# hot and reboot-staged installs. Only an explicit WebUI protect command clears it.
: > "$MODPATH/reauthorize"
set_perm "$MODPATH/reauthorize" 0 0 0600
# Do NOT chmod/chcon webroot: KernelSU sets its own WebView context.
ui_print "AndroMeld Guardian: observation is the default; enable protection in WebUI."
ui_print "No pairing access, system policy, or AndroMeld private files are modified."
# On supported managers this applies only this script/WebUI module immediately.
export MODULE_HOT_INSTALL_REQUEST=true
export MODULE_HOT_RUN_SCRIPT=hotinstall.sh
