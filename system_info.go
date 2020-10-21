package sysutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/shirou/gopsutil/process"
)

func tryProcFs() []*pb.ServerInfoItem {
	const dir = "/proc/sys/"
	item := &pb.ServerInfoItem{
		Tp:   "system",
		Name: "sysctl",
	}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		content, err := ioutil.ReadFile(path)
		// Ignore this file
		if err != nil {
			return nil
		}
		item.Pairs = append(item.Pairs, &pb.ServerInfoPair{
			Key:   strings.ReplaceAll(strings.TrimPrefix(path, dir), "/", "."),
			Value: strings.TrimSpace(string(content)),
		})
		return nil
	})
	if err != nil {
		return nil
	}
	return []*pb.ServerInfoItem{item}
}

func getSystemInfo() []*pb.ServerInfoItem {
	hugePage := getTransparentHugepageEnabled()
	if results := tryProcFs(); len(results) > 0 {
		return append(results, hugePage...)
	}
	// fallback to command line
	cmd := exec.Command("sysctl", "-a")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	buf := bytes.NewBuffer(out)
	reader := bufio.NewReader(buf)
	pairs := make([]*pb.ServerInfoPair, 0, 2048)
	for {
		l, _, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil
		}
		kv := strings.Split(string(l), ":")
		if len(kv) >= 2 {
			pairs = append(pairs, &pb.ServerInfoPair{
				Key:   kv[0],
				Value: strings.TrimSpace(kv[1]),
			})

		}
	}

	var results []*pb.ServerInfoItem
	results = append(results, &pb.ServerInfoItem{
		Tp:    "system",
		Name:  "sysctl",
		Pairs: pairs,
	})

	return append(results, hugePage...)
}

// TODO: use different `ServerInfoType` to collect process list
func getProcessList() []*pb.ServerInfoItem {
	var results []*pb.ServerInfoItem
	pids, err := process.Pids()
	if err != nil {
		return results
	}
	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		name, err := p.Name()
		if err != nil {
			continue
		}
		prop := func(fn func() (string, error)) string {
			s, _ := fn()
			return s
		}
		ct, err := p.CreateTime()
		if err != nil {
			continue
		}
		us, err := p.CPUPercent()
		if err != nil {
			continue
		}
		results = append(results, &pb.ServerInfoItem{
			Tp:   "process",
			Name: fmt.Sprintf("%s(%d)", name, pid),
			Pairs: []*pb.ServerInfoPair{
				{Key: "executable", Value: prop(p.Exe)},
				{Key: "cmd", Value: prop(p.Cmdline)},
				{Key: "cwd", Value: prop(p.Cwd)},
				{Key: "start-time", Value: fmt.Sprintf("%d", ct)},
				{Key: "status", Value: prop(p.Status)},
				{Key: "cpu-usage", Value: fmt.Sprintf("%.2f", us)},
			},
		})
	}

	return results
}

func getTransparentHugepageEnabled() []*pb.ServerInfoItem {
	path := "/sys/kernel/mm/transparent_hugepage/enabled"
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil
	}
	item := &pb.ServerInfoItem{
		Tp:   "system",
		Name: "kernel",
	}
	item.Pairs = append(item.Pairs, &pb.ServerInfoPair{
		Key:   "transparent_hugepage_enabled",
		Value: strings.TrimSpace(string(content)),
	})
	return []*pb.ServerInfoItem{item}
}
