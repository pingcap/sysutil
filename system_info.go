package sysutil

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"os/exec"
	"strings"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
)

func getSystemInfo() ([]*pb.ServerInfoItem, error) {
	return getSysctl()
}

func getSysctl() ([]*pb.ServerInfoItem, error) {
	cmd := exec.Command("sysctl", "-a")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
	buf := bytes.NewBuffer(out)
	reader := bufio.NewReader(buf)
	m := make(map[string]string)
	for {
		l, _, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		kv := strings.Split(string(l), ":")
		if len(kv) >= 2 {
			m[kv[0]] = strings.TrimSpace(kv[1])
		}
	}
	items := make([]*pb.ServerInfoItem, 0, len(singleDevicesLoadInfoFns))
	pairs := make([]*pb.ServerInfoPair, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, &pb.ServerInfoPair{
			Key:   k,
			Value: v,
		})
	}
	items = append(items, &pb.ServerInfoItem{
		Tp:    "system",
		Name:  "sysctl",
		Pairs: pairs,
	})
	return items, nil
}
