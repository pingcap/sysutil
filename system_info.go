package sysutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/shirou/gopsutil/process"
)

func getSystemInfo() []*pb.ServerInfoItem {
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
