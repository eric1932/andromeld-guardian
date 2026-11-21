package main

import (
	"os"
	"strings"
	"testing"
)

func zygoteFixture(drop string) []byte {
	kv := []string{
		"PATH=/product/bin:/apex/com.android.runtime/bin:/system/bin",
		"ANDROID_ROOT=/system", "ANDROID_DATA=/data",
		"ANDROID_ART_ROOT=/apex/com.android.art", "ANDROID_I18N_ROOT=/apex/com.android.i18n",
		"ANDROID_TZDATA_ROOT=/apex/com.android.tzdata",
		"BOOTCLASSPATH=/apex/com.android.art/javalib/core-oj.jar:/system/framework/framework.jar",
		"ANDROID_SOCKET_zygote=12", "ANDROID_SOCKET_usap_pool_primary=13",
	}
	var out []string
	for _, s := range kv {
		if drop == "" || !strings.HasPrefix(s, drop+"=") {
			out = append(out, s)
		}
	}
	return []byte(strings.Join(out, "\x00") + "\x00")
}

func TestLaunchEnvCopiesRuntimeRootsFromZygote(t *testing.T) {
	env, e := launchEnv(zygoteFixture(""), "/data/app/x/base.apk")
	if e != nil {
		t.Fatal(e)
	}
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, want := range []string{"CLASSPATH=/data/app/x/base.apk", "ANDROID_ART_ROOT=/apex/com.android.art",
		"ANDROID_I18N_ROOT=/apex/com.android.i18n", "ANDROID_TZDATA_ROOT=/apex/com.android.tzdata",
		"HOME=/", "PATH=/product/bin:/apex/com.android.runtime/bin:/system/bin"} {
		if !strings.Contains(joined, "\n"+want+"\n") {
			t.Fatal("missing", want, env)
		}
	}
	if strings.Contains(joined, "ANDROID_SOCKET_") {
		t.Fatal("zygote sockets leaked into the Agent environment")
	}
}

// 0.1.0 launched with only CLASSPATH/PATH/ANDROID_ROOT/ANDROID_DATA, and
// app_process exited silently before main. Never launch without these.
func TestLaunchEnvRefusesMissingRuntimeRoots(t *testing.T) {
	for _, k := range launchEnvRequired {
		if _, e := launchEnv(zygoteFixture(k), "/data/app/x/base.apk"); e == nil {
			t.Fatal("launched without", k)
		}
	}
	if _, e := launchEnv([]byte("BOOTCLASSPATH=a\nb\x00"), "/data/app/x/base.apk"); e == nil {
		t.Fatal("accepted a multi-line value")
	}
}

func TestLaunchEnvDefaultsPathOnlyWhenZygoteHasNone(t *testing.T) {
	env, e := launchEnv(zygoteFixture("PATH"), "/data/app/x/base.apk")
	if e != nil {
		t.Fatal(e)
	}
	n := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			n++
		}
	}
	if n != 1 {
		t.Fatal("PATH count", n, env)
	}
}

func TestAcceptedAgentIdentity(t *testing.T) {
	for _, c := range []struct {
		uid, euid int
		ok        bool
	}{{0, 0, true}, {2000, 2000, true}, {2000, 0, false}, {0, 2000, false}, {10588, 10588, false}} {
		if got := acceptedAgentIdentity(Identity{PID: 9, UID: c.uid, EUID: c.euid}); got != c.ok {
			t.Fatal(c, got)
		}
	}
}

func TestShellCredentialMatchesAdbShell(t *testing.T) {
	want := map[uint32]bool{3003: true, 3009: true, 1007: true, 1011: true}
	for _, g := range shellGroups {
		delete(want, g)
		if g == 0 || g == shellUID {
			t.Fatal("unexpected group", g)
		}
	}
	if len(want) != 0 || len(shellGroups) != 14 {
		t.Fatal("shell groups differ from adbd", want, len(shellGroups))
	}
}

func TestOnlyRootOwnedRegularJSONIsHandedBack(t *testing.T) {
	if !needsShellOwner(0o600, 0) {
		t.Fatal("root-owned regular file kept")
	}
	if needsShellOwner(0o600, shellUID) {
		t.Fatal("shell-owned file touched")
	}
	if needsShellOwner(os.ModeSymlink|0o777, 0) || needsShellOwner(os.ModeDir|0o700, 0) {
		t.Fatal("non-regular entry touched")
	}
}
