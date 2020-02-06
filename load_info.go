// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package sysutil

import (
	"fmt"
	"time"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

func getCpuLoad() []*pb.ServerInfoItem {
	var results []*pb.ServerInfoItem
	avgload, err := load.Avg()
	if err == nil {
		results = append(results, &pb.ServerInfoItem{
			Tp:   "cpu",
			Name: "cpu",
			Pairs: []*pb.ServerInfoPair{
				{Key: "load1", Value: fmt.Sprintf("%.2f", avgload.Load1)},
				{Key: "load5", Value: fmt.Sprintf("%.2f", avgload.Load5)},
				{Key: "load15", Value: fmt.Sprintf("%.2f", avgload.Load15)},
			},
		})
	}
	times, err := cpu.Times(true)
	if err == nil && len(times) > 0 {
		for _, t := range times {
			item := &pb.ServerInfoItem{
				Tp:   "cpu",
				Name: t.CPU,
				Pairs: []*pb.ServerInfoPair{
					{Key: "user", Value: fmt.Sprintf("%.2f", t.User)},
					{Key: "system", Value: fmt.Sprintf("%.2f", t.System)},
					{Key: "idle", Value: fmt.Sprintf("%.2f", t.Idle)},
					{Key: "nice", Value: fmt.Sprintf("%.2f", t.Nice)},
					{Key: "iowait", Value: fmt.Sprintf("%.2f", t.Iowait)},
					{Key: "irq", Value: fmt.Sprintf("%.2f", t.Irq)},
					{Key: "softirq", Value: fmt.Sprintf("%.2f", t.Softirq)},
					{Key: "steal", Value: fmt.Sprintf("%.2f", t.Steal)},
					{Key: "guest", Value: fmt.Sprintf("%.2f", t.Guest)},
					{Key: "guest_nice", Value: fmt.Sprintf("%.2f", t.GuestNice)},
				},
			}
			results = append(results, item)
		}
	}
	return results
}

func getMemLoad() []*pb.ServerInfoItem {
	var results []*pb.ServerInfoItem
	virt, err := mem.VirtualMemory()
	if err == nil {
		results = append(results, &pb.ServerInfoItem{
			Tp:   "memory",
			Name: "virtual",
			Pairs: []*pb.ServerInfoPair{
				{Key: "total", Value: fmt.Sprintf("%d", virt.Total)},
				{Key: "used", Value: fmt.Sprintf("%d", virt.Used)},
				{Key: "free", Value: fmt.Sprintf("%d", virt.Free)},
				{Key: "used-percent", Value: fmt.Sprintf("%.2f", float64(virt.Used)/float64(virt.Total))},
				{Key: "free-percent", Value: fmt.Sprintf("%.2f", float64(virt.Free)/float64(virt.Total))},
			},
		})
	}
	swap, err := mem.SwapMemory()
	if err == nil {
		results = append(results, &pb.ServerInfoItem{
			Tp:   "memory",
			Name: "swap",
			Pairs: []*pb.ServerInfoPair{
				{Key: "total", Value: fmt.Sprintf("%d", swap.Total)},
				{Key: "used", Value: fmt.Sprintf("%d", swap.Used)},
				{Key: "free", Value: fmt.Sprintf("%d", swap.Free)},
				{Key: "used-percent", Value: fmt.Sprintf("%.2f", float64(swap.Used)/float64(swap.Total))},
				{Key: "free-percent", Value: fmt.Sprintf("%.2f", float64(swap.Free)/float64(swap.Total))},
			},
		})
	}
	return results
}

func getNICLoad() []*pb.ServerInfoItem {
	var results []*pb.ServerInfoItem
	ics, err := net.IOCounters(true)
	if err != nil {
		return results
	}
	for _, ic := range ics {
		results = append(results, &pb.ServerInfoItem{
			Tp:   "net",
			Name: ic.Name,
			Pairs: []*pb.ServerInfoPair{
				{Key: "bytes-ent", Value: fmt.Sprintf("%d", ic.BytesSent)},
				{Key: "bytes-recv", Value: fmt.Sprintf("%d", ic.BytesRecv)},
				{Key: "packets-sent", Value: fmt.Sprintf("%d", ic.PacketsSent)},
				{Key: "packets-recv", Value: fmt.Sprintf("%d", ic.PacketsRecv)},
				{Key: "errin", Value: fmt.Sprintf("%d", ic.Errin)},
				{Key: "errout", Value: fmt.Sprintf("%d", ic.Errout)},
				{Key: "dropin", Value: fmt.Sprintf("%d", ic.Dropin)},
				{Key: "dropout", Value: fmt.Sprintf("%d", ic.Dropout)},
				{Key: "fifoin", Value: fmt.Sprintf("%d", ic.Fifoin)},
				{Key: "fifoout", Value: fmt.Sprintf("%d", ic.Fifoout)},
			},
		})
	}
	return results
}

func getDiskLoad() []*pb.ServerInfoItem {
	var results []*pb.ServerInfoItem
	snapshot, err := disk.IOCounters()
	if err != nil {
		return nil
	}
	time.Sleep(500 * time.Millisecond)
	current, err := disk.IOCounters()
	if err != nil {
		return nil
	}
	prev := map[string]disk.IOCountersStat{}
	for _, s := range snapshot {
		prev[s.Name] = s
	}
	var rate = func(p, c uint64) string {
		return fmt.Sprintf("%.2f", float64(c-p)/0.5)
	}
	for _, c := range current {
		p, ok := prev[c.Name]
		if !ok {
			continue
		}
		results = append(results, &pb.ServerInfoItem{
			Tp:   "net",
			Name: c.Name,
			Pairs: []*pb.ServerInfoPair{
				{Key: "read_count/s", Value: rate(p.ReadCount, c.ReadCount)},
				{Key: "merged_read_count/s", Value: rate(p.MergedReadCount, c.MergedReadCount)},
				{Key: "write_count/s", Value: rate(p.WriteCount, c.WriteCount)},
				{Key: "merged_write_count/s", Value: rate(p.MergedWriteCount, c.MergedWriteCount)},
				{Key: "read_bytes/s", Value: rate(p.ReadBytes, c.ReadBytes)},
				{Key: "write_bytes/s", Value: rate(p.WriteBytes, c.WriteBytes)},
			},
		})
	}
	return results
}

func getLoadInfo() []*pb.ServerInfoItem {
	var results []*pb.ServerInfoItem
	results = append(results, getCpuLoad()...)
	results = append(results, getMemLoad()...)
	results = append(results, getNICLoad()...)
	results = append(results, getDiskLoad()...)
	return results
}
