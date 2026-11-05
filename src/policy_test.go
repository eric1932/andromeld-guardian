package main

import (
	"bufio"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixture() (Config, Status, History, time.Time) {
	c := defaultConfig()
	c.Mode = "protect"
	c.ColdApproved = true
	s := Status{Supported: true, Daemon: DaemonStatus{PID: 1234, Alive: true, Responsive: true, Build: supportedBuild}, Web: WebStatus{Enabled: true}, Mac: MacStatus{Active: true}}
	return c, s, History{}, time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
}
func TestObserveNeverRepairs(t *testing.T) {
	c, s, h, n := fixture()
	c.Mode = "observe"
	s.Web.Enabled = false
	s.MissingConfirmed = true
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("observation mutated")
	}
}
func TestUnknownBuildBlocked(t *testing.T) {
	c, s, h, n := fixture()
	s.Supported = false
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("unknown build mutated")
	}
}
func TestNoClientsIsNotDaemonFailure(t *testing.T) {
	c, s, h, n := fixture()
	a := plan(c, s, h, n)
	if !reflect.DeepEqual(a, []Action{wakeWeb, wakeMac}) {
		t.Fatal(a)
	}
	for _, v := range a {
		if v == coldStart {
			t.Fatal("restarted healthy daemon")
		}
	}
}
func TestConnectedSessionsUntouched(t *testing.T) {
	c, s, h, n := fixture()
	s.Web.Connected = true
	s.Mac.Connected = true
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("disturbed connected users")
	}
}
func TestConnectedDeviceMacUntouched(t *testing.T) {
	c, s, h, n := fixture()
	s.Web.Connected = true
	s.Mac.DeviceConnected = true
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("disturbed device link")
	}
}
func TestUnresponsiveLiveDaemonNeverKilled(t *testing.T) {
	c, s, h, n := fixture()
	s.Daemon.Responsive = false
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("live process repair")
	}
}
func TestMissingUnconfirmedNeverColdStarts(t *testing.T) {
	c, s, h, n := fixture()
	s.Daemon.Alive = false
	h.MissingSamples = 10
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("absence not proven")
	}
}
func TestColdRequiresThreeSamples(t *testing.T) {
	c, s, h, n := fixture()
	s.Daemon = DaemonStatus{}
	s.MissingConfirmed = true
	h.MissingSamples = 2
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("premature cold start")
	}
	h.MissingSamples = 3
	if !reflect.DeepEqual(plan(c, s, h, n), []Action{coldStart}) {
		t.Fatal("missing restore")
	}
}
func TestColdRequiresKnownRootIdentity(t *testing.T) {
	c, s, h, n := fixture()
	c.ColdApproved = false
	s.Daemon = DaemonStatus{}
	s.MissingConfirmed = true
	h.MissingSamples = 10
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("invented new identity")
	}
}
func TestColdNeverWhenAliveEvenIfAbsenceFlagStale(t *testing.T) {
	c, s, h, n := fixture()
	s.MissingConfirmed = true
	h.MissingSamples = 3
	for _, a := range plan(c, s, h, n) {
		if a == coldStart {
			t.Fatal("cold started over live daemon")
		}
	}
}
func TestWakeCooldown(t *testing.T) {
	c, s, h, n := fixture()
	h.Attempts = []Attempt{{n.Add(-time.Minute), string(wakeWeb)}, {n.Add(-2 * time.Minute), string(wakeMac)}}
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("wake storm")
	}
}
func TestWakeRenewsAfterThreeMinutes(t *testing.T) {
	c, s, h, n := fixture()
	h.Attempts = []Attempt{{n.Add(-3 * time.Minute), string(wakeWeb)}, {n.Add(-3 * time.Minute), string(wakeMac)}}
	if len(plan(c, s, h, n)) != 2 {
		t.Fatal("not renewed")
	}
}
func TestColdCooldownTenMinutes(t *testing.T) {
	c, s, h, n := fixture()
	s.Daemon = DaemonStatus{}
	s.MissingConfirmed = true
	h.MissingSamples = 3
	h.Attempts = []Attempt{{n.Add(-9 * time.Minute), string(coldStart)}}
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("bootstrap storm")
	}
}
func TestColdHourlyLimit(t *testing.T) {
	c, s, h, n := fixture()
	s.Daemon = DaemonStatus{}
	s.MissingConfirmed = true
	h.MissingSamples = 3
	for _, v := range []int{11, 25, 50} {
		h.Attempts = append(h.Attempts, Attempt{n.Add(-time.Duration(v) * time.Minute), string(coldStart)})
	}
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("hourly cap bypassed")
	}
}
func TestBackwardsClockBlocksMutation(t *testing.T) {
	c, s, h, n := fixture()
	h.Attempts = []Attempt{{n.Add(time.Minute), string(wakeWeb)}, {n.Add(time.Minute), string(wakeMac)}}
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("clock change caused immediate retry")
	}
}
func TestHistoryPrunedAndBounded(t *testing.T) {
	_, _, h, n := fixture()
	for i := 0; i < 300; i++ {
		h.Attempts = append(h.Attempts, Attempt{n.Add(-time.Duration(i) * time.Minute), string(wakeWeb)})
	}
	h = trimHistory(h, n)
	if len(h.Attempts) > 150 {
		t.Fatal("unbounded history")
	}
}
func TestNoPairingOrKillActionsExist(t *testing.T) {
	c, s, h, n := fixture()
	for _, a := range plan(c, s, h, n) {
		if a != wakeWeb && a != wakeMac && a != coldStart {
			t.Fatal(a)
		}
	}
}
func TestInvalidConfigFailsClosed(t *testing.T) {
	c, s, h, n := fixture()
	c.Interval = 1
	if len(plan(c, s, h, n)) != 0 {
		t.Fatal("invalid configuration ran")
	}
}
func TestStrictRPCSchema(t *testing.T) {
	good := `{"ok":true,"enabled":true,"connected":false,"macEnabled":false,"agentVersionCode":26101400,"port":13012}`
	if _, e := decodeRPC([]byte(good), "web.status"); e != nil {
		t.Fatal(e)
	}
	for _, data := range []string{`{}`, strings.Replace(good, `"enabled":true`, `"enabled":null`, 1), strings.Replace(good, `"connected":false`, `"connected":"false"`, 1), strings.Replace(good, `"port":13012`, `"port":1.2`, 1), strings.Replace(good, `"ok":true`, `"ok":false`, 1)} {
		if _, e := decodeRPC([]byte(data), "web.status"); e == nil {
			t.Fatal("accepted incomplete schema", data)
		}
	}
}
func TestMacSchema(t *testing.T) {
	good := `{"ok":true,"active":true,"connected":false,"deviceConnected":false}`
	if _, e := decodeRPC([]byte(good), "mac.link"); e != nil {
		t.Fatal(e)
	}
	if _, e := decodeRPC([]byte(good), "web.toggle"); e == nil {
		t.Fatal("arbitrary operation")
	}
}
func TestSecretsNeverForwarded(t *testing.T) {
	good := `{"ok":true,"active":true,"connected":false,"deviceConnected":false,"connectedName":"PRIVATE_NAME","token":"SYNTHETIC_SECRET","url":"https://example.invalid/?key=SECRET"}`
	v, e := decodeRPC([]byte(good), "mac.link")
	if e != nil {
		t.Fatal(e)
	}
	out, _ := json.Marshal(v)
	if strings.Contains(string(out), "SECRET") || strings.Contains(string(out), "PRIVATE_NAME") {
		t.Fatal(string(out))
	}
}
func TestBoundedFrame(t *testing.T) {
	if _, e := readLine(bufio.NewReader(strings.NewReader(strings.Repeat("x", 10000)+"\n")), 8192); e == nil {
		t.Fatal("oversize")
	}
	if _, e := readLine(bufio.NewReader(strings.NewReader("unterminated")), 100); e == nil {
		t.Fatal("EOF without frame accepted")
	}
}
func TestEstablishedRequiresServerPortClientPortAndOwnedInode(t *testing.T) {
	row := "header\n0: 0100007F:32DE 0100007F:C350 01 0 0 0 0 0 123\n"
	ids := map[string]bool{"123": true}
	if !socketMatch(row, ids, 13022, 50000, true) {
		t.Fatal("valid tuple failed")
	}
	if socketMatch(row, ids, 13022, 50001, true) || socketMatch(row, map[string]bool{"456": true}, 13022, 50000, true) || socketMatch(strings.Replace(row, " 01 ", " 0A ", 1), ids, 13022, 50000, true) {
		t.Fatal("wrong tuple accepted")
	}
}
func TestListenerRequiresLoopbackNotWildcard(t *testing.T) {
	row := "0: 0100007F:32DE 00000000:0000 0A 0 0 0 0 0 123"
	ids := map[string]bool{"123": true}
	if !socketMatch(row, ids, 13022, 0, false) {
		t.Fatal("valid listener")
	}
	if socketMatch(strings.Replace(row, "0100007F", "00000000", 1), ids, 13022, 0, false) {
		t.Fatal("wildcard accepted")
	}
}
func TestDefaultDoesNotAutoProtect(t *testing.T) {
	c := defaultConfig()
	if c.Mode != "observe" || c.ColdApproved {
		t.Fatal(c)
	}
}
