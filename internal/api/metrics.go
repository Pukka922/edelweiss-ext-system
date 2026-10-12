package api

import (
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/sse"

	"github.com/Pukka922/edelweiss-ext-system/internal/procfs"
)

type Memory struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
	Used      uint64 `json:"used"`
	SwapTotal uint64 `json:"swapTotal"`
	SwapUsed  uint64 `json:"swapUsed"`
}

type Disk struct {
	Mount  string `json:"mount"`
	Device string `json:"device"`
	FSType string `json:"fsType"`
	Total  uint64 `json:"total"`
	Used   uint64 `json:"used"`
}

type NetRate struct {
	Iface string  `json:"iface"`
	RxBps float64 `json:"rxBps"`
	TxBps float64 `json:"txBps"`
}

type Metrics struct {
	Time   time.Time  `json:"time"`
	CPU    float64    `json:"cpu"`
	Load   [3]float64 `json:"load"`
	Memory Memory     `json:"memory"`
	Disks  []Disk     `json:"disks"`
	Net    []NetRate  `json:"net"`
}

// sampler turns counters into rates between two samples.
type sampler struct {
	s      *Service
	cpu    procfs.CPUTimes
	net    map[string]procfs.NetCounters
	at     time.Time
	primed bool
}

func (sm *sampler) sample() (Metrics, error) {
	now := sm.s.Now()
	cpu, err := sm.s.FS.CPUTimes()
	if err != nil {
		return Metrics{}, err
	}
	net, err := sm.s.FS.NetDev()
	if err != nil {
		return Metrics{}, err
	}
	mem, err := sm.s.FS.MemInfo()
	if err != nil {
		return Metrics{}, err
	}
	m := Metrics{
		Time: now.UTC(),
		Memory: Memory{
			Total: mem.Total, Available: mem.Available, Used: mem.Total - min(mem.Available, mem.Total),
			SwapTotal: mem.SwapTotal, SwapUsed: mem.SwapTotal - min(mem.SwapFree, mem.SwapTotal),
		},
		Disks: []Disk{},
		Net:   []NetRate{},
	}
	if load, err := sm.s.FS.LoadAvg(); err == nil {
		m.Load = load
	}
	if mounts, err := sm.s.FS.Mounts(); err == nil {
		for _, mnt := range mounts {
			total, used, err := sm.s.Statfs(mnt.Path)
			if err != nil || total == 0 {
				continue
			}
			m.Disks = append(m.Disks, Disk{Mount: mnt.Path, Device: mnt.Device, FSType: mnt.Type, Total: total, Used: used})
		}
	}
	dt := now.Sub(sm.at).Seconds()
	if sm.primed {
		m.CPU = math.Round(procfs.CPUUsage(sm.cpu, cpu)*10) / 10
	}
	names := make([]string, 0, len(net))
	for name := range net {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r := NetRate{Iface: name}
		if prev, ok := sm.net[name]; ok && sm.primed && dt > 0 {
			r.RxBps = rate(prev.RxBytes, net[name].RxBytes, dt)
			r.TxBps = rate(prev.TxBytes, net[name].TxBytes, dt)
		}
		m.Net = append(m.Net, r)
	}
	sm.cpu, sm.net, sm.at, sm.primed = cpu, net, now, true
	return m, nil
}

func rate(prev, cur uint64, seconds float64) float64 {
	if cur < prev { // counter reset
		return 0
	}
	return math.Round(float64(cur-prev) / seconds)
}

// metrics streams a Metrics event shortly after connecting and then every Interval.
func (s *Service) metrics(_ fiber.Ctx, stream *sse.Stream) error {
	sm := &sampler{s: s}
	// the 200 headers are already sent and nothing reads the returned error: log it here
	if _, err := sm.sample(); err != nil {
		slog.Error("sampling metrics failed", "err", err)
		return err
	}
	timer := time.NewTimer(min(s.Interval, 500*time.Millisecond))
	defer timer.Stop()
	for sent := 0; s.maxEvents == 0 || sent < s.maxEvents; sent++ {
		select {
		case <-stream.Done():
			return nil
		case <-timer.C:
		}
		m, err := sm.sample()
		if err != nil {
			slog.Error("sampling metrics failed", "err", err)
			return err
		}
		if err := stream.Event(sse.Event{Name: "metrics", Data: m}); err != nil {
			return nil // client gone
		}
		timer.Reset(s.Interval)
	}
	return nil
}
