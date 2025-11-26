package sysmetrics

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
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
	queueStats func() (int, int64)
	prefs      func() config.CollectPrefs
}

func New(queueStats func() (int, int64), prefsProvider func() config.CollectPrefs) ports.Collector {
	return &collector{queueStats: queueStats, prefs: prefsProvider}
}

func (c *collector) Name() string { return "sysmetrics" }

func (c *collector) Interval() time.Duration { return 10 * time.Second }

type snapshot struct {
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	CPU          cpuSnapshot     `json:"cpu,omitempty"`
	Memory       memSnapshot     `json:"memory,omitempty"`
	Disk         diskSnapshot    `json:"disk,omitempty"`
	Network      netSnapshot     `json:"network,omitempty"`
	Host         hostSnapshot    `json:"host,omitempty"`
	Sensors      sensorsSnap     `json:"sensors,omitempty"`
	NetActive    netActive       `json:"net_active,omitempty"`
	Power        powerSnapshot   `json:"power,omitempty"`
	Sanity       sanitySnapshot  `json:"sanity,omitempty"`
	GPU          []gpuSnapshot   `json:"gpu,omitempty"`
	Services     []serviceSnap   `json:"services,omitempty"`
	TimeSync     timeSyncSnap    `json:"time_sync,omitempty"`
	Logs         []logFileSnap   `json:"logs,omitempty"`
	Updates      []updatesSnap   `json:"updates,omitempty"`
	Agent        agentSnap       `json:"agent,omitempty"`
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
)

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
	QueueItems int    `json:"queue_items,omitempty"`
	QueueBytes int64  `json:"queue_bytes,omitempty"`
	Version    string `json:"version,omitempty"`
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
	Source  string `json:"source"`
	Pending int    `json:"pending"`
	Error   string `json:"error,omitempty"`
}

type procSnapshot struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name,omitempty"`
	CPUPercent float64 `json:"cpu_percent,omitempty"`
	RSSBytes   uint64  `json:"rss_bytes,omitempty"`
	VMSBytes   uint64  `json:"vms_bytes,omitempty"`
}

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	p := prefs.Default()
	if c.prefs != nil {
		p = c.prefs()
	}

	s := snapshot{Capabilities: make(map[string]bool)}
	agentInfo := agentSnap{Version: version.Version}

	if p.CPU {
		cpuOk := false
		if totals, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(totals) > 0 {
			s.CPU.PercentTotal = totals[0]
			cpuOk = true
		}
		if perCPU, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
			s.CPU.PercentPerCPU = perCPU
			cpuOk = true
		}
		if l, err := load.AvgWithContext(ctx); err == nil {
			s.CPU.Load1, s.CPU.Load5, s.CPU.Load15 = l.Load1, l.Load5, l.Load15
			cpuOk = true
		}
		if n, err := cpu.CountsWithContext(ctx, true); err == nil {
			s.CPU.CoresLogical = n
			cpuOk = true
		}
		if n, err := cpu.CountsWithContext(ctx, false); err == nil {
			s.CPU.CoresPhysical = n
			cpuOk = true
		}
		if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
			s.CPU.FreqCurrentMHz = infos[0].Mhz
			s.CPU.FreqMaxMHz = infos[0].Mhz
			cpuOk = true
		}
		s.Capabilities["cpu"] = cpuOk
	} else {
		s.Capabilities["cpu"] = false
	}

	if p.Memory {
		memOk := false
		if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
			s.Memory = memSnapshot{
				Total:       vm.Total,
				Used:        vm.Used,
				Free:        vm.Free,
				UsedPercent: vm.UsedPercent,
				Buffers:     vm.Buffers,
				Cached:      vm.Cached,
			}
			memOk = true
		}
		if swap, err := mem.SwapMemoryWithContext(ctx); err == nil {
			s.Memory.SwapTotal = swap.Total
			s.Memory.SwapUsed = swap.Used
			s.Memory.SwapFree = swap.Free
			s.Memory.SwapUsedPerc = swap.UsedPercent
			memOk = true
		}
		s.Capabilities["memory"] = memOk
	} else {
		s.Capabilities["memory"] = false
	}

	if p.Disk {
		diskOk := false
		if parts, err := disk.PartitionsWithContext(ctx, true); err == nil {
			devSeen := make(map[string]struct{})
			for _, p := range parts {
				if u, err := disk.UsageWithContext(ctx, p.Mountpoint); err == nil {
					s.Disk.Filesystems = append(s.Disk.Filesystems, diskFS{
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
				s.Disk.SMART = sm
				diskOk = true
			}
		}
		if ioStats, err := disk.IOCountersWithContext(ctx); err == nil {
			for name, io := range ioStats {
				s.Disk.IOStats = append(s.Disk.IOStats, diskIO{
					Device:      name,
					Reads:       io.ReadCount,
					Writes:      io.WriteCount,
					ReadBytes:   io.ReadBytes,
					WriteBytes:  io.WriteBytes,
					ReadTimeMs:  io.ReadTime,
					WriteTimeMs: io.WriteTime,
				})
			}
			if len(s.Disk.IOStats) > 0 {
				sort.Slice(s.Disk.IOStats, func(i, j int) bool { return s.Disk.IOStats[i].Device < s.Disk.IOStats[j].Device })
				diskOk = true
			}
		}
		s.Capabilities["disk"] = diskOk
	} else {
		s.Capabilities["disk"] = false
	}

	if p.Network {
		netOk := false
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
				s.Network.Interfaces = append(s.Network.Interfaces, netIf{
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
			if len(s.Network.Interfaces) > 0 {
				netOk = true
			}
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
				if c.Status == "LISTEN" {
					lp := listenPort{
						Proto:     protoName(c.Type),
						LocalAddr: c.Laddr.IP,
						LocalPort: c.Laddr.Port,
					}
					listening = append(listening, lp)
				}
			}
			s.NetActive = netActive{
				ConnectionsByState: stateCount,
				Listening:          listening,
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
			s.Host = hostSnapshot{
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
		if temps, err := host.SensorsTemperaturesWithContext(ctx); err == nil {
			for _, t := range temps {
				s.Sensors.Temperatures = append(s.Sensors.Temperatures, tempReading{
					Sensor: t.SensorKey,
					TempC:  t.Temperature,
				})
			}
		}
		if fans := readFanSpeeds(); len(fans) > 0 {
			s.Sensors.Fans = fans
		}
		s.Capabilities["sensors"] = len(s.Sensors.Temperatures) > 0 || len(s.Sensors.Fans) > 0
	} else {
		s.Capabilities["sensors"] = false
	}

	if p.Power {
		if bats, err := battery.GetAll(); err == nil {
			for _, b := range bats {
				s.Power.Batteries = append(s.Power.Batteries, batterySnapshot{
					Percent:        b.Current / b.Full * 100,
					State:          b.State.String(),
					DesignCapacity: b.Design,
					FullCapacity:   b.Full,
					ChargeRateMw:   b.ChargeRate,
					Voltage:        b.Voltage,
				})
			}
		}
		s.Capabilities["power"] = len(s.Power.Batteries) > 0
	} else {
		s.Capabilities["power"] = false
	}

	if p.Sanity {
		s.Sanity = sanitySnapshot{
			Ping: multiPing(pingTargets(), 2*time.Second),
			DNS:  multiDNS(dnsTargets(), 2*time.Second),
		}
		s.Capabilities["sanity"] = len(s.Sanity.Ping) > 0 || len(s.Sanity.DNS) > 0
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
		s.TimeSync = timeSyncCheck("time.google.com", 3*time.Second)
		s.Capabilities["time_sync"] = s.TimeSync.Source != ""
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
	} else {
		s.Capabilities["agent"] = false
	}

	s.Agent = agentInfo
	if p.Processes {
		s.Processes = topProcesses(ctx, 5)
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
	ts.OffsetMs = int64(resp.ClockOffset / time.Millisecond)
	ts.RTTMs = resp.RTT.Milliseconds()
	return ts
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
		return []updatesSnap{aptUpdates(timeout)}
	case "darwin":
		return []updatesSnap{macUpdates(timeout)}
	case "windows":
		return []updatesSnap{{Source: "windows_update", Pending: 0, Error: "not implemented"}}
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

func topProcesses(ctx context.Context, limit int) []procSnapshot {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil || len(procs) == 0 {
		return nil
	}

	stats := make([]procSnapshot, 0, len(procs))
	for _, p := range procs {
		name, _ := p.NameWithContext(ctx)
		cpuPct, _ := p.PercentWithContext(ctx, 0)
		if mi, err := p.MemoryInfoWithContext(ctx); err == nil {
			stats = append(stats, procSnapshot{
				PID:        p.Pid,
				Name:       name,
				CPUPercent: cpuPct,
				RSSBytes:   mi.RSS,
				VMSBytes:   mi.VMS,
			})
		}
	}

	sort.Slice(stats, func(i, j int) bool { return stats[i].CPUPercent > stats[j].CPUPercent })
	if len(stats) > limit {
		stats = stats[:limit]
	}
	return stats
}
