package sysutil

import (
	"fmt"
	"runtime"
	"strings"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

func getHardwareInfo() []*pb.ServerInfoItem {
	var results []*pb.ServerInfoItem
	// cpu
	infos, err := cpu.Info()
	if err == nil && len(infos) > 0 {
		physicalCores, err := cpu.Counts(false)
		if err != nil {
			physicalCores = int(infos[0].Cores)
		}
		results = append(results, &pb.ServerInfoItem{
			Tp:   "cpu",
			Name: "cpu",
			Pairs: []*pb.ServerInfoPair{
				{Key: "cpu-arch", Value: fmt.Sprintf("%s", runtime.GOARCH)},
				{Key: "cpu-logical-cores", Value: fmt.Sprintf("%d", runtime.NumCPU())},
				{Key: "cpu-physical-cores", Value: fmt.Sprintf("%d", physicalCores)},
				{Key: "cpu-frequency", Value: fmt.Sprintf("%.2fMHz", infos[0].Mhz)},
				{Key: "cache", Value: fmt.Sprintf("%d", infos[0].CacheSize)},
			},
		})
	}

	// memory
	ms, err := mem.VirtualMemory()
	if err == nil {
		results = append(results, &pb.ServerInfoItem{
			Tp:   "memory",
			Name: "memory",
			Pairs: []*pb.ServerInfoPair{
				{Key: "capacity", Value: fmt.Sprintf("%d", ms.Total)},
			},
		})
	}

	// disk
	parts, err := disk.Partitions(true)
	if err == nil && len(parts) > 0 {
		for _, p := range parts {
			if !strings.HasPrefix(p.Device, "/dev/") {
				continue
			}
			usage, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue
			}
			results = append(results, &pb.ServerInfoItem{
				Tp:   "disk",
				Name: p.Device[5:],
				Pairs: []*pb.ServerInfoPair{
					{Key: "fstype", Value: p.Fstype},
					{Key: "opts", Value: p.Opts},
					{Key: "path", Value: p.Mountpoint},
					{Key: "total", Value: fmt.Sprintf("%d", usage.Total)},
					{Key: "free", Value: fmt.Sprintf("%d", usage.Free)},
					{Key: "used", Value: fmt.Sprintf("%d", usage.Used)},
					{Key: "free-percent", Value: fmt.Sprintf("%.2f", (100-usage.UsedPercent)/100.00)},
					{Key: "used-percent", Value: fmt.Sprintf("%.2f", usage.UsedPercent/100.00)},
				},
			})
		}
	}

	// network
	nics, err := net.Interfaces()
	if err == nil && len(nics) > 0 {
		for _, nic := range nics {
			flag := func(f string) string {
				for _, s := range nic.Flags {
					if s == f {
						return "true"
					}
				}
				return "false"
			}
			var addrs []string
			for _, addr := range nic.Addrs {
				addrs = append(addrs, addr.Addr)
			}
			results = append(results, &pb.ServerInfoItem{
				Tp:   "net",
				Name: nic.Name,
				Pairs: []*pb.ServerInfoPair{
					{Key: "mac", Value: nic.HardwareAddr},
					{Key: "is-up", Value: flag("up")},
					{Key: "is-broadcast", Value: flag("broadcast")},
					{Key: "is-multicast", Value: flag("multicast")},
					{Key: "is-loopback", Value: flag("loopback")},
					{Key: "is-point-to-point", Value: flag("pointtopoint")},
					{Key: "addresses", Value: strings.Join(addrs, ",")},
				},
			})
		}
	}
	return results
}
