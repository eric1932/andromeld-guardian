package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const dataDir = "/data/adb/andromeld_guardian"

type Store struct {
	Status  Status  `json:"status"`
	Events  []Event `json:"events"`
	History History `json:"history"`
}
type View struct {
	OK      bool    `json:"ok"`
	Version string  `json:"version"`
	Config  Config  `json:"config"`
	Status  Status  `json:"status"`
	Events  []Event `json:"events"`
}

func moduleDir() string {
	p, e := os.Executable()
	if e != nil {
		return ""
	}
	return filepath.Dir(filepath.Dir(p))
}
func unavailableModule() bool {
	root := moduleDir()
	if root == "" {
		return true
	}
	for _, name := range []string{"disable", "remove"} {
		if _, e := os.Stat(filepath.Join(root, name)); e == nil {
			return true
		}
	}
	return false
}
func secureData() error {
	if os.Geteuid() != 0 {
		return errors.New("requires_root")
	}
	if _, e := os.Stat("/system/bin/getprop"); e != nil {
		return errors.New("requires_android")
	}
	if e := os.Mkdir(dataDir, 0700); e != nil && !os.IsExist(e) {
		return errEvidence
	}
	st, e := os.Lstat(dataDir)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe_data_directory")
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 {
		return errors.New("unsafe_data_owner")
	}
	return nil
}
func loadJSON(name string, value any) error {
	b, e := readBounded(filepath.Join(dataDir, name), 128*1024)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, value)
}
func saveJSON(name string, value any) error {
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	p, e := os.CreateTemp(dataDir, ".atomic-")
	if e != nil {
		return e
	}
	tmp := p.Name()
	defer os.Remove(tmp)
	p.Chmod(0600)
	if _, e = p.Write(append(b, '\n')); e != nil {
		p.Close()
		return e
	}
	if e = p.Sync(); e != nil {
		p.Close()
		return e
	}
	if e = p.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(dataDir, name))
}
func loadConfig() (Config, error) {
	c := defaultConfig()
	e := loadJSON("config.json", &c)
	if !os.IsNotExist(e) && (e != nil || !validConfig(c)) {
		return defaultConfig(), errors.New("invalid_config_observation_only")
	}
	_, markerErr := os.Lstat(filepath.Join(moduleDir(), "reauthorize"))
	return consentConfig(c, !os.IsNotExist(markerErr)), nil
}
func consentConfig(c Config, needsConsent bool) Config {
	if needsConsent {
		c.Mode = "observe"
	}
	return c
}
func decodeStore(b []byte) (Store, error) {
	var s Store
	var root map[string]json.RawMessage
	if json.Unmarshal(b, &root) != nil {
		return s, errEvidence
	}
	var history map[string]json.RawMessage
	if json.Unmarshal(root["history"], &history) != nil || history["attempts"] == nil || history["missing_samples"] == nil {
		return s, errEvidence
	}
	if json.Unmarshal(b, &s) != nil || s.Status.CheckedAt.IsZero() || s.History.Attempts == nil || len(s.History.Attempts) > 150 || s.History.MissingSamples < 0 || s.History.MissingSamples > 3 {
		return Store{}, errEvidence
	}
	for _, a := range s.History.Attempts {
		if a.At.IsZero() || (a.Kind != string(wakeWeb) && a.Kind != string(wakeMac) && a.Kind != string(coldStart)) {
			return Store{}, errEvidence
		}
	}
	return s, nil
}
func loadStore() (Store, error) {
	b, e := readBounded(filepath.Join(dataDir, "state.json"), 128*1024)
	if e != nil {
		return Store{}, e
	}
	return decodeStore(b)
}
func historyAllows(c Config, err error) bool { return err == nil || c.Mode == "observe" }
func cachedStore() Store {
	s, e := loadStore()
	if e != nil {
		s.Status.Health = "blocked"
		s.Status.Reason = "history_unavailable_observation_only"
	}
	return s
}
func acquire(name string) (*os.File, error) {
	p := filepath.Join(dataDir, name)
	fd, e := syscall.Open(p, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), p)
	if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func release(f *os.File) {
	if f != nil {
		f.Close()
	}
}
func workerRunning() bool {
	f, e := acquire("worker.lock")
	if e != nil {
		return errors.Is(e, syscall.EWOULDBLOCK) || errors.Is(e, syscall.EAGAIN)
	}
	release(f)
	return false
}
func emit(v any) { json.NewEncoder(os.Stdout).Encode(v) }
func outputView(c Config, s Store) {
	s.Status.WorkerRunning = workerRunning()
	if s.Events == nil {
		s.Events = []Event{}
	}
	emit(View{true, version, c, s.Status, s.Events})
}
func addEvent(s *Store, kind, detail string) {
	// Only fixed caller-owned text belongs here. Never append raw command output.
	if n := len(s.Events); n > 0 && s.Events[n-1].Kind == kind && time.Since(s.Events[n-1].At) < 5*time.Minute {
		return
	}
	s.Events = append(s.Events, Event{time.Now().UTC(), kind, detail})
	if len(s.Events) > 60 {
		s.Events = s.Events[len(s.Events)-60:]
	}
}
func configStillProtect() bool {
	c, e := loadConfig()
	if e != nil || c.Mode != "protect" || unavailableModule() {
		return false
	}
	_, e = os.Stat(filepath.Join(dataDir, "stop"))
	return os.IsNotExist(e)
}
func tick() (Config, Store, error) {
	lock, e := acquire("control.lock")
	if e != nil {
		return Config{}, Store{}, errors.New("check_in_progress")
	}
	defer release(lock)
	c, configErr := loadConfig()
	store, historyErr := loadStore()
	now := time.Now().UTC()
	s, t, p := collect()
	if !historyAllows(c, historyErr) {
		// Never replace an unknown rate-limit history with an empty trusted one.
		// Observation can initialize fresh evidence; protection cannot self-reset.
		s.Health = "blocked"
		s.Reason = "history_unavailable_observation_only"
		store.Status = s
		return c, store, nil
	}
	if s.MissingConfirmed {
		if store.History.MissingSamples < 3 {
			store.History.MissingSamples++
		}
	} else {
		store.History.MissingSamples = 0
	}
	store.History = trimHistory(store.History, now)
	if configErr != nil {
		s.Health = "blocked"
		s.Reason = "invalid_config_observation_only"
	}
	old := store.Status
	if old.Reason != s.Reason || old.Daemon.PID != s.Daemon.PID {
		addEvent(&store, "state_changed", s.Reason)
	}
	for _, action := range plan(c, s, store.History, now) {
		// Policy and module flags are rechecked immediately before every mutation.
		if !configStillProtect() {
			break
		}
		// Persist the attempt BEFORE sending, including uncertain/time-out outcomes.
		// A crash/restart cannot erase the cooldown or cause an immediate retry storm.
		store.History.Attempts = append(store.History.Attempts, Attempt{now, string(action)})
		store.History = trimHistory(store.History, now)
		store.Status = s
		if e = saveJSON("state.json", store); e != nil {
			return c, store, errors.New("attempt_not_persisted_no_action")
		}
		actionErr := errEvidence
		switch action {
		case coldStart:
			if t == nil && s.MissingConfirmed && c.ColdApproved {
				actionErr = bootstrap(p)
			}
		case wakeWeb, wakeMac:
			if t != nil {
				actionErr = nativeWake(action, *t, p)
			}
		}
		if actionErr != nil {
			addEvent(&store, "recovery_unconfirmed", string(action)+"_failed_or_unknown_no_immediate_retry")
		} else {
			kind := "web_wake_requested"
			if action == wakeMac {
				kind = "mac_wake_requested"
			}
			if action == coldStart {
				kind = "daemon_bootstrap_requested"
			}
			addEvent(&store, kind, "request_sent_verification_pending")
		}
		// A request being accepted is NOT an 'online' result. Recollect actual state.
		time.Sleep(400 * time.Millisecond)
		s, t, p = collect()
	}
	s.WorkerRunning = workerRunning()
	store.Status = s
	if e = saveJSON("state.json", store); e != nil {
		return c, store, errors.New("status_not_saved")
	}
	return c, store, nil
}
func startWorker() error {
	if unavailableModule() {
		return errors.New("module_disabled")
	}
	if workerRunning() {
		return nil
	}
	if _, e := os.Stat(filepath.Join(dataDir, "stop")); e == nil {
		// A stop that timed out remains a persistent revocation, even on start.
		c, _ := loadConfig()
		c.Mode = "observe"
		if e = saveJSON("config.json", c); e != nil {
			return e
		}
		if e = os.Remove(filepath.Join(dataDir, "stop")); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	self, e := os.Executable()
	if e != nil {
		return e
	}
	return launchWorker(self)
}
func worker() error {
	lock, e := acquire("worker.lock")
	if e != nil {
		return nil
	}
	defer release(lock)
	// No wakelock or exact alarms. Deep sleep can delay checks; do not wake the
	// phone merely to renew access. Single lock prevents duplicate workers.
	next := time.Time{}
	for {
		if unavailableModule() {
			return nil
		}
		if _, e = os.Stat(filepath.Join(dataDir, "stop")); e == nil {
			return nil
		}
		if !time.Now().Before(next) {
			c, _, err := tick()
			if err != nil {
				c, _ = loadConfig()
			}
			next = time.Now().Add(time.Duration(c.Interval) * time.Second)
		}
		time.Sleep(5 * time.Second)
	}
}
func pauseAndStop() error {
	// Mark stop FIRST so an in-progress tick cannot launch its next action.
	if e := os.WriteFile(filepath.Join(dataDir, "stop"), []byte("stop\n"), 0600); e != nil {
		return e
	}
	deadline := time.Now().Add(10 * time.Second)
	var lock *os.File
	var e error
	for time.Now().Before(deadline) {
		lock, e = acquire("control.lock")
		if e == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if e != nil {
		return errors.New("check_in_progress_retry_after_it_finishes")
	}
	c, _ := loadConfig()
	c.Mode = "observe"
	if e = saveJSON("config.json", c); e != nil {
		release(lock)
		return e
	}
	release(lock)
	end := time.Now().Add(12 * time.Second)
	for time.Now().Before(end) {
		if !workerRunning() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("worker_stopping_not_yet_confirmed")
}
func changePolicy(mode string) error {
	if mode != "protect" && mode != "observe" {
		return errors.New("invalid_mode")
	}
	lock, e := acquire("control.lock")
	if e != nil {
		return errors.New("check_in_progress")
	}
	defer release(lock)
	c, e := loadConfig()
	if e != nil {
		return e
	}
	if mode == "protect" {
		if _, e = loadStore(); e != nil {
			return errors.New("history_unavailable_run_observation_check_first")
		}
		if unavailableModule() {
			return errors.New("module_disabled")
		}
		s, t, _ := collect()
		if !s.Supported {
			return errors.New("unsupported_app_version")
		}
		// An existing verified Agent identity (shell or root) is required before enabling cold repair.
		if t != nil && s.Daemon.Responsive && acceptedAgentIdentity(t.Process) && privateAgentData() {
			if e = rememberCertificate(); e != nil {
				return errors.New("trusted_identity_not_saved")
			}
			c.ColdApproved = true
		}
		if !c.ColdApproved {
			return errors.New("open_andromeld_root_service_once_before_enabling")
		}
	}
	c.Mode = mode
	if e = saveJSON("config.json", c); e != nil {
		return e
	}
	// Only this explicit user policy command can re-authorize a new installation.
	if mode == "protect" {
		if e = os.Remove(filepath.Join(moduleDir(), "reauthorize")); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	if e = os.Remove(filepath.Join(dataDir, "stop")); e != nil && !os.IsNotExist(e) {
		return e
	}
	return nil
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "_rpc" {
		os.Exit(rpcChild(os.Args[2:]))
	}
	if e := secureData(); e != nil {
		emit(map[string]any{"ok": false, "error": "android_root_environment_unavailable"})
		os.Exit(2)
	}
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"view"}
	}
	var err error
	switch args[0] {
	case "view":
		if len(args) != 1 {
			err = errors.New("invalid_arguments")
			break
		}
		c, e := loadConfig()
		if e != nil {
			err = e
			break
		}
		outputView(c, cachedStore())
		return
	case "tick":
		if len(args) != 1 {
			err = errors.New("invalid_arguments")
			break
		}
		c, s, e := tick()
		if e != nil {
			err = e
			break
		}
		outputView(c, s)
		return
	case "policy":
		if len(args) != 2 {
			err = errors.New("invalid_arguments")
			break
		}
		err = changePolicy(args[1])
		if err == nil {
			err = startWorker()
		}
	case "interval":
		if len(args) != 2 {
			err = errors.New("invalid_arguments")
			break
		}
		n, e := strconv.Atoi(args[1])
		if e != nil || (n != 60 && n != 120 && n != 300) {
			err = errors.New("invalid_interval")
			break
		}
		lock, e := acquire("control.lock")
		if e != nil {
			err = errors.New("check_in_progress")
			break
		}
		c, e := loadConfig()
		if e == nil {
			c.Interval = n
			e = saveJSON("config.json", c)
		}
		release(lock)
		err = e
	case "start":
		if len(args) != 1 {
			err = errors.New("invalid_arguments")
		} else {
			err = startWorker()
		}
	case "stop":
		if len(args) != 1 {
			err = errors.New("invalid_arguments")
		} else {
			err = pauseAndStop()
		}
	case "worker":
		if len(args) != 1 {
			err = errors.New("invalid_arguments")
		} else {
			err = worker()
		}
		if err == nil {
			return
		}
	default:
		err = errors.New("unknown_command")
	}
	if err != nil {
		// Only our fixed command/state codes, never syscall paths or raw server errors.
		msg := err.Error()
		safe := map[string]bool{"invalid_arguments": true, "invalid_mode": true, "unsupported_app_version": true, "open_andromeld_root_service_once_before_enabling": true, "module_disabled": true, "check_in_progress": true, "invalid_interval": true, "invalid_config_observation_only": true, "unknown_command": true, "check_in_progress_retry_after_it_finishes": true, "worker_stopping_not_yet_confirmed": true, "attempt_not_persisted_no_action": true, "status_not_saved": true, "trusted_identity_not_saved": true, "history_unavailable_run_observation_check_first": true}
		if !safe[msg] || strings.ContainsAny(msg, "\r\n") {
			msg = "operation_unconfirmed"
		}
		emit(map[string]any{"ok": false, "error": msg})
		os.Exit(2)
	}
	c, _ := loadConfig()
	outputView(c, cachedStore())
}
