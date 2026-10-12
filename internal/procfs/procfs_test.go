package procfs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var fixture = FS{Root: "testdata"}

func TestCPUTimesAndUsage(t *testing.T) {
	got, err := fixture.CPUTimes()
	if err != nil {
		t.Fatal(err)
	}
	if got != (CPUTimes{Total: 9390, Idle: 3722}) {
		t.Fatalf("CPUTimes = %+v", got)
	}
	if u := CPUUsage(CPUTimes{Total: 1000, Idle: 800}, CPUTimes{Total: 1100, Idle: 825}); u != 75 {
		t.Fatalf("usage = %v", u)
	}
	if u := CPUUsage(got, got); u != 0 {
		t.Fatalf("usage without progress = %v", u)
	}
}

func TestMemInfo(t *testing.T) {
	m, err := fixture.MemInfo()
	if err != nil {
		t.Fatal(err)
	}
	want := MemInfo{Total: 2048000 * 1024, Available: 1024000 * 1024, SwapTotal: 1048576 * 1024, SwapFree: 786432 * 1024}
	if m != want {
		t.Fatalf("MemInfo = %+v", m)
	}
}

func TestLoadAvgAndUptime(t *testing.T) {
	load, err := fixture.LoadAvg()
	if err != nil || load != [3]float64{0.52, 0.34, 0.20} {
		t.Fatalf("LoadAvg = %v, %v", load, err)
	}
	up, err := fixture.Uptime()
	if err != nil || up != 3725*time.Second+500*time.Millisecond {
		t.Fatalf("Uptime = %v, %v", up, err)
	}
}

func TestNetDev(t *testing.T) {
	n, err := fixture.NetDev()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(n, map[string]NetCounters{"eth0": {RxBytes: 5000000, TxBytes: 2500000}}) {
		t.Fatalf("NetDev = %+v", n)
	}
}

// rootWith creates a root directory holding the given files (slash-separated paths below the root).
func rootWith(t *testing.T, files map[string]string) FS {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return FS{Root: root}
}

func TestMounts(t *testing.T) {
	// The fixture also mounts tmpfs over the ext4 file system /dev/sdc1 at /mnt/scratch: what is visible there is
	// the tmpfs, so the mount point is not listed.
	m, err := fixture.Mounts()
	if err != nil {
		t.Fatal(err)
	}
	want := []Mount{
		{Device: "/dev/sda3", Path: "/", Type: "ext4"},
		{Device: "/dev/sdb1", Path: "/boot", Type: "vfat"},
		{Device: "/dev/md0", Path: "/srv/my data", Type: "xfs"},
	}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("Mounts = %+v", m)
	}

	// a bind-mounted file is not a file system
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "etc/hostname"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "proc/mounts"), []byte("/dev/sda3 / ext4 rw 0 0\n/dev/sda3 /etc/hostname ext4 rw 0 0\n"), 0o644)
	m, err = FS{Root: root}.Mounts()
	if err != nil || len(m) != 1 || m[0].Path != "/" {
		t.Fatalf("Mounts with file bind mount = %+v, %v", m, err)
	}
}

// The last mount per mount point wins before the file system type is looked at: a tmpfs (or any other file system
// that is not listed) over a real one hides it, a real one over a tmpfs shows up.
func TestMountsLastWinsBeforeTheTypeFilter(t *testing.T) {
	cases := map[string]struct {
		mounts string
		want   []Mount
	}{
		"tmpfs over ext4 hides it": {
			"/dev/sda3 / ext4 rw 0 0\n/dev/sdb1 /data ext4 rw 0 0\ntmpfs /data tmpfs rw 0 0\n",
			[]Mount{{Device: "/dev/sda3", Path: "/", Type: "ext4"}},
		},
		"ext4 over tmpfs shows": {
			"/dev/sda3 / ext4 rw 0 0\ntmpfs /data tmpfs rw 0 0\n/dev/sdb1 /data ext4 rw 0 0\n",
			[]Mount{{Device: "/dev/sda3", Path: "/", Type: "ext4"}, {Device: "/dev/sdb1", Path: "/data", Type: "ext4"}},
		},
		"ext4 again over the tmpfs over ext4 shows the newest": {
			"/dev/sda3 / ext4 rw 0 0\n/dev/sdb1 /data ext4 rw 0 0\ntmpfs /data tmpfs rw 0 0\n/dev/sdc1 /data ext4 rw 0 0\n",
			[]Mount{{Device: "/dev/sda3", Path: "/", Type: "ext4"}, {Device: "/dev/sdc1", Path: "/data", Type: "ext4"}},
		},
		"proc over a real mount point hides it": {
			"/dev/sda3 / ext4 rw 0 0\n/dev/sdb1 /mnt ext4 rw 0 0\nproc /mnt proc rw 0 0\n",
			[]Mount{{Device: "/dev/sda3", Path: "/", Type: "ext4"}},
		},
		"overlay below the root is not a file system": {
			"/dev/sda3 / ext4 rw 0 0\n/dev/sdb1 /mnt ext4 rw 0 0\noverlay /mnt overlay rw 0 0\n",
			[]Mount{{Device: "/dev/sda3", Path: "/", Type: "ext4"}},
		},
		"overlay at the root is the container's file system": {
			"overlay / overlay rw 0 0\ntmpfs /run tmpfs rw 0 0\n",
			[]Mount{{Device: "overlay", Path: "/", Type: "overlay"}},
		},
		"the first position is kept": {
			"/dev/sda3 / ext4 rw 0 0\n/dev/sdb1 /a ext4 rw 0 0\n/dev/sdc1 /b ext4 rw 0 0\n/dev/sdd1 /a ext4 rw 0 0\n",
			[]Mount{
				{Device: "/dev/sda3", Path: "/", Type: "ext4"},
				{Device: "/dev/sdd1", Path: "/a", Type: "ext4"},
				{Device: "/dev/sdc1", Path: "/b", Type: "ext4"},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := rootWith(t, map[string]string{"proc/mounts": tc.mounts}).Mounts()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Mounts = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCPUInfoWithoutAModelName(t *testing.T) {
	const generic = "processor\t: 0\nCPU implementer\t: 0x41\nCPU architecture: 8\nCPU part\t: 0xd08\n\n" +
		"processor\t: 1\nCPU implementer\t: 0x41\nCPU architecture: 8\nCPU part\t: 0xd08\n"
	cases := map[string]struct {
		files map[string]string
		want  CPUInfo
	}{
		"generic aarch64 fixture: implementer and part": {
			nil, // FS{Root: "testdata/aarch64"}
			CPUInfo{Model: "ARM (implementer 0x41, part 0xd08)", Cores: 2},
		},
		"device tree model, NUL terminated": {
			map[string]string{"proc/cpuinfo": generic, "proc/device-tree/model": "Raspberry Pi 4 Model B Rev 1.4\x00"},
			CPUInfo{Model: "Raspberry Pi 4 Model B Rev 1.4", Cores: 2},
		},
		"device tree model, NUL and newline": {
			map[string]string{"proc/cpuinfo": generic, "proc/device-tree/model": "Radxa ROCK 5B \x00\n"},
			CPUInfo{Model: "Radxa ROCK 5B", Cores: 2},
		},
		"empty device tree model falls through to implementer and part": {
			map[string]string{"proc/cpuinfo": generic, "proc/device-tree/model": "\x00"},
			CPUInfo{Model: "ARM (implementer 0x41, part 0xd08)", Cores: 2},
		},
		"the Model line of cpuinfo wins over the device tree": {
			map[string]string{
				"proc/cpuinfo":           "processor\t: 0\nModel\t\t: Raspberry Pi 3 Model B Plus Rev 1.3\n",
				"proc/device-tree/model": "something else\x00",
			},
			CPUInfo{Model: "Raspberry Pi 3 Model B Plus Rev 1.3", Cores: 1},
		},
		"no hint at all": {
			map[string]string{"proc/cpuinfo": "processor\t: 0\nCPU implementer\t: 0x41\n"},
			CPUInfo{Cores: 1},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fs := FS{Root: "testdata/aarch64"}
			if tc.files != nil {
				fs = rootWith(t, tc.files)
			}
			got, err := fs.CPUInfo()
			if err != nil || got != tc.want {
				t.Fatalf("CPUInfo = %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
}

func TestOSReleaseAndCPUInfo(t *testing.T) {
	r, err := fixture.OSRelease()
	if err != nil {
		t.Fatal(err)
	}
	if r != (OSRelease{Name: "Alpine Linux", Version: "3.24.2", PrettyName: "Alpine Linux v3.24"}) {
		t.Fatalf("OSRelease = %+v", r)
	}
	c, err := fixture.CPUInfo()
	if err != nil || c != (CPUInfo{Model: "Intel(R) Core(TM) i5-8250U CPU @ 1.60GHz", Cores: 2}) {
		t.Fatalf("CPUInfo = %+v, %v", c, err)
	}
}

func TestMissingFiles(t *testing.T) {
	empty := FS{Root: t.TempDir()}
	if _, err := empty.CPUTimes(); err == nil {
		t.Fatal("expected error")
	}
	if _, err := empty.MemInfo(); err == nil {
		t.Fatal("expected error")
	}
}
