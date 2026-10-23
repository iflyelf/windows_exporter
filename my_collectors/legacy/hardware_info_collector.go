//go:build windows

// Package collector 是 windows_exporter v0.23.1 的采集器包。
//
// 本文件是 my_collectors/hardware_info_collector.go 的 legacy 版本：
// v0.23.1 使用旧的 Collector 接口和 WMI 实现，不能直接使用 v0.32 的
// internal/mi 版本。这里只依赖 v0.23.1 已有的 wmi / registry / prometheus，
// 因此可以用 Go 1.20 构建，从而支持 Windows Server 2008 R2。
package collector

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows/registry"
)

// hardwareInfoCache 避免每次 scrape 都查询 WMI（WMI 调用较慢）。
var hardwareInfoCache = struct {
	LastScan time.Time
	Data     *HardwareInfo
	Mutex    sync.RWMutex
}{Data: nil}

var hardwareInfoInterval = func() time.Duration {
	if v := os.Getenv("HARDWARE_INFO_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 8 * time.Hour
}()

var hardwareInfoTimeout = func() time.Duration {
	if v := os.Getenv("HARDWARE_INFO_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 10 * time.Second
}()

// HardwareInfo 与 v0.32 版本保持同名字段，方便统一告警/看板。
type HardwareInfo struct {
	BoardVendor   string
	BoardProduct  string
	BoardSerial   string
	BoardVersion  string
	SystemVendor  string
	SystemProduct string
	SystemSerial  string
	SystemUUID    string
	CPUModel      string
	CPUCores      int
	MemoryTotal   float64
	DiskTotal     float64
	OSName        string
	OSType        string
	OSVersion     string
}

type win32ComputerSystem struct {
	Manufacturer        string
	Model               string
	TotalPhysicalMemory uint64
}

type win32BIOS struct {
	SerialNumber string
}

type win32ComputerSystemProduct struct {
	UUID string
}

type win32BaseBoard struct {
	Manufacturer string
	Product      string
	SerialNumber string
	Version      string
}

type win32Processor struct {
	Name                      string
	NumberOfCores             uint32
	NumberOfLogicalProcessors uint32
}

type win32LogicalDisk struct {
	Size uint64
}

// queryWMI 为 WMI 查询增加超时保护；WMI 在部分老机器上可能挂起。
func queryWMI(ctx context.Context, query string, dst interface{}) error {
	done := make(chan error, 1)
	go func() {
		done <- wmi.Query(query, dst)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func readOSInfo(info *HardwareInfo) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer func() { _ = k.Close() }()

	if v, _, err := k.GetStringValue("ProductName"); err == nil && v != "" {
		info.OSName = v
	}
	version, _, _ := k.GetStringValue("CurrentVersion")
	build, _, _ := k.GetStringValue("CurrentBuildNumber")
	info.OSType = "windows"
	if version != "" {
		info.OSVersion = version
		if build != "" {
			info.OSVersion += "." + build
		}
	}
}

func collectHardwareInfo() *HardwareInfo {
	ctx, cancel := context.WithTimeout(context.Background(), hardwareInfoTimeout)
	defer cancel()

	info := &HardwareInfo{
		BoardVendor: "未知", BoardProduct: "未知", BoardSerial: "未知", BoardVersion: "未知",
		SystemVendor: "未知", SystemProduct: "未知", SystemSerial: "未知", SystemUUID: "未知",
		CPUModel: "未知", OSName: "未知", OSType: "windows", OSVersion: "未知",
	}

	var systems []win32ComputerSystem
	if queryWMI(ctx, "SELECT Manufacturer, Model, TotalPhysicalMemory FROM Win32_ComputerSystem", &systems) == nil && len(systems) > 0 {
		if systems[0].Manufacturer != "" {
			info.SystemVendor = systems[0].Manufacturer
		}
		if systems[0].Model != "" {
			info.SystemProduct = systems[0].Model
		}
		info.MemoryTotal = float64(systems[0].TotalPhysicalMemory)
	}

	var bios []win32BIOS
	if queryWMI(ctx, "SELECT SerialNumber FROM Win32_BIOS", &bios) == nil && len(bios) > 0 && bios[0].SerialNumber != "" {
		info.SystemSerial = bios[0].SerialNumber
	}

	var products []win32ComputerSystemProduct
	if queryWMI(ctx, "SELECT UUID FROM Win32_ComputerSystemProduct", &products) == nil && len(products) > 0 && products[0].UUID != "" {
		info.SystemUUID = products[0].UUID
	}

	var boards []win32BaseBoard
	if queryWMI(ctx, "SELECT Manufacturer, Product, SerialNumber, Version FROM Win32_BaseBoard", &boards) == nil && len(boards) > 0 {
		b := boards[0]
		if b.Manufacturer != "" {
			info.BoardVendor = b.Manufacturer
		}
		if b.Product != "" {
			info.BoardProduct = b.Product
		}
		if b.SerialNumber != "" {
			info.BoardSerial = b.SerialNumber
		}
		if b.Version != "" {
			info.BoardVersion = b.Version
		}
	}

	var cpus []win32Processor
	if queryWMI(ctx, "SELECT Name, NumberOfCores, NumberOfLogicalProcessors FROM Win32_Processor", &cpus) == nil {
		cores := 0
		for _, c := range cpus {
			if info.CPUModel == "未知" && c.Name != "" {
				info.CPUModel = strings.TrimSpace(c.Name)
			}
			if c.NumberOfLogicalProcessors > 0 {
				cores += int(c.NumberOfLogicalProcessors)
			} else {
				cores += int(c.NumberOfCores)
			}
		}
		info.CPUCores = cores
	}

	var disks []win32LogicalDisk
	if queryWMI(ctx, "SELECT Size FROM Win32_LogicalDisk WHERE DriveType=3", &disks) == nil {
		for _, d := range disks {
			info.DiskTotal += float64(d.Size)
		}
	}

	readOSInfo(info)
	return info
}

func getHardwareInfo() *HardwareInfo {
	hardwareInfoCache.Mutex.RLock()
	expired := time.Since(hardwareInfoCache.LastScan) > hardwareInfoInterval || hardwareInfoCache.Data == nil
	hardwareInfoCache.Mutex.RUnlock()
	if expired {
		hardwareInfoCache.Mutex.Lock()
		if time.Since(hardwareInfoCache.LastScan) > hardwareInfoInterval || hardwareInfoCache.Data == nil {
			hardwareInfoCache.Data = collectHardwareInfo()
			hardwareInfoCache.LastScan = time.Now()
		}
		hardwareInfoCache.Mutex.Unlock()
	}
	hardwareInfoCache.Mutex.RLock()
	defer hardwareInfoCache.Mutex.RUnlock()
	return hardwareInfoCache.Data
}

func convertSize(size float64) (float64, string) {
	switch {
	case size >= 1024*1024*1024*1024:
		return size / (1024 * 1024 * 1024 * 1024), "TB"
	case size >= 1024*1024*1024:
		return size / (1024 * 1024 * 1024), "GB"
	case size >= 1024*1024:
		return size / (1024 * 1024), "MB"
	default:
		return size / 1024, "KB"
	}
}

type hardwareInfoCollector struct {
	boardInfoDesc   *prometheus.Desc
	systemInfoDesc  *prometheus.Desc
	cpuInfoDesc     *prometheus.Desc
	memoryTotalDesc *prometheus.Desc
	diskTotalDesc   *prometheus.Desc
	osInfoDesc      *prometheus.Desc
}

// NewHardwareInfoCollector 供 registerCollector 使用（v0.23.1 的构造签名）。
func NewHardwareInfoCollector(_ log.Logger) (Collector, error) {
	return &hardwareInfoCollector{
		boardInfoDesc: prometheus.NewDesc(
			"node_hardware_board_info",
			"主板信息（厂商、型号、序列号、版本）",
			[]string{"vendor", "product", "serial", "version"}, nil,
		),
		systemInfoDesc: prometheus.NewDesc(
			"node_hardware_system_info",
			"系统信息（厂商、型号、序列号、UUID）",
			[]string{"vendor", "product", "serial", "uuid"}, nil,
		),
		cpuInfoDesc: prometheus.NewDesc(
			"node_hardware_cpu_info",
			"CPU信息（型号、逻辑核心数）",
			[]string{"model"}, nil,
		),
		memoryTotalDesc: prometheus.NewDesc(
			"node_hardware_memory_total_bytes",
			"内存总容量（单位：字节）",
			[]string{"display"}, nil,
		),
		diskTotalDesc: prometheus.NewDesc(
			"node_hardware_disk_total_bytes",
			"磁盘总容量（单位：字节）",
			[]string{"display"}, nil,
		),
		osInfoDesc: prometheus.NewDesc(
			"node_hardware_os_info",
			"操作系统信息（名称、类型、版本）",
			[]string{"name", "type", "version"}, nil,
		),
	}, nil
}

func (c *hardwareInfoCollector) Collect(_ *ScrapeContext, ch chan<- prometheus.Metric) error {
	info := getHardwareInfo()

	ch <- prometheus.MustNewConstMetric(
		c.boardInfoDesc, prometheus.GaugeValue, 1,
		info.BoardVendor, info.BoardProduct, info.BoardSerial, info.BoardVersion,
	)
	ch <- prometheus.MustNewConstMetric(
		c.systemInfoDesc, prometheus.GaugeValue, 1,
		info.SystemVendor, info.SystemProduct, info.SystemSerial, info.SystemUUID,
	)
	ch <- prometheus.MustNewConstMetric(
		c.cpuInfoDesc, prometheus.GaugeValue, float64(info.CPUCores), info.CPUModel,
	)

	memVal, memUnit := convertSize(info.MemoryTotal)
	ch <- prometheus.MustNewConstMetric(
		c.memoryTotalDesc, prometheus.GaugeValue, info.MemoryTotal,
		fmt.Sprintf("%.0f %s", math.Round(memVal), memUnit),
	)

	diskVal, diskUnit := convertSize(info.DiskTotal)
	ch <- prometheus.MustNewConstMetric(
		c.diskTotalDesc, prometheus.GaugeValue, info.DiskTotal,
		fmt.Sprintf("%.0f %s", math.Round(diskVal), diskUnit),
	)

	ch <- prometheus.MustNewConstMetric(
		c.osInfoDesc, prometheus.GaugeValue, 1,
		info.OSName, info.OSType, info.OSVersion,
	)
	return nil
}

func init() {
	registerCollector("hardware_info", NewHardwareInfoCollector)
}
