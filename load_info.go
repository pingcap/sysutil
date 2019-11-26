package sysutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

// singleDevicesLoadInfoFns is the slice of single device load info functions.
// Such as cpu, memory.
var singleDevicesLoadInfoFns = []struct {
	tp   string
	name string
	fn   func() (interface{}, error)
}{
	{"cpu", "cpu", getCPULoad},
	{"mem", "virtual", getVirtualMemStat},
	{"mem", "swap", getSwapMemStat},
}

// multiDevicesLoadInfoFns is the slice of multi-device load info functions.
// Such as disk, network card.
var multiDevicesLoadInfoFns = []struct {
	tp string
	fn func() (map[string]interface{}, error)
}{
	{"cpu", getCPUUsage},
	{"net", getNetIOs},
	{"disk", getDiskIOs},
	{"disk", getDiskUsage},
}

func getLoadInfo() ([]*pb.ServerInfoItem, error) {
	items := make([]*pb.ServerInfoItem, 0, len(singleDevicesLoadInfoFns))
	for _, f := range singleDevicesLoadInfoFns {
		data, err := f.fn()
		if err != nil {
			return nil, err
		}
		item, err := convertToServerInfoItems(f.tp, f.name, data)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for _, f := range multiDevicesLoadInfoFns {
		ds, err := f.fn()
		if err != nil {
			return nil, err
		}

		for k, data := range ds {
			item, err := convertToServerInfoItems(f.tp, k, data)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	return items, nil
}

func getCPULoad() (interface{}, error) {
	return load.Avg()
}

func getVirtualMemStat() (interface{}, error) {
	return mem.VirtualMemory()
}

func getSwapMemStat() (interface{}, error) {
	return mem.SwapMemory()
}

func getCPUUsage() (map[string]interface{}, error) {
	usages, err := cpu.Percent(time.Millisecond*100, true)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{}, len(usages))
	for i,usage := range usages {
		name := "cpu-" +  strconv.FormatInt(int64(i),10)
		m[name] = map[string]interface{}{
			"usage": usage,
		}
	}
	return m,nil
}

func getNetIOs() (map[string]interface{}, error) {
	ics, err := net.IOCounters(true)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{}, len(ics))
	for _, ic := range ics {
		m[ic.Name] = ic
	}
	return m, nil
}

func getDiskIOs() (map[string]interface{}, error) {
	disksIO, err := disk.IOCounters()
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{}, len(disksIO))
	for _, diskIO := range disksIO {
		m[diskIO.Name] = diskIO
	}
	return m, nil
}

func getDiskUsage() (map[string]interface{}, error) {
	parts, err := disk.Partitions(true)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{}, len(parts))
	for _, part := range parts {
		diskInfo, err := disk.Usage(part.Mountpoint)
		if err != nil {
			return nil, err
		}
		m[part.Device] = diskInfo
	}
	return m, nil
}

func convertToServerInfoItems(tp, name string, data interface{}) (*pb.ServerInfoItem, error) {
	buf, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	m := make(map[string]interface{})
	err = json.Unmarshal(buf, &m)
	if err != nil {
		return nil, err
	}

	pairs := make([]*pb.ServerInfoPair, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, &pb.ServerInfoPair{
			Key:   convertCamelNameToKebabName(k),
			Value: fmt.Sprintf("%v", v),
		})
	}
	return &pb.ServerInfoItem{
		Tp:    tp,
		Name:  name,
		Pairs: pairs,
	}, nil
}

func convertCamelNameToKebabName(name string) string {
	var buf bytes.Buffer
	for _, c := range name {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
			buf.WriteByte('-')
		}
		buf.WriteByte(byte(c))

	}
	return buf.String()
}
