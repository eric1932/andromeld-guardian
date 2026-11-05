package main

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestUpgradeMarkerAlwaysRevokesOldProtection(t *testing.T) {
	c, _, _, _ := fixture()
	got := consentConfig(c, true)
	if got.Mode != "observe" {
		t.Fatal("staged upgrade inherited old protection")
	}
	if consentConfig(c, false).Mode != "protect" {
		t.Fatal("explicitly authorized policy lost")
	}
}
func TestLateStopGatePreventsDispatch(t *testing.T) {
	allowed := true
	calls := 0
	// Simulate an expensive preparation that was followed by stop/disable/remove.
	allowed = false
	err := dispatchWhen(func() bool { return allowed }, func() error { calls++; return nil })
	if err == nil || calls != 0 {
		t.Fatal("sent after protection was revoked")
	}
}
func TestDispatchNeverRetriesUnknownResult(t *testing.T) {
	calls := 0
	err := dispatchWhen(func() bool { return true }, func() error { calls++; return errors.New("transport result unknown") })
	if err == nil || calls != 1 {
		t.Fatal("repeated mutation", calls)
	}
}
func trustedStore() Store {
	_, s, _, n := fixture()
	s.CheckedAt = n
	return Store{Status: s, Events: []Event{}, History: History{Attempts: []Attempt{{n, string(wakeWeb)}}}}
}
func TestHistoryMustContainTrustedBoundedEvidence(t *testing.T) {
	valid, _ := json.Marshal(trustedStore())
	if _, e := decodeStore(valid); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{
		`{}`, `{"history":{}}`, `{"status":{},"history":{"missing_samples":0,"attempts":[]}}`,
		`{"history":{"missing_samples":0,"attempts":null}}`, string(valid[:len(valid)-2]),
	} {
		if _, e := decodeStore([]byte(raw)); e == nil {
			t.Fatal("corrupt/missing history accepted", raw)
		}
	}
	for _, mutate := range []func(*Store){
		func(s *Store) { s.History.Attempts = nil },
		func(s *Store) { s.History.MissingSamples = 4 },
		func(s *Store) { s.History.Attempts[0].Kind = "unknown_command" },
		func(s *Store) { s.History.Attempts = make([]Attempt, 151) },
	} {
		s := trustedStore()
		mutate(&s)
		b, _ := json.Marshal(s)
		if _, e := decodeStore(b); e == nil {
			t.Fatal("invalid history accepted")
		}
	}
}
func TestUnknownHistoryCannotSelfResetInProtect(t *testing.T) {
	c, _, _, _ := fixture()
	if historyAllows(c, errors.New("read error")) {
		t.Fatal("lost cooldowns while still protected")
	}
	if !historyAllows(c, nil) {
		t.Fatal("trusted history refused")
	}
	c.Mode = "observe"
	if !historyAllows(c, errors.New("first observation")) {
		t.Fatal("cannot initialize in observation")
	}
}
