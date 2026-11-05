package main

import (
	"os"
	"os/exec"
	"syscall"
)

func launchWorker(self string) error {
	null, e := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if e != nil {
		return e
	}
	defer null.Close()
	cmd := exec.Command(self, "worker")
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null
	cmd.Env = []string{"PATH=/system/bin:/system/xbin", "ANDROID_ROOT=/system", "ANDROID_DATA=/data"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = cmd.Start(); e != nil {
		return e
	}
	// Worker acquires its own lifetime flock before doing any work. Multiple start
	// requests may fork contenders, but only one can run a monitoring tick.
	return cmd.Process.Release()
}
