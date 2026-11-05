package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const pkg = "com.catchingnow.andfiles.helper"
const agentDir = "/data/local/tmp/AndroMeldAgent"
const procName = pkg + ":device_agent"

type AgentState struct {
	PID   int `json:"pid"`
	Port  int `json:"port"`
	Admin int `json:"webAdminPort"`
	Build int `json:"agent"`
}
type Identity struct {
	PID   int
	UID   int
	EUID  int
	Ticks uint64
}
type Target struct {
	State   AgentState
	Process Identity
}
type Package struct {
	UID   int
	Build int
	APK   string
}
type RPCResult struct {
	OK              bool `json:"ok"`
	Enabled         bool `json:"enabled"`
	MacEnabled      bool `json:"macEnabled"`
	Connected       bool `json:"connected"`
	Active          bool `json:"active"`
	DeviceConnected bool `json:"deviceConnected"`
	AgentBuild      int  `json:"agentVersionCode"`
	Port            int  `json:"port"`
}

var errEvidence = errors.New("evidence_unavailable")

func readBounded(p string, n int64) ([]byte, error) {
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, n+1))
	if e != nil || int64(len(b)) > n {
		return nil, errEvidence
	}
	return b, nil
}

type cappedWriter struct {
	buf bytes.Buffer
	max int
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		return 0, errEvidence
	}
	return w.buf.Write(p)
}
func command(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out := &cappedWriter{max: 512 * 1024}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if e := cmd.Run(); e != nil {
		return nil, errEvidence
	}
	return out.buf.Bytes(), nil
}
func packageInfo() (Package, error) {
	uidOut, e := command(5*time.Second, "/system/bin/pm", "list", "packages", "-U", "--user", "0", pkg)
	if e != nil {
		return Package{}, e
	}
	re := regexp.MustCompile(`(?m)^package:` + regexp.QuoteMeta(pkg) + ` uid:(\d+)\s*$`)
	matches := re.FindAllSubmatch(uidOut, -1)
	if len(matches) != 1 {
		return Package{}, errEvidence
	}
	uid, _ := strconv.Atoi(string(matches[0][1]))
	if uid < 10000 || uid > 19999 {
		return Package{}, errEvidence
	}
	dumps, e := command(5*time.Second, "/system/bin/dumpsys", "package", pkg)
	if e != nil {
		return Package{}, e
	}
	code := regexp.MustCompile(`\bversionCode=(\d+)`).FindSubmatch(dumps)
	if len(code) != 2 {
		return Package{}, errEvidence
	}
	build, _ := strconv.Atoi(string(code[1]))
	path, e := command(5*time.Second, "/system/bin/pm", "path", "--user", "0", pkg)
	if e != nil {
		return Package{}, e
	}
	// This version is a single base.apk; split installs require a new validated adapter.
	lines := strings.Fields(string(path))
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "package:/data/app/") || !strings.HasSuffix(lines[0], "/base.apk") {
		return Package{}, errEvidence
	}
	apk := strings.TrimPrefix(lines[0], "package:")
	if strings.ContainsAny(apk, "\r\n\x00") {
		return Package{}, errEvidence
	}
	return Package{uid, build, apk}, nil
}
func readAgentState() (AgentState, error) {
	b, e := readBounded(agentDir+"/state.json", 16384)
	if e != nil {
		return AgentState{}, e
	}
	var s AgentState
	e = json.Unmarshal(b, &s)
	if e != nil || s.PID <= 1 || s.Port < 1024 || s.Port > 65535 || s.Admin < 13022 || s.Admin > 13031 || s.Build <= 0 {
		return s, errEvidence
	}
	return s, nil
}
func identity(pid int) (Identity, error) {
	base := fmt.Sprintf("/proc/%d/", pid)
	b, e := readBounded(base+"cmdline", 8192)
	if e != nil {
		return Identity{}, e
	}
	if string(bytes.SplitN(b, []byte{0}, 2)[0]) != procName {
		return Identity{}, errEvidence
	}
	st, e := readBounded(base+"stat", 8192)
	if e != nil {
		return Identity{}, e
	}
	end := bytes.LastIndexByte(st, ')')
	if end < 0 {
		return Identity{}, errEvidence
	}
	fields := strings.Fields(string(st[end+1:]))
	if len(fields) < 20 || fields[0] == "Z" {
		return Identity{}, errEvidence
	}
	ticks, e := strconv.ParseUint(fields[19], 10, 64)
	if e != nil {
		return Identity{}, errEvidence
	}
	status, e := readBounded(base+"status", 16384)
	if e != nil {
		return Identity{}, e
	}
	m := regexp.MustCompile(`(?m)^Uid:\s+(\d+)\s+(\d+)`).FindSubmatch(status)
	if len(m) != 3 {
		return Identity{}, errEvidence
	}
	uid, _ := strconv.Atoi(string(m[1]))
	euid, _ := strconv.Atoi(string(m[2]))
	return Identity{pid, uid, euid, ticks}, nil
}
func sockets(pid int) (map[string]bool, error) {
	entries, e := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if e != nil {
		return nil, e
	}
	out := map[string]bool{}
	for _, f := range entries {
		link, e := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, f.Name()))
		if e != nil {
			continue
		}
		if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
			out[strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")] = true
		}
	}
	return out, nil
}
func socketMatch(table string, inodes map[string]bool, server, client int, established bool) bool {
	for _, line := range strings.Split(table, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || !inodes[f[9]] {
			continue
		}
		la := strings.Split(f[1], ":")
		ra := strings.Split(f[2], ":")
		if len(la) != 2 || len(ra) != 2 {
			continue
		}
		// Require the exact IPv4 endpoint the child connects to, not an unrelated ::1 socket.
		loop := func(a string) bool { return a == "0100007F" || a == "0000000000000000FFFF00000100007F" }
		if !loop(la[0]) {
			continue
		}
		port, e := strconv.ParseInt(la[1], 16, 32)
		if e != nil || int(port) != server {
			continue
		}
		if !established && f[3] == "0A" {
			return true
		}
		remote, e := strconv.ParseInt(ra[1], 16, 32)
		if established && f[3] == "01" && e == nil && int(remote) == client && loop(ra[0]) {
			return true
		}
	}
	return false
}
func ownsSocket(t Target, client int, established bool) bool {
	ids, e := sockets(t.Process.PID)
	if e != nil {
		return false
	}
	for _, p := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, e := readBounded(p, 2*1024*1024)
		if e == nil && socketMatch(string(b), ids, t.State.Admin, client, established) {
			return true
		}
	}
	return false
}
func validatedTarget() (Target, error) {
	s, e := readAgentState()
	if e != nil {
		return Target{}, e
	}
	p, e := identity(s.PID)
	if e != nil {
		return Target{}, e
	}
	t := Target{s, p}
	if !ownsSocket(t, 0, false) {
		return t, errEvidence
	}
	return t, nil
}
func targetStill(t Target) bool {
	current, e := validatedTarget()
	return e == nil && current == t
}
func gate(t Target, client int) bool {
	until := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(until) {
		if !targetStill(t) {
			return false
		}
		if ownsSocket(t, client, true) {
			return targetStill(t)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var out []byte
	for {
		part, e := r.ReadSlice('\n')
		out = append(out, part...)
		if len(out) > max+1 {
			return nil, errEvidence
		}
		if e == nil {
			return out[:len(out)-1], nil
		}
		if e != bufio.ErrBufferFull {
			return nil, e
		}
	}
}
func rpcChild(args []string) int {
	if len(args) != 3 || os.Geteuid() != 0 {
		return 2
	}
	uid, e := strconv.Atoi(args[0])
	if e != nil || uid < 10000 || uid > 19999 {
		return 2
	}
	port, e := strconv.Atoi(args[1])
	if e != nil || port < 13022 || port > 13031 {
		return 2
	}
	op := args[2]
	if op != "web.status" && op != "mac.link" {
		return 2
	}
	// syscall.Setuid uses the Go runtime's all-thread credential transition.
	if syscall.Setgroups([]int{}) != nil || syscall.Setgid(uid) != nil || syscall.Setuid(uid) != nil {
		return 2
	}
	if os.Geteuid() != uid {
		return 2
	}
	c, e := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if e != nil {
		return 2
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	json.NewEncoder(os.Stdout).Encode(map[string]int{"client_port": c.LocalAddr().(*net.TCPAddr).Port})
	reader := bufio.NewReaderSize(os.Stdin, 256)
	line, e := readLine(reader, 32)
	if e != nil || string(line) != "GO" {
		return 2
	}
	payload, _ := json.Marshal(map[string]string{"op": op})
	payload = append(payload, '\n')
	if _, e = c.Write(payload); e != nil {
		return 2
	}
	response, e := readLine(bufio.NewReaderSize(c, 1024), 8192)
	if e != nil {
		return 2
	}
	result, e := decodeRPC(response, op)
	if e != nil {
		return 2
	}
	// The type deliberately excludes client names, identities, addresses and secrets.
	json.NewEncoder(os.Stdout).Encode(result)
	return 0
}
func rpc(t Target, uid int, op string) (RPCResult, error) {
	if op != "web.status" && op != "mac.link" {
		return RPCResult{}, errEvidence
	}
	self, e := os.Executable()
	if e != nil {
		return RPCResult{}, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "_rpc", strconv.Itoa(uid), strconv.Itoa(t.State.Admin), op)
	in, e := cmd.StdinPipe()
	if e != nil {
		return RPCResult{}, e
	}
	defer in.Close()
	out, e := cmd.StdoutPipe()
	if e != nil {
		return RPCResult{}, e
	}
	cmd.Stderr = io.Discard
	if e = cmd.Start(); e != nil {
		return RPCResult{}, e
	}
	defer func() { cancel(); cmd.Wait() }()
	r := bufio.NewReaderSize(out, 1024)
	packet, e := readLine(r, 256)
	if e != nil {
		return RPCResult{}, errEvidence
	}
	var phase struct {
		Port int `json:"client_port"`
	}
	if json.Unmarshal(packet, &phase) != nil || phase.Port < 1 || phase.Port > 65535 || !gate(t, phase.Port) {
		return RPCResult{}, errEvidence
	}
	if _, e = io.WriteString(in, "GO\n"); e != nil {
		return RPCResult{}, errEvidence
	}
	packet, e = readLine(r, 8192)
	if e != nil {
		return RPCResult{}, errEvidence
	}
	var result RPCResult
	if json.Unmarshal(packet, &result) != nil || !result.OK || !targetStill(t) {
		return RPCResult{}, errEvidence
	}
	return result, nil
}

// Only a complete /proc enumeration can confirm absence; permission errors do
// not mean 'dead'. A same-name process with an unexpected UID blocks recovery.
func anyAgent() (bool, error) {
	dirs, e := os.ReadDir("/proc")
	if e != nil {
		return false, e
	}
	for _, d := range dirs {
		if _, e := strconv.Atoi(d.Name()); e != nil {
			continue
		}
		b, e := readBounded("/proc/"+d.Name()+"/cmdline", 16384)
		if e != nil {
			if os.IsNotExist(e) || errors.Is(e, syscall.ESRCH) {
				continue
			}
			return false, errEvidence
		}
		if string(bytes.SplitN(b, []byte{0}, 2)[0]) == procName {
			return true, nil
		}
	}
	return false, nil
}
func collect() (Status, *Target, Package) {
	s := Status{CheckedAt: time.Now().UTC(), Health: "unknown", Reason: "evidence_unavailable"}
	p, e := packageInfo()
	if e != nil {
		s.Reason = "package_unconfirmed"
		return s, nil, p
	}
	s.Daemon.Build = p.Build
	s.Supported = p.Build == supportedBuild
	if !s.Supported {
		s.Health = "blocked"
		s.Reason = "unsupported_app_version"
		return s, nil, p
	}
	t, e := validatedTarget()
	if e != nil {
		exists, scanErr := anyAgent()
		s.Daemon.Alive = exists
		s.MissingConfirmed = scanErr == nil && !exists
		if s.MissingConfirmed {
			s.Health = "degraded"
			s.Reason = "daemon_absent"
		} else {
			s.Health = "blocked"
			s.Reason = "daemon_target_unconfirmed"
		}
		return s, nil, p
	}
	s.Daemon.PID = t.Process.PID
	s.Daemon.Alive = true
	if t.State.Build != p.Build || t.Process.UID != 0 || t.Process.EUID != 0 {
		s.Health = "blocked"
		s.Reason = "daemon_identity_or_version_mismatch"
		return s, nil, p
	}
	w, e := rpc(t, p.UID, "web.status")
	if e != nil {
		s.Health = "degraded"
		s.Reason = "admin_unresponsive_no_automatic_kill"
		return s, &t, p
	}
	m, e := rpc(t, p.UID, "mac.link")
	if e != nil || w.AgentBuild != p.Build {
		s.Health = "degraded"
		s.Reason = "status_incomplete"
		return s, &t, p
	}
	s.Daemon.Responsive = true
	s.Web = WebStatus{w.Enabled, w.Connected, w.Port}
	s.Mac = MacStatus{m.Active, m.Connected, m.DeviceConnected, w.MacEnabled}
	s.Health = "healthy"
	s.Reason = "ready"
	if !s.Web.Enabled || !s.Mac.Active {
		s.Health = "degraded"
		s.Reason = "access_idle_or_disabled"
	}
	return s, &t, p
}
func nativeWake(kind Action, t Target, p Package) error {
	if kind != wakeWeb && kind != wakeMac {
		return errEvidence
	}
	// Revalidate BOTH package and daemon before the only external mutation.
	fresh, e := packageInfo()
	if e != nil || fresh != p || p.Build != supportedBuild || !targetStill(t) {
		return errEvidence
	}
	w, e := rpc(t, p.UID, "web.status")
	if e != nil {
		return errEvidence
	}
	m, e := rpc(t, p.UID, "mac.link")
	if e != nil {
		return errEvidence
	}
	if kind == wakeWeb && w.Connected {
		return nil
	}
	if kind == wakeMac && (m.Connected || m.DeviceConnected) {
		return nil
	}
	event := "open_web_service"
	if kind == wakeMac {
		event = "open_mac_link"
	}
	// A ROOT-local explicit broadcast reuses the app's FCM message handler.
	// No cloud push, ticket, credential, raw gateway toggle, prefs edit or pairing.
	return dispatchWhen(configStillProtect, func() error {
		b, err := command(10*time.Second, "/system/bin/am", "broadcast", "--user", "0", "-a", "com.google.android.c2dm.intent.RECEIVE", "-n", pkg+"/com.google.firebase.iid.FirebaseInstanceIdReceiver", "--es", "t", event)
		if err != nil || !bytes.Contains(b, []byte("Broadcast completed")) {
			return errEvidence
		}
		return nil
	})
}
func privateAgentData() bool {
	for _, p := range []string{agentDir, agentDir + "/identity.p8", agentDir + "/identity.crt", agentDir + "/clients.json"} {
		st, e := os.Lstat(p)
		if e != nil || st.Mode()&os.ModeSymlink != 0 {
			return false
		}
		sys, ok := st.Sys().(*syscall.Stat_t)
		if !ok || (sys.Uid != 0 && sys.Uid != 2000) || st.Mode().Perm()&0077 != 0 {
			return false
		}
		if p == agentDir {
			if !st.IsDir() {
				return false
			}
		} else {
			if !st.Mode().IsRegular() || st.Size() == 0 {
				return false
			}
		}
	}
	return true
}

// Trust only the already-enrolled public certificate captured while the live
// root daemon was verified. Do not generate keys or alter shell/root ownership.
func certificateDigest() (string, error) {
	b, e := readBounded(agentDir+"/identity.crt", 65536)
	if e != nil {
		return "", e
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}
func rememberCertificate() error {
	digest, e := certificateDigest()
	if e != nil {
		return e
	}
	return saveJSON("trust.json", map[string]string{"certificate_sha256": digest})
}
func matchesTrustedCertificate() bool {
	var trust map[string]string
	if loadJSON("trust.json", &trust) != nil {
		return false
	}
	digest, e := certificateDigest()
	return e == nil && trust["certificate_sha256"] == digest
}
func bootstrap(p Package) error {
	fresh, e := packageInfo()
	if e != nil || fresh != p || p.Build != supportedBuild || !privateAgentData() || !matchesTrustedCertificate() {
		return errEvidence
	}
	live, e := anyAgent()
	if e != nil || live {
		return errEvidence
	}
	// Official single-instance file lock in DeviceAgentMain also arbitrates races
	// with the app's own reconnect watcher. Do not unlink it or kill its owner.
	cmd := exec.Command("/system/bin/app_process", "/system/bin", "--nice-name="+procName, pkg+".DeviceAgentMain")
	cmd.Env = []string{"CLASSPATH=" + p.APK, "PATH=/system/bin:/system/xbin", "ANDROID_ROOT=/system", "ANDROID_DATA=/data"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devnull, e := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if e != nil {
		return e
	}
	defer devnull.Close()
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	return dispatchWhen(configStillProtect, func() error {
		if err := cmd.Start(); err != nil {
			return errEvidence
		}
		go cmd.Wait() // Reap only our launched process; never signal unrelated PIDs.
		return nil
	})
}

// Check policy/stop/module flags at the dispatch boundary, after all expensive
// preparation. In-flight OS calls cannot be recalled; do not launch the next one.
func dispatchWhen(allowed func() bool, send func() error) error {
	if !allowed() {
		return errEvidence
	}
	return send()
}

func decodeRPC(data []byte, op string) (RPCResult, error) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return RPCResult{}, errEvidence
	}
	boolKeys := []string{"ok", "active", "connected", "deviceConnected"}
	intKeys := []string{}
	if op == "web.status" {
		boolKeys = []string{"ok", "enabled", "connected", "macEnabled"}
		intKeys = []string{"agentVersionCode", "port"}
	} else if op != "mac.link" {
		return RPCResult{}, errEvidence
	}
	for _, key := range boolKeys {
		b, exists := raw[key]
		if !exists || (!bytes.Equal(b, []byte("true")) && !bytes.Equal(b, []byte("false"))) {
			return RPCResult{}, errEvidence
		}
	}
	for _, key := range intKeys {
		b, exists := raw[key]
		var n int
		if !exists || bytes.Equal(b, []byte("null")) || json.Unmarshal(b, &n) != nil {
			return RPCResult{}, errEvidence
		}
	}
	var result RPCResult
	if json.Unmarshal(data, &result) != nil || !result.OK {
		return RPCResult{}, errEvidence
	}
	return result, nil
}
