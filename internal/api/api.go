// Package api implements the HTTP API of the system extension.
package api

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/sys/unix"

	"github.com/Pukka922/edelweiss-ext-system/internal/procfs"
	"github.com/Pukka922/edelweiss/sdk"
)

// Service holds the data sources; tests replace them.
type Service struct {
	FS           procfs.FS
	Statfs       func(path string) (total, used uint64, err error)
	Uname        func() (kernel, arch string, err error)
	Hostname     func() (string, error)
	SetHostname  func(name string) error
	HostnameFile string
	Interval     time.Duration
	Now          func() time.Time
	maxEvents    int // tests: end the metrics stream after this many events (0 = never)

	mu sync.Mutex // serializes hostname changes
}

// NewService reads the real host.
func NewService() *Service {
	return &Service{
		FS:           procfs.New(),
		Statfs:       statfs,
		Uname:        uname,
		Hostname:     os.Hostname,
		SetHostname:  func(name string) error { return unix.Sethostname([]byte(name)) },
		HostnameFile: "/etc/hostname",
		Interval:     2 * time.Second,
		Now:          time.Now,
	}
}

func statfs(path string) (uint64, uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	// the block counts are in units of the fragment size (statfs(2) f_frsize); old kernels report 0
	size := uint64(st.Frsize)
	if size == 0 {
		size = uint64(st.Bsize)
	}
	return st.Blocks * size, (st.Blocks - st.Bfree) * size, nil
}

func uname() (string, string, error) {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return "", "", err
	}
	return unix.ByteSliceToString(u.Release[:]), unix.ByteSliceToString(u.Machine[:]), nil
}

// Register adds the routes to app.
func Register(app *fiber.App, s *Service) {
	app.Get("/info", s.info)
	app.Get("/metrics", sdk.SSE(s.metrics))
	app.Put("/hostname", sdk.RequireAdmin(), s.setHostname)
}

type OSInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	PrettyName string `json:"prettyName"`
}

type CPUInfo struct {
	Model string `json:"model"`
	Cores int    `json:"cores"`
}

type Info struct {
	Hostname      string    `json:"hostname"`
	OS            OSInfo    `json:"os"`
	Kernel        string    `json:"kernel"`
	Arch          string    `json:"arch"`
	CPU           CPUInfo   `json:"cpu"`
	MemoryTotal   uint64    `json:"memoryTotal"`
	BootTime      time.Time `json:"bootTime"`
	UptimeSeconds int64     `json:"uptimeSeconds"`
}

func failed(what string, err error) error {
	slog.Error("reading system information failed", "what", what, "err", err)
	return sdk.NewError(fiber.StatusInternalServerError, "system_info_failed", "could not read "+what)
}

func (s *Service) info(c fiber.Ctx) error {
	host, err := s.Hostname()
	if err != nil {
		return failed("hostname", err)
	}
	kernel, arch, err := s.Uname()
	if err != nil {
		return failed("kernel version", err)
	}
	mem, err := s.FS.MemInfo()
	if err != nil {
		return failed("memory information", err)
	}
	up, err := s.FS.Uptime()
	if err != nil {
		return failed("uptime", err)
	}
	osr, err := s.FS.OSRelease()
	if err != nil {
		osr = procfs.OSRelease{Name: "Linux", PrettyName: "Linux"}
	}
	cpu, _ := s.FS.CPUInfo()
	up = up.Truncate(time.Second)
	return c.JSON(Info{
		Hostname:      host,
		OS:            OSInfo{Name: osr.Name, Version: osr.Version, PrettyName: osr.PrettyName},
		Kernel:        kernel,
		Arch:          arch,
		CPU:           CPUInfo{Model: cpu.Model, Cores: cpu.Cores},
		MemoryTotal:   mem.Total,
		BootTime:      s.Now().Add(-up).UTC().Truncate(time.Second),
		UptimeSeconds: int64(up / time.Second),
	})
}
