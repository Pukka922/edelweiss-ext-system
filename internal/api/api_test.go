package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/Pukka922/edelweiss-ext-system/internal/procfs"
	"github.com/Pukka922/edelweiss/sdk"
	"github.com/Pukka922/edelweiss/sdk/sdktest"
)

var now = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

// TestMain keeps the expected error logs of the failure tests out of the test output.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

func testService(t *testing.T) (*Service, *[]string) {
	t.Helper()
	var set []string
	s := &Service{
		FS:           procfs.FS{Root: "../procfs/testdata"},
		Statfs:       func(string) (uint64, uint64, error) { return 1000, 250, nil },
		Uname:        func() (string, string, error) { return "6.6.50-0-lts", "x86_64", nil },
		Hostname:     func() (string, error) { return "alpine", nil },
		SetHostname:  func(n string) error { set = append(set, n); return nil },
		HostnameFile: filepath.Join(t.TempDir(), "hostname"),
		Interval:     10 * time.Millisecond,
		Now:          func() time.Time { return now },
	}
	return s, &set
}

func newApp(s *Service) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: sdk.ErrorHandler})
	Register(app, s)
	return app
}

var alice = sdk.User{Name: "alice", UID: 1000}
var admin = sdk.User{Name: "alice", UID: 1000, Admin: true}

func TestInfo(t *testing.T) {
	s, _ := testService(t)
	resp := sdktest.Do(t, newApp(s), sdktest.Request("GET", "/info", nil, alice))
	info := sdktest.DecodeJSON[Info](t, resp)
	want := Info{
		Hostname:      "alpine",
		OS:            OSInfo{Name: "Alpine Linux", Version: "3.24.2", PrettyName: "Alpine Linux v3.24"},
		Kernel:        "6.6.50-0-lts",
		Arch:          "x86_64",
		CPU:           CPUInfo{Model: "Intel(R) Core(TM) i5-8250U CPU @ 1.60GHz", Cores: 2},
		MemoryTotal:   2048000 * 1024,
		BootTime:      now.Add(-3725 * time.Second),
		UptimeSeconds: 3725,
	}
	if !info.BootTime.Equal(want.BootTime) {
		t.Fatalf("bootTime = %v, want %v", info.BootTime, want.BootTime)
	}
	info.BootTime = want.BootTime
	if info != want {
		t.Fatalf("info = %+v\nwant  %+v", info, want)
	}
}

func TestSamplerComputesRates(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		p := filepath.Join(root, name)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"proc/meminfo", "proc/loadavg", "proc/mounts"} {
		data, _ := os.ReadFile(filepath.Join("../procfs/testdata", f))
		write(f, string(data))
	}
	header := "Inter-|\n face |\n"
	write("proc/stat", "cpu  100 0 100 800 0 0 0 0 0 0\n")
	write("proc/net/dev", header+"  eth0: 1000 0 0 0 0 0 0 0 500 0 0 0 0 0 0 0\n")
	clock := now
	s, _ := testService(t)
	s.FS = procfs.FS{Root: root}
	s.Now = func() time.Time { return clock }
	sm := &sampler{s: s}
	if _, err := sm.sample(); err != nil {
		t.Fatal(err)
	}
	write("proc/stat", "cpu  150 0 150 900 0 0 0 0 0 0\n")
	write("proc/net/dev", header+"  eth0: 3000 0 0 0 0 0 0 0 1500 0 0 0 0 0 0 0\n")
	clock = clock.Add(2 * time.Second)
	m, err := sm.sample()
	if err != nil {
		t.Fatal(err)
	}
	if m.CPU != 50 {
		t.Fatalf("cpu = %v", m.CPU)
	}
	if len(m.Net) != 1 || m.Net[0] != (NetRate{Iface: "eth0", RxBps: 1000, TxBps: 500}) {
		t.Fatalf("net = %+v", m.Net)
	}
	if m.Memory.Used != (2048000-1024000)*1024 || m.Memory.SwapUsed != (1048576-786432)*1024 {
		t.Fatalf("memory = %+v", m.Memory)
	}
	if len(m.Disks) != 3 || m.Disks[0] != (Disk{Mount: "/", Device: "/dev/sda3", FSType: "ext4", Total: 1000, Used: 250}) {
		t.Fatalf("disks = %+v", m.Disks)
	}
}

func TestMetricsStream(t *testing.T) {
	s, _ := testService(t)
	s.maxEvents = 2
	resp := sdktest.Do(t, newApp(s), sdktest.Request("GET", "/metrics", nil, alice))
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)
	events := 0
	for sc.Scan() {
		line := sc.Text()
		if line == "event: metrics" {
			events++
		}
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var m Metrics
			if err := json.Unmarshal([]byte(data), &m); err != nil {
				t.Fatalf("bad event data %q: %v", data, err)
			}
			if m.Memory.Total == 0 || len(m.Disks) == 0 {
				t.Fatalf("incomplete metrics %+v", m)
			}
		}
	}
	if events != 2 {
		t.Fatalf("events = %d", events)
	}
}

func TestSetHostname(t *testing.T) {
	s, set := testService(t)
	app := newApp(s)
	body := func(h string) *strings.Reader { return strings.NewReader(`{"hostname":"` + h + `"}`) }
	if err := os.WriteFile(s.HostnameFile, []byte("alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fileIs := func(want string) {
		t.Helper()
		if data, _ := os.ReadFile(s.HostnameFile); string(data) != want {
			t.Fatalf("hostname file %q, want %q", data, want)
		}
	}

	resp := sdktest.Do(t, app, sdktest.Request("PUT", "/hostname", body("nas"), alice))
	if resp.StatusCode != 403 {
		t.Fatalf("non-admin: %d", resp.StatusCode)
	}
	resp = sdktest.Do(t, app, sdktest.Request("PUT", "/hostname", body("bad_host!"), admin))
	if resp.StatusCode != 400 {
		t.Fatalf("invalid: %d", resp.StatusCode)
	}
	if len(*set) != 0 {
		t.Fatalf("rejected requests changed the hostname: %v", *set)
	}
	fileIs("alpine\n")
	resp = sdktest.Do(t, app, sdktest.Request("PUT", "/hostname", body(" NAS-01.lan "), admin))
	got := sdktest.DecodeJSON[map[string]string](t, resp)
	if resp.StatusCode != 200 || got["hostname"] != "nas-01.lan" {
		t.Fatalf("set: %d %v", resp.StatusCode, got)
	}
	fileIs("nas-01.lan\n")
	if len(*set) != 1 || (*set)[0] != "nas-01.lan" {
		t.Fatalf("SetHostname calls %v", *set)
	}

	// a failing SetHostname must not touch /etc/hostname
	s.SetHostname = func(string) error { return errors.New("operation not permitted") }
	resp = sdktest.Do(t, app, sdktest.Request("PUT", "/hostname", body("nas"), admin))
	if resp.StatusCode != 500 {
		t.Fatalf("set failure: %d", resp.StatusCode)
	}
	fileIs("nas-01.lan\n")
}

// A failing file write rolls the kernel hostname back to the previous one.
func TestSetHostnameRestoresKernelHostnameWhenFileWriteFails(t *testing.T) {
	s, set := testService(t)
	s.HostnameFile = filepath.Join(t.TempDir(), "missing-dir", "hostname")
	resp := sdktest.Do(t, newApp(s),
		sdktest.Request("PUT", "/hostname", strings.NewReader(`{"hostname":"nas"}`), admin))
	if resp.StatusCode != 500 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got := sdktest.DecodeJSON[struct {
		Error struct{ Code string } `json:"error"`
	}](t, resp)
	if got.Error.Code != "hostname_write_failed" {
		t.Fatalf("error code %q", got.Error.Code)
	}
	if want := []string{"nas", "alpine"}; !slices.Equal(*set, want) {
		t.Fatalf("SetHostname calls %v, want %v", *set, want)
	}
}

// Concurrent requests must not interleave; the race detector flags the unguarded SetHostname data.
func TestSetHostnameSerializesConcurrentRequests(t *testing.T) {
	s, set := testService(t)
	app := newApp(s)
	const n = 8
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			req := sdktest.Request("PUT", "/hostname", strings.NewReader(`{"hostname":"nas"}`), admin)
			resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second, FailOnTimeout: true})
			if err != nil {
				t.Error(err)
				return
			}
			if resp.StatusCode != 200 {
				t.Errorf("status %d", resp.StatusCode)
			}
			_ = resp.Body.Close()
		})
	}
	wg.Wait()
	if len(*set) != n {
		t.Fatalf("SetHostname calls %d, want %d", len(*set), n)
	}
}

func TestValidHostname(t *testing.T) {
	for _, h := range []string{
		"a", "nas-01", "host.example.lan", "host1.lan", strings.Repeat("a", 63),
		strings.Repeat("a", 60) + ".lan", // exactly 64: the kernel limit (HOST_NAME_MAX)
	} {
		if !ValidHostname(h) {
			t.Errorf("%q should be valid", h)
		}
	}
	for _, h := range []string{
		"", "-a", "a-", "a_b", "A", "a..b", strings.Repeat("a", 64),
		strings.Repeat("a", 61) + ".lan", // 65: the kernel rejects it with EINVAL
		strings.Repeat("a", 63) + ".lan", // 67
		strings.Repeat("a.", 127) + "a",
		"192.168.1.1", "123", // last label all digits (RFC 1123 section 2.1)
	} {
		if ValidHostname(h) {
			t.Errorf("%q should be invalid", h)
		}
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// The 200 headers are already sent when sampling fails, so the log is the only trace.
func TestMetricsLogsSamplingError(t *testing.T) {
	var logs lockedBuffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.DiscardHandler)) })
	s, _ := testService(t)
	s.FS = procfs.FS{Root: t.TempDir()} // no /proc files: the first sample fails
	resp := sdktest.Do(t, newApp(s), sdktest.Request("GET", "/metrics", nil, alice))
	if _, err := io.Copy(io.Discard, resp.Body); err != nil { // wait for the stream to end
		t.Fatal(err)
	}
	if out := logs.String(); !strings.Contains(out, "sampling metrics failed") {
		t.Fatalf("sampling error not logged: %q", out)
	}
}

func TestStatfs(t *testing.T) {
	total, used, err := statfs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 || used > total {
		t.Fatalf("total %d, used %d", total, used)
	}
}
