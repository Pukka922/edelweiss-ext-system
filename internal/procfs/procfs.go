// Package procfs reads system information from /proc and /etc.
package procfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// FS reads below Root: "/" on a real host, a testdata directory in tests.
type FS struct{ Root string }

func New() FS { return FS{Root: "/"} }

func (f FS) path(p string) string { return filepath.Join(f.Root, p) }

func (f FS) lines(p string) ([]string, error) {
	data, err := os.ReadFile(f.path(p))
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n"), nil
}

// CPUTimes are the jiffies of all CPUs together.
type CPUTimes struct{ Total, Idle uint64 }

func (f FS) CPUTimes() (CPUTimes, error) {
	lines, err := f.lines("proc/stat")
	if err != nil {
		return CPUTimes{}, err
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var t CPUTimes
		// user nice system idle iowait irq softirq steal; guest values are already part of user/nice
		for i, v := range fields[1:min(len(fields), 9)] {
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return CPUTimes{}, fmt.Errorf("proc/stat: %w", err)
			}
			t.Total += n
			if i == 3 || i == 4 {
				t.Idle += n
			}
		}
		return t, nil
	}
	return CPUTimes{}, errors.New("proc/stat: no cpu line")
}

// CPUUsage is the busy share between two samples in percent (0–100).
func CPUUsage(prev, cur CPUTimes) float64 {
	total := float64(cur.Total) - float64(prev.Total)
	if total <= 0 {
		return 0
	}
	idle := float64(cur.Idle) - float64(prev.Idle)
	return min(max((total-idle)/total*100, 0), 100)
}

// MemInfo values are in bytes.
type MemInfo struct{ Total, Available, SwapTotal, SwapFree uint64 }

func (f FS) MemInfo() (MemInfo, error) {
	lines, err := f.lines("proc/meminfo")
	if err != nil {
		return MemInfo{}, err
	}
	values := map[string]uint64{}
	for _, line := range lines {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		if len(fields) > 1 && fields[1] == "kB" {
			n *= 1024
		}
		values[name] = n
	}
	if values["MemTotal"] == 0 {
		return MemInfo{}, errors.New("proc/meminfo: MemTotal missing")
	}
	m := MemInfo{Total: values["MemTotal"], Available: values["MemAvailable"], SwapTotal: values["SwapTotal"], SwapFree: values["SwapFree"]}
	if _, ok := values["MemAvailable"]; !ok {
		m.Available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	return m, nil
}

func (f FS) LoadAvg() ([3]float64, error) {
	lines, err := f.lines("proc/loadavg")
	if err != nil {
		return [3]float64{}, err
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 3 {
		return [3]float64{}, errors.New("proc/loadavg: too few fields")
	}
	var load [3]float64
	for i := range load {
		if load[i], err = strconv.ParseFloat(fields[i], 64); err != nil {
			return [3]float64{}, fmt.Errorf("proc/loadavg: %w", err)
		}
	}
	return load, nil
}

func (f FS) Uptime() (time.Duration, error) {
	lines, err := f.lines("proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(lines[0])
	if len(fields) == 0 {
		return 0, errors.New("proc/uptime: empty")
	}
	sec, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("proc/uptime: %w", err)
	}
	return time.Duration(sec * float64(time.Second)), nil
}

type NetCounters struct{ RxBytes, TxBytes uint64 }

// NetDev returns the byte counters of all interfaces except loopback.
func (f FS) NetDev() (map[string]NetCounters, error) {
	lines, err := f.lines("proc/net/dev")
	if err != nil {
		return nil, err
	}
	out := map[string]NetCounters{}
	for _, line := range lines[min(2, len(lines)):] {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		fields := strings.Fields(rest)
		if name == "lo" || len(fields) < 9 {
			continue
		}
		rx, err1 := strconv.ParseUint(fields[0], 10, 64)
		tx, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out[name] = NetCounters{RxBytes: rx, TxBytes: tx}
	}
	return out, nil
}

type Mount struct{ Device, Path, Type string }

var realFS = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "vfat": true,
	"exfat": true, "f2fs": true, "zfs": true, "ntfs3": true, "jfs": true,
}

// Mounts returns mounted real file systems in mount order. The last entry per
// mount point wins (it is the visible one), whatever its type: a tmpfs over an
// ext4 mount point hides it. Only then are the types filtered: overlay counts
// only as the root file system of a container; single-file bind mounts (a
// container's /etc/hostname, /etc/hosts …) are skipped.
func (f FS) Mounts() ([]Mount, error) {
	lines, err := f.lines("proc/mounts")
	if err != nil {
		return nil, err
	}
	var order []string // mount points by their first appearance
	visible := map[string]Mount{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		m := Mount{Device: unescape(fields[0]), Path: unescape(fields[1]), Type: fields[2]}
		if _, ok := visible[m.Path]; !ok {
			order = append(order, m.Path)
		}
		visible[m.Path] = m
	}
	var out []Mount
	for _, p := range order {
		m := visible[p]
		if !realFS[m.Type] && !(m.Type == "overlay" && m.Path == "/") {
			continue
		}
		if fi, err := os.Stat(f.path(m.Path)); err == nil && !fi.IsDir() {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// unescape decodes the octal escapes (\040 etc.) used in /proc/mounts.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

type OSRelease struct{ Name, Version, PrettyName string }

func (f FS) OSRelease() (OSRelease, error) {
	lines, err := f.lines("etc/os-release")
	if err != nil {
		return OSRelease{}, err
	}
	values := map[string]string{}
	for _, line := range lines {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		if unq, err := strconv.Unquote(value); err == nil {
			value = unq
		} else {
			value = strings.Trim(value, `'"`)
		}
		values[key] = value
	}
	r := OSRelease{Name: values["NAME"], Version: values["VERSION_ID"], PrettyName: values["PRETTY_NAME"]}
	if data, err := os.ReadFile(f.path("etc/alpine-release")); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			r.Version = v
		}
	}
	if r.PrettyName == "" {
		r.PrettyName = strings.TrimSpace(r.Name + " " + r.Version)
	}
	return r, nil
}

type CPUInfo struct {
	Model string
	Cores int
}

func (f FS) CPUInfo() (CPUInfo, error) {
	lines, err := f.lines("proc/cpuinfo")
	if err != nil {
		return CPUInfo{Cores: runtime.NumCPU()}, err
	}
	var info CPUInfo
	models := map[string]string{}
	for _, line := range lines {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "processor" {
			info.Cores++
		}
		if _, seen := models[key]; !seen {
			models[key] = value
		}
	}
	// x86: "model name"; arm: "Model" (board) or "Hardware"
	for _, k := range []string{"model name", "Model", "Hardware", "cpu model"} {
		if v := models[k]; v != "" {
			info.Model = v
			break
		}
	}
	if info.Model == "" {
		info.Model = f.boardModel()
	}
	if info.Model == "" && models["CPU implementer"] != "" && models["CPU part"] != "" {
		// generic aarch64 kernels name neither the CPU nor the board
		info.Model = fmt.Sprintf("ARM (implementer %s, part %s)", models["CPU implementer"], models["CPU part"])
	}
	if info.Cores == 0 {
		info.Cores = runtime.NumCPU()
	}
	return info, nil
}

// boardModel is the model from the device tree (arm boards); the file ends with a NUL byte.
func (f FS) boardModel() string {
	data, err := os.ReadFile(f.path("proc/device-tree/model"))
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\x00 \t\r\n")
}
