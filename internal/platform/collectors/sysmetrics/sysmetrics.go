package sysmetrics

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/beevik/ntp"
	"github.com/distatus/battery"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/version"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type collector struct {
	queueStats   func() (int, int64)
	runtimeStats func() AgentRuntimeStats
	prefs        func() config.CollectPrefs
}

type AgentRuntimeStats struct {
	FlushOK        int64
	FlushErr       int64
	LastFlushMs    int64
	LastFlushBatch int64
}

func New(queueStats func() (int, int64), prefsProvider func() config.CollectPrefs, runtimeStatsProvider ...func() AgentRuntimeStats) ports.Collector {
	var runtimeStats func() AgentRuntimeStats
	if len(runtimeStatsProvider) > 0 {
		runtimeStats = runtimeStatsProvider[0]
	}
	return &collector{queueStats: queueStats, runtimeStats: runtimeStats, prefs: prefsProvider}
}

func (c *collector) Name() string { return "sysmetrics" }

func (c *collector) Interval() time.Duration { return 10 * time.Second }

type snapshot struct {
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	CPU          *cpuSnapshot    `json:"cpu,omitempty"`
	Memory       *memSnapshot    `json:"memory,omitempty"`
	Disk         *diskSnapshot   `json:"disk,omitempty"`
	Network      *netSnapshot    `json:"network,omitempty"`
	Host         *hostSnapshot   `json:"host,omitempty"`
	Sensors      *sensorsSnap    `json:"sensors,omitempty"`
	NetActive    *netActive      `json:"net_active,omitempty"`
	Power        *powerSnapshot  `json:"power,omitempty"`
	Sanity       *sanitySnapshot `json:"sanity,omitempty"`
	GPU          []gpuSnapshot   `json:"gpu,omitempty"`
	Services     []serviceSnap   `json:"services,omitempty"`
	TimeSync     *timeSyncSnap   `json:"time_sync,omitempty"`
	Vulns        *vulnsSnap      `json:"vulns,omitempty"`
	Inventory    *inventorySnap  `json:"inventory,omitempty"`
	Logs         []logFileSnap   `json:"logs,omitempty"`
	Updates      []updatesSnap   `json:"updates,omitempty"`
	Agent        *agentSnap      `json:"agent,omitempty"`
	Processes    []procSnapshot  `json:"processes,omitempty"`
}

type cpuSnapshot struct {
	PercentTotal   float64   `json:"percent_total"`
	PercentPerCPU  []float64 `json:"percent_per_cpu,omitempty"`
	Load1          float64   `json:"load1,omitempty"`
	Load5          float64   `json:"load5,omitempty"`
	Load15         float64   `json:"load15,omitempty"`
	CoresLogical   int       `json:"cores_logical,omitempty"`
	CoresPhysical  int       `json:"cores_physical,omitempty"`
	FreqCurrentMHz float64   `json:"freq_current_mhz,omitempty"`
	FreqMaxMHz     float64   `json:"freq_max_mhz,omitempty"`
}

type memSnapshot struct {
	Total        uint64  `json:"total_bytes"`
	Used         uint64  `json:"used_bytes"`
	Free         uint64  `json:"free_bytes"`
	UsedPercent  float64 `json:"used_percent"`
	Buffers      uint64  `json:"buffers_bytes,omitempty"`
	Cached       uint64  `json:"cached_bytes,omitempty"`
	SwapTotal    uint64  `json:"swap_total_bytes"`
	SwapUsed     uint64  `json:"swap_used_bytes"`
	SwapFree     uint64  `json:"swap_free_bytes"`
	SwapUsedPerc float64 `json:"swap_used_percent"`
}

type diskSnapshot struct {
	Filesystems []diskFS     `json:"filesystems,omitempty"`
	IOStats     []diskIO     `json:"io_stats,omitempty"`
	SMART       []smartState `json:"smart,omitempty"`
}

type diskFS struct {
	Mount          string  `json:"mount"`
	FSType         string  `json:"fs_type"`
	Total          uint64  `json:"total_bytes"`
	Used           uint64  `json:"used_bytes"`
	Free           uint64  `json:"free_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	InodesTotal    uint64  `json:"inodes_total,omitempty"`
	InodesUsed     uint64  `json:"inodes_used,omitempty"`
	InodesFree     uint64  `json:"inodes_free,omitempty"`
	InodesUsedPerc float64 `json:"inodes_used_percent,omitempty"`
}

type diskIO struct {
	Device      string `json:"device"`
	Reads       uint64 `json:"reads"`
	Writes      uint64 `json:"writes"`
	ReadBytes   uint64 `json:"read_bytes"`
	WriteBytes  uint64 `json:"write_bytes"`
	ReadTimeMs  uint64 `json:"read_time_ms"`
	WriteTimeMs uint64 `json:"write_time_ms"`
}

type smartState struct {
	Device       string  `json:"device"`
	Health       string  `json:"health,omitempty"`
	TemperatureC float64 `json:"temperature_c,omitempty"`
}

type netSnapshot struct {
	Interfaces []netIf `json:"interfaces,omitempty"`
}

type netIf struct {
	Name        string   `json:"name"`
	MTU         int      `json:"mtu,omitempty"`
	MAC         string   `json:"mac,omitempty"`
	IPs         []string `json:"ips,omitempty"`
	Flags       []string `json:"flags,omitempty"`
	BytesSent   uint64   `json:"bytes_sent"`
	BytesRecv   uint64   `json:"bytes_recv"`
	PacketsSent uint64   `json:"packets_sent"`
	PacketsRecv uint64   `json:"packets_recv"`
	ErrIn       uint64   `json:"err_in"`
	ErrOut      uint64   `json:"err_out"`
	DropIn      uint64   `json:"drop_in"`
	DropOut     uint64   `json:"drop_out"`
	IsUp        bool     `json:"is_up"`
}

type hostSnapshot struct {
	Hostname         string `json:"hostname,omitempty"`
	OS               string `json:"os,omitempty"`
	Platform         string `json:"platform,omitempty"`
	PlatformFamily   string `json:"platform_family,omitempty"`
	PlatformVersion  string `json:"platform_version,omitempty"`
	KernelVersion    string `json:"kernel_version,omitempty"`
	UptimeSec        uint64 `json:"uptime_sec,omitempty"`
	BootTimeUnix     uint64 `json:"boot_time_unix,omitempty"`
	Virtualization   string `json:"virtualization,omitempty"`
	VirtualizationRo string `json:"virtualization_role,omitempty"`
}

type sensorsSnap struct {
	Temperatures []tempReading `json:"temperatures,omitempty"`
	Fans         []fanReading  `json:"fans,omitempty"`
}

type tempReading struct {
	Sensor string  `json:"sensor"`
	TempC  float64 `json:"temp_c"`
}

type fanReading struct {
	Sensor string `json:"sensor"`
	RPM    int64  `json:"rpm"`
}

type netActive struct {
	ConnectionsByState map[string]int `json:"connections_by_state,omitempty"`
	Listening          []listenPort   `json:"listening,omitempty"`
}

type listenPort struct {
	Proto     string `json:"proto"`
	LocalAddr string `json:"local_addr"`
	LocalPort uint32 `json:"local_port"`
}

type powerSnapshot struct {
	Batteries []batterySnapshot `json:"batteries,omitempty"`
}

type batterySnapshot struct {
	Percent        float64 `json:"percent"`
	State          string  `json:"state"`
	DesignCapacity float64 `json:"design_capacity_wh,omitempty"`
	FullCapacity   float64 `json:"full_capacity_wh,omitempty"`
	ChargeRateMw   float64 `json:"charge_rate_mw,omitempty"`
	Voltage        float64 `json:"voltage_v,omitempty"`
}

type sanitySnapshot struct {
	Ping []sanityCheck `json:"ping,omitempty"`
	DNS  []sanityCheck `json:"dns,omitempty"`
}

type sanityCheck struct {
	Target     string `json:"target"`
	Success    bool   `json:"success"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

var (
	defaultPingTargets = []string{"1.1.1.1:53", "8.8.8.8:53"}
	defaultDNSTargets  = []string{"example.com", "google.com"}
	cveSigCache        []cveSignature
	cveSigLastURL      string
	cveSigLastFetch    time.Time
	cveSigMu           sync.Mutex
)

// Compatibilidade defensiva para ambientes com coluna SQL estreita (ex.: MEDIUMINT).
// Evita perder o snapshot inteiro quando o offset NTP extrapola o range suportado no backend.
const snapshotTimeOffsetMaxAbsMs int64 = 8_388_607

type gpuSnapshot struct {
	Vendor       string  `json:"vendor"`
	Name         string  `json:"name,omitempty"`
	MemoryTotal  float64 `json:"memory_total_mb,omitempty"`
	MemoryUsed   float64 `json:"memory_used_mb,omitempty"`
	MemoryFree   float64 `json:"memory_free_mb,omitempty"`
	UtilPercent  float64 `json:"util_percent,omitempty"`
	TemperatureC float64 `json:"temperature_c,omitempty"`
	FanPercent   float64 `json:"fan_percent,omitempty"`
	PowerW       float64 `json:"power_w,omitempty"`
}

type serviceSnap struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type agentSnap struct {
	QueueItems     int    `json:"queue_items,omitempty"`
	QueueBytes     int64  `json:"queue_bytes,omitempty"`
	FlushOKTotal   int64  `json:"flush_ok_total"`
	FlushErrTotal  int64  `json:"flush_err_total"`
	LastFlushMs    int64  `json:"last_flush_ms"`
	LastFlushBatch int64  `json:"last_flush_batch"`
	Version        string `json:"version,omitempty"`
}

type timeSyncSnap struct {
	Source    string  `json:"source"`
	OffsetMs  int64   `json:"offset_ms,omitempty"`
	RTTMs     int64   `json:"rtt_ms,omitempty"`
	Error     string  `json:"error,omitempty"`
	LastCheck float64 `json:"last_check_unix,omitempty"`
}

type logFileSnap struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type updatesSnap struct {
	Source        string         `json:"source"`
	Pending       int            `json:"pending,omitempty"`
	Error         string         `json:"error,omitempty"`
	LastCheckUnix int64          `json:"last_check_unix,omitempty"`
	Security      securityUpdate `json:"security,omitempty"`
}

type vulnsSnap struct {
	CVEs []string `json:"cves"`
}

type inventorySnap struct {
	LinuxRPMPackages []rpmPkg     `json:"linux_rpm_packages,omitempty"`
	OSRelease        osRelease    `json:"os_release,omitempty"`
	Kernel           kernelInfo   `json:"kernel,omitempty"`
	Repos            repoSnap     `json:"repos,omitempty"`
	WinHotfixes      []winHotfix  `json:"windows_hotfixes,omitempty"`
	WinApps          []winApp     `json:"windows_apps,omitempty"`
	WinFeatures      []winFeature `json:"windows_features,omitempty"`
}

type rpmPkg struct {
	Name    string `json:"name"`
	Epoch   int    `json:"epoch"`
	Version string `json:"version"`
	Release string `json:"release"`
	Arch    string `json:"arch,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
	Source  string `json:"source,omitempty"`
}

type osRelease struct {
	ID         string `json:"id,omitempty"`
	VersionID  string `json:"version_id,omitempty"`
	PrettyName string `json:"pretty_name,omitempty"`
}

type kernelInfo struct {
	Running   string   `json:"running,omitempty"`
	Installed []rpmPkg `json:"installed,omitempty"`
}

type repoSnap struct {
	Enabled []string `json:"enabled,omitempty"`
	Raw     string   `json:"raw,omitempty"`
}

type winHotfix struct {
	ID          string `json:"id,omitempty"`
	InstalledOn string `json:"installed_on,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
}

type winApp struct {
	Name            string `json:"name,omitempty"`
	Version         string `json:"version,omitempty"`
	Vendor          string `json:"vendor,omitempty"`
	Install         string `json:"install_date,omitempty"`
	Source          string `json:"source,omitempty"`
	InstallLocation string `json:"install_location,omitempty"`
	InstallSource   string `json:"install_source,omitempty"`
	UninstallString string `json:"uninstall_string,omitempty"`
}

type winFeature struct {
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Installed   bool   `json:"installed"`
}

type pendingUpdate struct {
	UpdateID     string   `json:"update_id,omitempty"`
	Title        string   `json:"title,omitempty"`
	KBIDs        []string `json:"kb_ids,omitempty"`
	Category     string   `json:"category,omitempty"`
	Severity     string   `json:"severity,omitempty"`
	IsDownloaded *bool    `json:"is_downloaded,omitempty"`
	IsInstalled  *bool    `json:"is_installed,omitempty"`
}

type securityUpdate struct {
	Advisories   []securityAdvisory `json:"advisories,omitempty"`
	Pending      []pendingUpdate    `json:"pending_updates,omitempty"`
	PendingCount int                `json:"pending_count,omitempty"`
}

type securityAdvisory struct {
	AdvisoryID string   `json:"advisory_id,omitempty"`
	Severity   string   `json:"severity,omitempty"`
	CVEs       []string `json:"cves,omitempty"`
	Packages   []string `json:"packages,omitempty"`
}

type cveSignature struct {
	OS      string   `json:"os,omitempty"`
	Pkg     string   `json:"pkg"`
	Op      string   `json:"op"`
	Version string   `json:"version"`
	CVEs    []string `json:"cves"`
}

type procSnapshot struct {
	PID            int32   `json:"pid"`
	Name           string  `json:"name,omitempty"`
	CPUPercent     float64 `json:"cpu_percent,omitempty"`
	RSSBytes       uint64  `json:"rss_bytes,omitempty"`
	VMSBytes       uint64  `json:"vms_bytes,omitempty"`
	IOReadBytes    uint64  `json:"io_read_bytes,omitempty"`
	IOWriteBytes   uint64  `json:"io_write_bytes,omitempty"`
	CreateTimeUnix int64   `json:"create_time_unix,omitempty"`
	Status         string  `json:"status,omitempty"`
	Cmdline        string  `json:"cmdline,omitempty"`
}

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	p := prefs.Default()
	if c.prefs != nil {
		p = c.prefs()
	}
	if p.Paused {
		return nil, nil
	}

	s := snapshot{Capabilities: make(map[string]bool)}
	agentInfo := agentSnap{Version: version.Version}

	if p.CPU {
		cpuOk := false
		cpuSnap := &cpuSnapshot{}
		if totals, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(totals) > 0 {
			cpuSnap.PercentTotal = totals[0]
			cpuOk = true
		}
		if perCPU, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
			cpuSnap.PercentPerCPU = perCPU
			cpuOk = true
		}
		if l, err := load.AvgWithContext(ctx); err == nil {
			cpuSnap.Load1, cpuSnap.Load5, cpuSnap.Load15 = l.Load1, l.Load5, l.Load15
			cpuOk = true
		}
		if n, err := cpu.CountsWithContext(ctx, true); err == nil {
			cpuSnap.CoresLogical = n
			cpuOk = true
		}
		if n, err := cpu.CountsWithContext(ctx, false); err == nil {
			cpuSnap.CoresPhysical = n
			cpuOk = true
		}
		if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
			cpuSnap.FreqCurrentMHz = infos[0].Mhz
			cpuSnap.FreqMaxMHz = infos[0].Mhz
			cpuOk = true
		}
		if cpuOk {
			s.CPU = cpuSnap
		}
		s.Capabilities["cpu"] = cpuOk
	} else {
		s.Capabilities["cpu"] = false
	}

	if p.Memory {
		memOk := false
		memSnap := &memSnapshot{}
		if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
			memSnap.Total = vm.Total
			memSnap.Used = vm.Used
			memSnap.Free = vm.Free
			memSnap.UsedPercent = vm.UsedPercent
			memSnap.Buffers = vm.Buffers
			memSnap.Cached = vm.Cached
			memOk = true
		}
		if swap, err := mem.SwapMemoryWithContext(ctx); err == nil {
			memSnap.SwapTotal = swap.Total
			memSnap.SwapUsed = swap.Used
			memSnap.SwapFree = swap.Free
			memSnap.SwapUsedPerc = swap.UsedPercent
			memOk = true
		}
		if memOk {
			s.Memory = memSnap
		}
		s.Capabilities["memory"] = memOk
	} else {
		s.Capabilities["memory"] = false
	}

	if p.Disk {
		diskOk := false
		diskSnap := &diskSnapshot{}
		if parts, err := disk.PartitionsWithContext(ctx, true); err == nil {
			devSeen := make(map[string]struct{})
			for _, p := range parts {
				if u, err := disk.UsageWithContext(ctx, p.Mountpoint); err == nil {
					diskSnap.Filesystems = append(diskSnap.Filesystems, diskFS{
						Mount:          p.Mountpoint,
						FSType:         p.Fstype,
						Total:          u.Total,
						Used:           u.Used,
						Free:           u.Free,
						UsedPercent:    u.UsedPercent,
						InodesTotal:    u.InodesTotal,
						InodesUsed:     u.InodesUsed,
						InodesFree:     u.InodesFree,
						InodesUsedPerc: u.InodesUsedPercent,
					})
					diskOk = true
				}
				if p.Device != "" {
					devSeen[p.Device] = struct{}{}
				}
			}
			if sm := collectSMART(devSeen); len(sm) > 0 {
				diskSnap.SMART = sm
				diskOk = true
			}
		}
		if ioStats, err := disk.IOCountersWithContext(ctx); err == nil {
			for name, io := range ioStats {
				diskSnap.IOStats = append(diskSnap.IOStats, diskIO{
					Device:      name,
					Reads:       io.ReadCount,
					Writes:      io.WriteCount,
					ReadBytes:   io.ReadBytes,
					WriteBytes:  io.WriteBytes,
					ReadTimeMs:  io.ReadTime,
					WriteTimeMs: io.WriteTime,
				})
			}
			if len(diskSnap.IOStats) > 0 {
				sort.Slice(diskSnap.IOStats, func(i, j int) bool { return diskSnap.IOStats[i].Device < diskSnap.IOStats[j].Device })
				diskOk = true
			}
		}
		if diskOk {
			s.Disk = diskSnap
		}
		s.Capabilities["disk"] = diskOk
	} else {
		s.Capabilities["disk"] = false
	}

	if p.Network {
		netOk := false
		netSnap := &netSnapshot{}
		if ifs, err := gnet.InterfacesWithContext(ctx); err == nil {
			ioByName := map[string]gnet.IOCountersStat{}
			if ioStats, err := gnet.IOCountersWithContext(ctx, true); err == nil {
				for _, st := range ioStats {
					ioByName[st.Name] = st
				}
			}
			for _, inf := range ifs {
				io := ioByName[inf.Name]
				var addrs []string
				for _, a := range inf.Addrs {
					addrs = append(addrs, a.Addr)
				}
				netSnap.Interfaces = append(netSnap.Interfaces, netIf{
					Name:        inf.Name,
					MTU:         inf.MTU,
					MAC:         inf.HardwareAddr,
					IPs:         addrs,
					Flags:       inf.Flags,
					BytesSent:   io.BytesSent,
					BytesRecv:   io.BytesRecv,
					PacketsSent: io.PacketsSent,
					PacketsRecv: io.PacketsRecv,
					ErrIn:       io.Errin,
					ErrOut:      io.Errout,
					DropIn:      io.Dropin,
					DropOut:     io.Dropout,
					IsUp:        containsFlag(inf.Flags, "up") || containsFlag(inf.Flags, "UP"),
				})
			}
			if len(netSnap.Interfaces) > 0 {
				netOk = true
			}
		}
		if netOk {
			s.Network = netSnap
		}
		s.Capabilities["network"] = netOk
	} else {
		s.Capabilities["network"] = false
	}

	if p.NetActive {
		if conns, err := gnet.ConnectionsWithContext(ctx, "inet"); err == nil {
			stateCount := make(map[string]int)
			var listening []listenPort
			for _, c := range conns {
				stateCount[c.Status]++
				if isListeningPort(c) {
					listening = append(listening, listenPort{
						Proto:     protoName(c.Type),
						LocalAddr: c.Laddr.IP,
						LocalPort: c.Laddr.Port,
					})
				}
			}
			if len(stateCount) > 0 || len(listening) > 0 {
				s.NetActive = &netActive{
					ConnectionsByState: stateCount,
					Listening:          listening,
				}
			}
			s.Capabilities["net_active"] = true
		} else {
			s.Capabilities["net_active"] = false
		}
	} else {
		s.Capabilities["net_active"] = false
	}

	if p.Host {
		if hi, err := host.InfoWithContext(ctx); err == nil {
			s.Host = &hostSnapshot{
				Hostname:         hi.Hostname,
				OS:               hi.OS,
				Platform:         hi.Platform,
				PlatformFamily:   hi.PlatformFamily,
				PlatformVersion:  hi.PlatformVersion,
				KernelVersion:    hi.KernelVersion,
				UptimeSec:        hi.Uptime,
				BootTimeUnix:     hi.BootTime,
				Virtualization:   hi.VirtualizationSystem,
				VirtualizationRo: hi.VirtualizationRole,
			}
			s.Capabilities["host"] = true
		} else {
			s.Capabilities["host"] = false
		}
	} else {
		s.Capabilities["host"] = false
	}

	if p.Sensors {
		sensors := &sensorsSnap{}
		if temps, err := host.SensorsTemperaturesWithContext(ctx); err == nil {
			for _, t := range temps {
				sensors.Temperatures = append(sensors.Temperatures, tempReading{
					Sensor: t.SensorKey,
					TempC:  t.Temperature,
				})
			}
		}
		if fans := readFanSpeeds(); len(fans) > 0 {
			sensors.Fans = fans
		}
		sensorsOk := len(sensors.Temperatures) > 0 || len(sensors.Fans) > 0
		if sensorsOk {
			s.Sensors = sensors
		}
		s.Capabilities["sensors"] = sensorsOk
	} else {
		s.Capabilities["sensors"] = false
	}

	if p.Power {
		power := &powerSnapshot{}
		if bats, err := battery.GetAll(); err == nil {
			for _, b := range bats {
				power.Batteries = append(power.Batteries, batterySnapshot{
					Percent:        b.Current / b.Full * 100,
					State:          b.State.String(),
					DesignCapacity: b.Design,
					FullCapacity:   b.Full,
					ChargeRateMw:   b.ChargeRate,
					Voltage:        b.Voltage,
				})
			}
		}
		powerOk := len(power.Batteries) > 0
		if powerOk {
			s.Power = power
		}
		s.Capabilities["power"] = powerOk
	} else {
		s.Capabilities["power"] = false
	}

	if p.Sanity {
		sanity := &sanitySnapshot{
			Ping: multiPing(pingTargets(), 2*time.Second),
			DNS:  multiDNS(dnsTargets(), 2*time.Second),
		}
		sanityOk := len(sanity.Ping) > 0 || len(sanity.DNS) > 0
		if sanityOk {
			s.Sanity = sanity
		}
		s.Capabilities["sanity"] = sanityOk
	} else {
		s.Capabilities["sanity"] = false
	}

	if p.GPU {
		if gpus := collectGPUs(); len(gpus) > 0 {
			s.GPU = gpus
			s.Capabilities["gpu"] = true
		} else {
			s.Capabilities["gpu"] = false
		}
	} else {
		s.Capabilities["gpu"] = false
	}

	if p.Services {
		if services := collectServices(); len(services) > 0 {
			s.Services = services
			s.Capabilities["services"] = true
		} else {
			s.Capabilities["services"] = false
		}
	} else {
		s.Capabilities["services"] = false
	}

	if p.TimeSync {
		ts := timeSyncCheck("time.google.com", 3*time.Second)
		if ts.Source != "" {
			s.TimeSync = &ts
			s.Capabilities["time_sync"] = true
		} else {
			s.Capabilities["time_sync"] = false
		}
	} else {
		s.Capabilities["time_sync"] = false
	}

	if p.Logs {
		if logs := collectLogs(); len(logs) > 0 {
			s.Logs = logs
			s.Capabilities["logs"] = true
		} else {
			s.Capabilities["logs"] = false
		}
	} else {
		s.Capabilities["logs"] = false
	}

	if p.Updates {
		if updates := collectUpdates(3 * time.Second); len(updates) > 0 {
			s.Updates = updates
			s.Capabilities["updates"] = true
		} else {
			s.Capabilities["updates"] = false
		}
	} else {
		s.Capabilities["updates"] = false
	}

	if p.Agent {
		if c.queueStats != nil {
			items, bytes := c.queueStats()
			agentInfo.QueueItems = items
			agentInfo.QueueBytes = bytes
			s.Capabilities["agent"] = true
		} else {
			s.Capabilities["agent"] = false
		}
		if c.runtimeStats != nil {
			runtime := c.runtimeStats()
			agentInfo.FlushOKTotal = runtime.FlushOK
			agentInfo.FlushErrTotal = runtime.FlushErr
			agentInfo.LastFlushMs = runtime.LastFlushMs
			agentInfo.LastFlushBatch = runtime.LastFlushBatch
			s.Capabilities["agent"] = true
		}
		s.Agent = &agentInfo
	} else {
		s.Capabilities["agent"] = false
	}

	if p.Vulns {
		cves := detectCVEs(ctx, p)
		if len(cves) > 0 {
			s.Vulns = &vulnsSnap{CVEs: cves}
		}
		s.Capabilities["vulns"] = true
	} else {
		s.Capabilities["vulns"] = false
	}
	if p.Inventory {
		inv := collectInventory(ctx)
		hasInventory := len(inv.LinuxRPMPackages) > 0 ||
			len(inv.WinApps) > 0 ||
			len(inv.WinHotfixes) > 0 ||
			len(inv.WinFeatures) > 0 ||
			inv.OSRelease.ID != "" ||
			inv.OSRelease.VersionID != "" ||
			inv.OSRelease.PrettyName != "" ||
			inv.Kernel.Running != "" ||
			len(inv.Kernel.Installed) > 0 ||
			len(inv.Repos.Enabled) > 0 ||
			inv.Repos.Raw != ""
		if hasInventory {
			s.Inventory = &inv
		}
		s.Capabilities["inventory"] = hasInventory
	} else {
		s.Capabilities["inventory"] = false
	}
	if p.Processes {
		s.Processes = topProcesses(ctx, 10, 10)
		s.Capabilities["processes"] = len(s.Processes) > 0
	} else {
		s.Capabilities["processes"] = false
	}

	return json.Marshal(s)
}

func containsFlag(flags []string, target string) bool {
	for _, f := range flags {
		if f == target {
			return true
		}
	}
	return false
}

func pingCheck(target string, timeout time.Duration) sanityCheck {
	start := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.Dial("tcp", target)
	elapsed := time.Since(start)
	if err == nil && conn != nil {
		_ = conn.Close()
	}
	return sanityCheck{
		Target:     target,
		Success:    err == nil,
		DurationMs: elapsed.Milliseconds(),
		Error:      errString(err),
	}
}

func multiPing(targets []string, timeout time.Duration) []sanityCheck {
	var out []sanityCheck
	for _, t := range targets {
		out = append(out, pingCheck(t, timeout))
	}
	return out
}

func dnsCheck(hostname string, timeout time.Duration) sanityCheck {
	resolver := net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	_, err := resolver.LookupHost(ctx, hostname)
	elapsed := time.Since(start)
	return sanityCheck{
		Target:     hostname,
		Success:    err == nil,
		DurationMs: elapsed.Milliseconds(),
		Error:      errString(err),
	}
}

func multiDNS(targets []string, timeout time.Duration) []sanityCheck {
	var out []sanityCheck
	for _, t := range targets {
		out = append(out, dnsCheck(t, timeout))
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func timeSyncCheck(hostname string, timeout time.Duration) timeSyncSnap {
	resp, err := ntp.QueryWithOptions(hostname, ntp.QueryOptions{Timeout: timeout})
	ts := timeSyncSnap{Source: hostname, LastCheck: float64(time.Now().Unix())}
	if err != nil {
		ts.Error = err.Error()
		return ts
	}
	offsetMs := int64(resp.ClockOffset / time.Millisecond)
	ts.OffsetMs = clampAbsInt64(offsetMs, snapshotTimeOffsetMaxAbsMs)
	if ts.OffsetMs != offsetMs {
		ts.Error = "offset_ms_clamped"
	}
	ts.RTTMs = resp.RTT.Milliseconds()
	return ts
}

func clampAbsInt64(v, maxAbs int64) int64 {
	if maxAbs < 0 {
		maxAbs = -maxAbs
	}
	if v > maxAbs {
		return maxAbs
	}
	if v < -maxAbs {
		return -maxAbs
	}
	return v
}

// collectSMART tenta usar smartctl para retornar health/temperatura das unidades vistas.
func collectSMART(devices map[string]struct{}) []smartState {
	path, err := exec.LookPath("smartctl")
	if err != nil || len(devices) == 0 {
		return nil
	}
	var out []smartState
	for dev := range devices {
		health, temp := smartForDevice(path, dev)
		if health == "" && temp == 0 {
			continue
		}
		out = append(out, smartState{
			Device:       dev,
			Health:       health,
			TemperatureC: temp,
		})
	}
	return out
}

func smartForDevice(smartctlPath, device string) (health string, temp float64) {
	// Health
	out, err := exec.Command(smartctlPath, "-H", device).CombinedOutput()
	if err == nil {
		health = parseSmartHealth(string(out))
	}
	// Temperature via attributes (ATA/SATA/NVMe)
	outAttrs, err := exec.Command(smartctlPath, "-A", device).CombinedOutput()
	if err == nil {
		temp = parseSmartTemp(string(outAttrs))
	}
	return
}

func parseSmartHealth(out string) string {
	scanner := bufio.NewScanner(bytes.NewBufferString(out))
	for scanner.Scan() {
		ln := scanner.Text()
		if strings.Contains(ln, "overall-health") || strings.Contains(ln, "SMART Health Status") || strings.Contains(ln, "SMART overall-health") {
			if strings.Contains(strings.ToUpper(ln), "PASSED") {
				return "PASSED"
			}
			if strings.Contains(strings.ToUpper(ln), "OK") {
				return "OK"
			}
			return strings.TrimSpace(ln)
		}
	}
	return ""
}

func parseSmartTemp(out string) float64 {
	scanner := bufio.NewScanner(bytes.NewBufferString(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		// Look for known attribute names
		name := strings.ToLower(fields[0])
		if strings.Contains(name, "temperature") {
			for i := len(fields) - 1; i >= 0; i-- {
				if v, err := strconv.ParseFloat(fields[i], 64); err == nil {
					return v
				}
			}
		}
	}
	return 0
}

// collectLogs faz best effort procurando arquivos .log na pasta ./logs.
func collectLogs() []logFileSnap {
	base := "./logs"
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var logs []logFileSnap
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		fp := filepath.Join(base, e.Name())
		if info, err := os.Stat(fp); err == nil {
			logs = append(logs, logFileSnap{Path: fp, SizeBytes: info.Size()})
		}
	}
	return logs
}

// collectUpdates faz best effort para contar updates pendentes por SO.
func collectUpdates(timeout time.Duration) []updatesSnap {
	switch runtime.GOOS {
	case "linux":
		return []updatesSnap{linuxSecurityUpdates(timeout)}
	case "darwin":
		return []updatesSnap{macUpdates(timeout)}
	case "windows":
		return []updatesSnap{windowsSecurityUpdates(timeout)}
	default:
		return nil
	}
}

func aptUpdates(timeout time.Duration) updatesSnap {
	path, err := exec.LookPath("apt-get")
	if err != nil {
		return updatesSnap{Source: "apt", Error: "apt-get not found"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-s", "upgrade").Output()
	if ctx.Err() == context.DeadlineExceeded {
		return updatesSnap{Source: "apt", Error: "timeout"}
	}
	if err != nil {
		return updatesSnap{Source: "apt", Error: err.Error()}
	}
	count := 0
	scanner := bufio.NewScanner(bytes.NewBuffer(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Inst ") {
			count++
		}
	}
	return updatesSnap{Source: "apt", Pending: count}
}

func macUpdates(timeout time.Duration) updatesSnap {
	path, err := exec.LookPath("softwareupdate")
	if err != nil {
		return updatesSnap{Source: "softwareupdate", Error: "not found"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-l").Output()
	if ctx.Err() == context.DeadlineExceeded {
		return updatesSnap{Source: "softwareupdate", Error: "timeout"}
	}
	if err != nil {
		return updatesSnap{Source: "softwareupdate", Error: err.Error()}
	}
	count := strings.Count(string(out), "*")
	return updatesSnap{Source: "softwareupdate", Pending: count}
}

// linuxSecurityUpdates tenta coletar advisories/cves via dnf updateinfo --security.
func linuxSecurityUpdates(timeout time.Duration) updatesSnap {
	// se existir apt, mantém comportamento antigo
	if _, err := exec.LookPath("apt-get"); err == nil {
		return aptUpdates(timeout)
	}
	path, err := exec.LookPath("dnf")
	if err != nil {
		path, err = exec.LookPath("yum")
		if err != nil {
			return updatesSnap{Source: "dnf", Error: "dnf/yum not found"}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmdList := exec.CommandContext(ctx, path, "updateinfo", "list", "--security")
	rawList, err := cmdList.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return updatesSnap{Source: "dnf", Error: "timeout"}
	}
	if err != nil {
		return updatesSnap{Source: "dnf", Error: err.Error()}
	}

	cmdInfo := exec.CommandContext(ctx, path, "updateinfo", "info", "--security")
	rawInfo, _ := cmdInfo.CombinedOutput() // best effort

	advisories := parseDnfUpdateinfo(rawInfo)
	pendingCount := len(advisories)
	if pendingCount == 0 {
		// fallback: contar linhas do list
		sc := bufio.NewScanner(bytes.NewBuffer(rawList))
		for sc.Scan() {
			ln := strings.TrimSpace(sc.Text())
			if ln == "" || strings.HasPrefix(strings.ToLower(ln), "last metadata expiration check") {
				continue
			}
			pendingCount++
		}
	}

	return updatesSnap{
		Source:        "dnf",
		LastCheckUnix: time.Now().Unix(),
		Security: securityUpdate{
			Advisories:   advisories,
			PendingCount: pendingCount,
		},
		Pending: pendingCount,
	}
}

func parseDnfUpdateinfo(raw []byte) []securityAdvisory {
	var out []securityAdvisory
	if len(raw) == 0 {
		return out
	}
	type state int
	const (
		none state = iota
		inPkg
	)
	var current securityAdvisory
	var section state
	sc := bufio.NewScanner(bytes.NewBuffer(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "===============================================================================") {
			if current.AdvisoryID != "" {
				out = append(out, current)
			}
			current = securityAdvisory{}
			section = none
			continue
		}
		// Example sections:
		// RHSA-2025:1234 Important/Sec. ...
		if strings.HasPrefix(line, "RHSA-") || strings.HasPrefix(line, "ELSA-") || strings.HasPrefix(line, "ALSA-") || strings.HasPrefix(line, "OSA-") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				current.AdvisoryID = parts[0]
				if len(parts) > 1 {
					current.Severity = parts[1]
				}
			}
			section = inPkg
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "cves:") {
			cves := strings.Fields(strings.TrimPrefix(line, "CVE:"))
			if len(cves) == 0 {
				rest := strings.TrimSpace(strings.TrimPrefix(line, "cves:"))
				cves = strings.Fields(rest)
			}
			current.CVEs = append(current.CVEs, cves...)
			continue
		}
		// packages lines look like: kernel.x86_64 4.18.0-553.33.1.el8_10
		if section == inPkg {
			parts := strings.Fields(line)
			if len(parts) >= 2 && strings.Contains(parts[0], ".") {
				current.Packages = append(current.Packages, parts[0]+" "+parts[1])
			}
		}
	}
	if current.AdvisoryID != "" {
		out = append(out, current)
	}
	return out
}

// windowsSecurityUpdates consulta updates pendentes via PowerShell (MSFT Update Session).
func windowsSecurityUpdates(timeout time.Duration) updatesSnap {
	path, err := exec.LookPath("powershell")
	if err != nil {
		return updatesSnap{Source: "windows_update", Error: "powershell not found"}
	}
	script := `
	$session = New-Object -ComObject Microsoft.Update.Session
	$searcher = $session.CreateUpdateSearcher()
	$result = $searcher.Search("IsInstalled=0 and Type='Software' and IsHidden=0")
	$updates = @()
	foreach ($u in $result.Updates) {
	  $kbids = @()
	  foreach ($id in $u.KBArticleIDs) { $kbids += $id }
	  $updates += [PSCustomObject]@{
	    UpdateID = $u.Identity.UpdateID
	    Title = $u.Title
	    KBIDs = $kbids
	    Category = ($u.Categories | Where-Object { $_.Type -eq 'UpdateClassification' } | Select-Object -First 1 -ExpandProperty Name)
	    Severity = $u.MsrcSeverity
	    IsDownloaded = $u.IsDownloaded
	    IsInstalled = $u.IsInstalled
	  }
	}
	$updates | ConvertTo-Json
	`
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-Command", script)
	raw, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return updatesSnap{Source: "windows_update", Error: "timeout"}
	}
	if err != nil {
		return updatesSnap{Source: "windows_update", Error: err.Error()}
	}
	if len(raw) == 0 {
		return updatesSnap{Source: "windows_update", LastCheckUnix: time.Now().Unix()}
	}
	// normaliza quando retorna um único objeto
	if len(raw) > 0 && raw[0] == '{' {
		raw = []byte("[" + strings.TrimSpace(string(raw)) + "]")
	}
	var items []pendingUpdate
	if err := json.Unmarshal(raw, &items); err != nil {
		return updatesSnap{Source: "windows_update", Error: err.Error()}
	}
	return updatesSnap{
		Source:        "windows_update",
		LastCheckUnix: time.Now().Unix(),
		Security: securityUpdate{
			Pending:      items,
			PendingCount: len(items),
		},
		Pending: len(items),
	}
}

// collectGPUs tenta coletar info de GPU via nvidia-smi (quando disponível).
func collectGPUs() []gpuSnapshot {
	cmd, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil
	}
	out, err := exec.Command(cmd, "--query-gpu=name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu,fan.speed,power.draw", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var gpus []gpuSnapshot
	for _, ln := range lines {
		fields := strings.Split(ln, ",")
		if len(fields) < 8 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		memTotal := parseFloat(fields[1])
		memUsed := parseFloat(fields[2])
		memFree := parseFloat(fields[3])
		util := parseFloat(fields[4])
		temp := parseFloat(fields[5])
		fan := parseFloat(fields[6])
		power := parseFloat(fields[7])
		gpus = append(gpus, gpuSnapshot{
			Vendor:       "nvidia",
			Name:         name,
			MemoryTotal:  memTotal,
			MemoryUsed:   memUsed,
			MemoryFree:   memFree,
			UtilPercent:  util,
			TemperatureC: temp,
			FanPercent:   fan,
			PowerW:       power,
		})
	}
	return gpus
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func pingTargets() []string { return defaultPingTargets }
func dnsTargets() []string  { return defaultDNSTargets }

// collectServices tenta consultar serviços via systemctl (Linux) ou sc query (Windows).
// Lista enxuta; expande conforme necessidade.
func collectServices() []serviceSnap {
	if runtime.GOOS == "windows" {
		return collectServicesWindows()
	}
	return collectServicesSystemd()
}

func collectServicesSystemd() []serviceSnap {
	path, err := exec.LookPath("systemctl")
	if err != nil {
		return nil
	}
	out, err := exec.Command(path, "list-units", "--type=service", "--state=running,failed", "--no-legend", "--plain").Output()
	if err != nil {
		return nil
	}
	var services []serviceSnap
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, ln := range lines {
		fields := strings.Fields(ln)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		loadActive := fields[1]
		services = append(services, serviceSnap{Name: name, Status: loadActive})
	}
	return services
}

func collectServicesWindows() []serviceSnap {
	path, err := exec.LookPath("sc.exe")
	if err != nil {
		return nil
	}
	out, err := exec.Command(path, "query", "type=service", "state=all").Output()
	if err != nil {
		return nil
	}
	var services []serviceSnap
	var current serviceSnap
	lines := strings.Split(string(out), "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "SERVICE_NAME:") {
			if current.Name != "" {
				services = append(services, current)
			}
			current = serviceSnap{Name: strings.TrimSpace(strings.TrimPrefix(ln, "SERVICE_NAME:"))}
		} else if strings.HasPrefix(ln, "STATE") {
			parts := strings.Fields(ln)
			if len(parts) >= 4 {
				current.Status = parts[3] // RUNNING/STOPPED
			}
		}
	}
	if current.Name != "" {
		services = append(services, current)
	}
	return services
}

// readFanSpeeds faz melhor esforço em Linux lendo /sys/class/hwmon/**/fan*_input.
func readFanSpeeds() []fanReading {
	var out []fanReading
	base := "/sys/class/hwmon"
	if _, err := os.Stat(base); err != nil {
		return nil
	}
	hwmons, err := filepath.Glob(filepath.Join(base, "hwmon*"))
	if err != nil {
		return nil
	}
	for _, hw := range hwmons {
		entries, err := os.ReadDir(hw)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(name, "fan") || !strings.HasSuffix(name, "_input") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(hw, name))
			if err != nil {
				continue
			}
			val, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
			if err != nil {
				continue
			}
			out = append(out, fanReading{Sensor: name, RPM: val})
		}
	}
	return out
}

func protoName(t uint32) string {
	switch t {
	case 1:
		return "tcp"
	case 2:
		return "udp"
	default:
		return "unknown"
	}
}

func isListeningPort(c gnet.ConnectionStat) bool {
	if c.Status == "LISTEN" {
		return true
	}
	// UDP sockets não têm estado LISTEN; considere portas UDP abertas.
	if c.Type == syscall.SOCK_DGRAM && c.Laddr.Port > 0 {
		return true
	}
	return false
}

func topProcesses(ctx context.Context, topCPU, topMem int) []procSnapshot {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil || len(procs) == 0 {
		return nil
	}

	type procData struct {
		p    *process.Process
		cpu  float64
		rss  uint64
		vms  uint64
		name string
	}

	all := make([]procData, 0, len(procs))
	for _, pr := range procs {
		name, _ := pr.NameWithContext(ctx)
		cpuPct, _ := pr.PercentWithContext(ctx, 0)
		if mi, err := pr.MemoryInfoWithContext(ctx); err == nil {
			all = append(all, procData{
				p:    pr,
				cpu:  cpuPct,
				rss:  mi.RSS,
				vms:  mi.VMS,
				name: name,
			})
		}
	}

	cpuOrder := make([]procData, len(all))
	memOrder := make([]procData, len(all))
	copy(cpuOrder, all)
	copy(memOrder, all)

	sort.Slice(cpuOrder, func(i, j int) bool { return cpuOrder[i].cpu > cpuOrder[j].cpu })
	sort.Slice(memOrder, func(i, j int) bool { return memOrder[i].rss > memOrder[j].rss })

	selected := make(map[int32]procData)
	for i := 0; i < len(cpuOrder) && i < topCPU; i++ {
		selected[cpuOrder[i].p.Pid] = cpuOrder[i]
	}
	for i := 0; i < len(memOrder) && i < topMem; i++ {
		if _, ok := selected[memOrder[i].p.Pid]; !ok {
			selected[memOrder[i].p.Pid] = memOrder[i]
		}
	}

	out := make([]procSnapshot, 0, len(selected))
	for _, data := range selected {
		snap := procSnapshot{
			PID:        data.p.Pid,
			Name:       data.name,
			CPUPercent: data.cpu,
			RSSBytes:   data.rss,
			VMSBytes:   data.vms,
		}
		if ct, err := data.p.CreateTimeWithContext(ctx); err == nil {
			snap.CreateTimeUnix = ct / 1000 // ms to s
		}
		if st, err := data.p.StatusWithContext(ctx); err == nil && len(st) > 0 {
			if len(st) == 1 {
				snap.Status = st[0]
			} else {
				snap.Status = strings.Join(st, ",")
			}
		}
		if cmd, err := data.p.CmdlineSliceWithContext(ctx); err == nil && len(cmd) > 0 {
			joined := strings.Join(cmd, " ")
			if len(joined) > 200 {
				joined = joined[:200]
			}
			snap.Cmdline = joined
		}
		if ioStats, err := data.p.IOCountersWithContext(ctx); err == nil && ioStats != nil {
			snap.IOReadBytes = ioStats.ReadBytes
			snap.IOWriteBytes = ioStats.WriteBytes
		}
		out = append(out, snap)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CPUPercent > out[j].CPUPercent })
	return out
}

// detectCVEs retorna uma lista de CVEs conhecidas para o host.
// Combina heurísticas locais e, se disponível, matching com assinaturas em data/cve_signatures.jsonl.
func detectCVEs(ctx context.Context, prefs config.CollectPrefs) []string {
	seen := make(map[string]struct{})
	add := func(id string) {
		if id != "" {
			seen[id] = struct{}{}
		}
	}

	// Heurísticas leves para nunca retornar vazio.
	// Kernel: se versão major < 5, marcar CVE-2016-5195 (Dirty COW) como sinalização de kernel legado.
	if hi, err := host.InfoWithContext(ctx); err == nil {
		if k := hi.KernelVersion; k != "" {
			if maj, _ := parseMajor(k); maj > 0 && maj < 5 {
				add("CVE-2016-5195")
			}
		}
		// Windows: versões antigas vulneráveis a BlueKeep (aproximação).
		if hi.OS == "windows" && hi.PlatformVersion != "" {
			// PlatformVersion costuma vir como "10.0.19045" ou "6.1.7601"
			if strings.HasPrefix(hi.PlatformVersion, "6.0.") || strings.HasPrefix(hi.PlatformVersion, "6.1.") || strings.HasPrefix(hi.PlatformVersion, "6.2.") || strings.HasPrefix(hi.PlatformVersion, "6.3.") {
				add("CVE-2019-0708") // BlueKeep em versões pré-10
			}
			// HiveNightmare/SeriousSAM em Windows 10/11 builds antigos.
			if strings.HasPrefix(hi.PlatformVersion, "10.0.") {
				add("CVE-2021-36934")
			}
		}
	}

	// OpenSSL: detectar versões antigas comuns.
	if path, err := exec.LookPath("openssl"); err == nil {
		out, err := exec.CommandContext(ctx, path, "version").Output()
		if err == nil {
			ver := parseOpenSSLVersion(string(out))
			if ver != "" {
				if ltSemver(ver, "1.1.1t") && strings.HasPrefix(ver, "1.1.1") {
					add("CVE-2023-0286")
				}
				if strings.HasPrefix(ver, "1.0.2") {
					add("CVE-2016-2107")
				}
				if strings.HasPrefix(ver, "1.0.1") {
					add("CVE-2014-0160") // Heartbleed
				}
			}
		}
	}

	// SSH: versões bem antigas suscetíveis a CVEs clássicas.
	if path, err := exec.LookPath("ssh"); err == nil {
		out, err := exec.CommandContext(ctx, path, "-V").CombinedOutput()
		if err == nil {
			ver := parseSSHVersion(string(out))
			if ver != "" {
				if strings.HasPrefix(ver, "5.") || strings.HasPrefix(ver, "6.0") || strings.HasPrefix(ver, "6.1") {
					add("CVE-2016-20012")
				}
				if strings.HasPrefix(ver, "7.2") || strings.HasPrefix(ver, "7.1") {
					add("CVE-2016-6210")
				}
			}
		}
	}

	if len(seen) == 0 {
		add("CVE-UNKNOWN-NO-DB")
	}
	// Regras a partir de assinaturas locais ou remotas (quando existirem).
	inv := detectPackages(ctx)
	for _, sig := range loadCveSignatures(prefs.CVESignaturesURL) {
		if sig.OS != "" && sig.OS != runtime.GOOS {
			continue
		}
		localVer, ok := inv[sig.Pkg]
		if !ok || localVer == "" {
			continue
		}
		if compareVersionOp(localVer, sig.Op, sig.Version) {
			for _, c := range sig.CVEs {
				add(c)
			}
		}
	}

	out := make([]string, 0, len(seen))
	for cve := range seen {
		out = append(out, cve)
	}
	sort.Strings(out)
	return out
}

func parseMajor(ver string) (int, error) {
	re := regexp.MustCompile(`^([0-9]+)`)
	m := re.FindStringSubmatch(ver)
	if len(m) < 2 {
		return 0, fmt.Errorf("no major")
	}
	return strconv.Atoi(m[1])
}

func parseOpenSSLVersion(out string) string {
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return ""
	}
	return strings.TrimSpace(fields[1])
}

func parseSSHVersion(out string) string {
	// ssh -V envia para stderr, mas CombinedOutput trata.
	parts := strings.Fields(out)
	if len(parts) == 0 {
		return ""
	}
	// Normalmente: OpenSSH_8.9p1, LibreSSL 3.3.6
	if strings.HasPrefix(parts[0], "OpenSSH_") {
		return strings.TrimPrefix(parts[0], "OpenSSH_")
	}
	return ""
}

// collectInventory retorna inventário para CVE: pacotes Linux (EVR), repos, kernel ou hotfix/apps/features Windows.
func collectInventory(ctx context.Context) inventorySnap {
	var inv inventorySnap
	switch runtime.GOOS {
	case "linux":
		inv.LinuxRPMPackages = listLinuxPackages(ctx)
		inv.OSRelease = readOSRelease()
		inv.Kernel = collectKernelInfo(ctx)
		inv.Repos = collectLinuxRepos(ctx)
	case "windows":
		inv.WinHotfixes = collectWindowsHotfixes(ctx)
		inv.WinApps = collectWindowsApps(ctx)
		inv.WinFeatures = collectWindowsFeatures(ctx)
	}
	return inv
}

// detectPackages retorna mapa pacote->versão a partir de dpkg/rpm (best effort).
func detectPackages(ctx context.Context) map[string]string {
	out := make(map[string]string)
	for _, pkg := range listLinuxPackages(ctx) {
		out[pkg.Name] = pkg.Version
	}
	return out
}

// listLinuxPackages devolve inventário com EVR para RPM (RHEL-like) ou versão simples via dpkg.
func listLinuxPackages(ctx context.Context) []rpmPkg {
	var pkgs []rpmPkg

	if path, err := exec.LookPath("dpkg"); err == nil {
		cmd := exec.CommandContext(ctx, path, "-l")
		raw, err := cmd.Output()
		if err == nil {
			sc := bufio.NewScanner(bytes.NewReader(raw))
			for sc.Scan() {
				ln := sc.Text()
				fields := strings.Fields(ln)
				if len(fields) >= 3 && strings.HasPrefix(fields[0], "ii") {
					pkgs = append(pkgs, rpmPkg{
						Name:    fields[1],
						Version: fields[2],
						Source:  "dpkg",
					})
				}
			}
		}
	}

	if path, err := exec.LookPath("rpm"); err == nil {
		cmd := exec.CommandContext(ctx, path, "-qa", "--qf", "%{NAME}\t%{EPOCHNUM}\t%{VERSION}\t%{RELEASE}\t%{ARCH}\t%{VENDOR}\n")
		raw, err := cmd.Output()
		if err == nil {
			sc := bufio.NewScanner(bytes.NewReader(raw))
			for sc.Scan() {
				ln := sc.Text()
				parts := strings.Split(ln, "\t")
				if len(parts) >= 5 {
					pkgs = append(pkgs, rpmPkg{
						Name:    parts[0],
						Epoch:   atoiPrefix(parts[1]),
						Version: parts[2],
						Release: parts[3],
						Arch:    parts[4],
						Vendor:  safeIdx(parts, 5),
						Source:  "rpm",
					})
				}
			}
		}
	}

	return pkgs
}

func safeIdx(parts []string, idx int) string {
	if idx < len(parts) {
		return parts[idx]
	}
	return ""
}

func collectKernelInfo(ctx context.Context) kernelInfo {
	k := kernelInfo{}
	if hi, err := host.InfoWithContext(ctx); err == nil {
		k.Running = hi.KernelVersion
	}
	// filtra pacotes kernel*
	for _, pkg := range listLinuxPackages(ctx) {
		if strings.HasPrefix(pkg.Name, "kernel") {
			k.Installed = append(k.Installed, pkg)
		}
	}
	return k
}

func readOSRelease() osRelease {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return osRelease{}
	}
	defer f.Close()
	var res osRelease
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(kv[0])
		val := strings.Trim(kv[1], "\"")
		switch key {
		case "id":
			res.ID = val
		case "version_id":
			res.VersionID = val
		case "pretty_name":
			res.PrettyName = val
		}
	}
	return res
}

func collectLinuxRepos(ctx context.Context) repoSnap {
	var rs repoSnap
	path, err := exec.LookPath("dnf")
	if err != nil {
		path, err = exec.LookPath("yum")
	}
	if err != nil {
		return rs
	}
	cmd := exec.CommandContext(ctx, path, "repolist", "-v")
	raw, err := cmd.Output()
	if err != nil {
		return rs
	}
	rs.Raw = string(raw)
	sc := bufio.NewScanner(bytes.NewBuffer(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(strings.ToLower(line), "repo-id") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				rs.Enabled = append(rs.Enabled, parts[1])
			}
		}
	}
	return rs
}

func loadCveSignatures(remoteURL string) []cveSignature {
	cveSigMu.Lock()
	defer cveSigMu.Unlock()

	// Recarrega remoto se URL mudou ou passou 6h.
	if remoteURL != "" && (remoteURL != cveSigLastURL || time.Since(cveSigLastFetch) > 6*time.Hour) {
		if sigs := fetchRemoteSignatures(remoteURL); len(sigs) > 0 {
			cveSigCache = sigs
			cveSigLastURL = remoteURL
			cveSigLastFetch = time.Now()
			return cveSigCache
		}
	}

	if len(cveSigCache) == 0 {
		path := "./data/cve_signatures.jsonl"
		f, err := os.Open(path)
		if err != nil {
			cveSigCache = nil
			return cveSigCache
		}
		defer f.Close()
		var sigs []cveSignature
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var s cveSignature
			if err := json.Unmarshal(sc.Bytes(), &s); err == nil {
				if len(s.CVEs) > 0 && s.Pkg != "" && s.Op != "" && s.Version != "" {
					sigs = append(sigs, s)
				}
			}
		}
		cveSigCache = sigs
	}

	return cveSigCache
}

// compareVersionOp faz comparação simples de versões "major.minor.patch" (com sufixos ignorados).
func compareVersionOp(local, op, target string) bool {
	cmp := compareSimpleVersion(local, target)
	switch op {
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "=", "==":
		return cmp == 0
	default:
		return false
	}
}

func compareSimpleVersion(a, b string) int {
	split := func(s string) []string {
		s = strings.ReplaceAll(s, "_", "-")
		s = strings.ReplaceAll(s, "+", "-")
		return strings.Split(s, ".")
	}
	pa := split(a)
	pb := split(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(pa) {
			ai = atoiPrefix(pa[i])
		}
		if i < len(pb) {
			bi = atoiPrefix(pb[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func atoiPrefix(s string) int {
	re := regexp.MustCompile(`^([0-9]+)`)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	v, _ := strconv.Atoi(m[1])
	return v
}

// fetchRemoteSignatures tenta baixar um jsonl de assinaturas CVE.
func fetchRemoteSignatures(url string) []cveSignature {
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil
	}
	var sigs []cveSignature
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		var s cveSignature
		if err := json.Unmarshal(sc.Bytes(), &s); err == nil {
			if len(s.CVEs) > 0 && s.Pkg != "" && s.Op != "" && s.Version != "" {
				sigs = append(sigs, s)
			}
		}
	}
	return sigs
}

// ltSemver faz comparação simples major.minor.patch[letter], sem suf fixos complexos.
func ltSemver(v, target string) bool {
	vParts := strings.SplitN(v, ".", 3)
	tParts := strings.SplitN(target, ".", 3)
	for len(vParts) < 3 {
		vParts = append(vParts, "0")
	}
	for len(tParts) < 3 {
		tParts = append(tParts, "0")
	}
	for i := 0; i < 3; i++ {
		vi := trimNonDigit(vParts[i])
		ti := trimNonDigit(tParts[i])
		viNum, _ := strconv.Atoi(vi)
		tiNum, _ := strconv.Atoi(ti)
		if viNum < tiNum {
			return true
		}
		if viNum > tiNum {
			return false
		}
	}
	// Se números iguais, comparar letras (ex.: 1.1.1d < 1.1.1t).
	vLetter := tailLetter(v)
	tLetter := tailLetter(target)
	return vLetter < tLetter
}

func trimNonDigit(s string) string {
	re := regexp.MustCompile(`^([0-9]+)`)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return "0"
	}
	return m[1]
}

func tailLetter(s string) rune {
	for i := len(s) - 1; i >= 0; i-- {
		ch := rune(s[i])
		if ch >= 'a' && ch <= 'z' {
			return ch
		}
	}
	return 0
}
