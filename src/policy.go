package main

import "time"

const version = "0.1.0"
const supportedBuild = 26101400

type Config struct {
	Mode         string `json:"mode"`
	Interval     int    `json:"interval"`
	KeepWeb      bool   `json:"keep_web"`
	KeepMac      bool   `json:"keep_mac"`
	ColdApproved bool   `json:"cold_approved"`
}

func defaultConfig() Config {
	return Config{Mode: "observe", Interval: 60, KeepWeb: true, KeepMac: true}
}
func validConfig(c Config) bool {
	return (c.Mode == "observe" || c.Mode == "protect") && (c.Interval == 60 || c.Interval == 120 || c.Interval == 300) && c.KeepWeb && c.KeepMac
}

type DaemonStatus struct {
	PID        int  `json:"pid"`
	Alive      bool `json:"alive"`
	Responsive bool `json:"responsive"`
	Build      int  `json:"build"`
}
type WebStatus struct {
	Enabled   bool `json:"enabled"`
	Connected bool `json:"connected"`
	Port      int  `json:"port"`
}
type MacStatus struct {
	Active          bool `json:"active"`
	Connected       bool `json:"connected"`
	DeviceConnected bool `json:"device_connected"`
	Pairing         bool `json:"pairing"`
}
type Status struct {
	CheckedAt        time.Time    `json:"checked_at"`
	Daemon           DaemonStatus `json:"daemon"`
	Web              WebStatus    `json:"web"`
	Mac              MacStatus    `json:"mac"`
	Supported        bool         `json:"supported"`
	WorkerRunning    bool         `json:"worker_running"`
	Health           string       `json:"health"`
	Reason           string       `json:"reason"`
	MissingConfirmed bool         `json:"missing_confirmed"`
}
type Event struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Detail string    `json:"detail"`
}
type Attempt struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
}
type History struct {
	MissingSamples int       `json:"missing_samples"`
	Attempts       []Attempt `json:"attempts"`
}
type Action string

const (
	wakeWeb   Action = "wake_web"
	wakeMac   Action = "wake_mac"
	coldStart Action = "cold_start"
)

// Pure, unit-testable decisions. No connection is NOT a daemon failure.
// Protect mode renews the app's own idle window through its native wake path.
// It NEVER opens pairing, restarts a live process, or kills any process.
func plan(c Config, s Status, h History, now time.Time) []Action {
	if !validConfig(c) || c.Mode != "protect" || !s.Supported {
		return nil
	}
	if s.MissingConfirmed && !s.Daemon.Alive {
		if c.ColdApproved && h.MissingSamples >= 3 && eligible(h, coldStart, now, 10*time.Minute, 3) {
			return []Action{coldStart}
		}
		return nil
	}
	if !s.Daemon.Alive || !s.Daemon.Responsive {
		return nil
	}
	actions := []Action{}
	if !s.Web.Connected && eligible(h, wakeWeb, now, 3*time.Minute, 24) {
		actions = append(actions, wakeWeb)
	}
	if !s.Mac.Connected && !s.Mac.DeviceConnected && eligible(h, wakeMac, now, 3*time.Minute, 24) {
		actions = append(actions, wakeMac)
	}
	return actions
}
func eligible(h History, kind Action, now time.Time, cooldown time.Duration, maximum int) bool {
	count := 0
	for _, a := range h.Attempts {
		if a.Kind != string(kind) {
			continue
		}
		elapsed := now.Sub(a.At)
		// A backwards wall-clock jump is handled conservatively (no mutations).
		if elapsed < 0 {
			return false
		}
		if elapsed < cooldown {
			return false
		}
		if elapsed < time.Hour {
			count++
		}
	}
	return count < maximum
}
func trimHistory(h History, now time.Time) History {
	out := []Attempt{}
	for _, a := range h.Attempts {
		if now.Sub(a.At) < 24*time.Hour {
			out = append(out, a)
		}
	}
	if len(out) > 150 {
		out = out[len(out)-150:]
	}
	h.Attempts = out
	return h
}
