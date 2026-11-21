package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Real-device check of the cold-start launch path without touching AndroMeld:
// same environment and credentials, harmless targets. Run as root on Android:
//
//	GUARDIAN_DEVICE_PROBE=1 ./guardian.test -test.run DeviceProbe -test.v
func TestDeviceProbeShellLaunch(t *testing.T) {
	if os.Getenv("GUARDIAN_DEVICE_PROBE") != "1" || os.Geteuid() != 0 {
		t.Skip("Android root only")
	}
	environ, e := zygoteEnviron()
	if e != nil {
		t.Fatal("zygote environment", e)
	}
	env, e := launchEnv(environ, "/system/framework/am.jar")
	if e != nil {
		t.Fatal("launch environment", e)
	}

	id := exec.Command("/system/bin/id")
	id.Env = env
	asShell(id)
	out, e := id.Output()
	if e != nil {
		t.Fatal("id as shell", e)
	}
	t.Log(strings.TrimSpace(string(out)))
	if !strings.HasPrefix(string(out), "uid=2000(shell) gid=2000(shell)") || !strings.Contains(string(out), "3003(inet)") {
		t.Fatal("not adb-shell credentials")
	}

	am := exec.Command("/system/bin/app_process", "/system/bin", "com.android.commands.am.Am", "help")
	am.Env = env
	asShell(am)
	out, e = am.Output()
	if e != nil || !strings.Contains(string(out), "Activity manager") {
		t.Fatal("app_process did not reach main as shell", e, len(out))
	}
	t.Log("app_process reached main as shell")
}
