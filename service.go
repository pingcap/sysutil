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
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"runtime"
	"sort"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/pingcap/log"
)

type DiagnosticsServer struct {
	logFile string
}

func NewDiagnosticsServer(logFile string) *DiagnosticsServer {
	return &DiagnosticsServer{
		logFile: logFile,
	}
}

// SearchLog implements the DiagnosticsServer interface.
func (d *DiagnosticsServer) SearchLog(req *pb.SearchLogRequest, stream pb.Diagnostics_SearchLogServer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			stackSize := runtime.Stack(buf, false)
			buf = buf[:stackSize]
			err = fmt.Errorf(fmt.Sprintf("search log panic, %v, stack is %v", r, string(buf)))
			log.Error(err.Error())
		}
	}()

	beginTime := req.StartTime
	endTime := req.EndTime
	if endTime == 0 {
		endTime = math.MaxInt64
	}

	ctx := stream.Context()
	logFiles, err := resolveFiles(ctx, d.logFile, beginTime, endTime)
	if err != nil {
		return err
	}

	// Sort log files by start time
	var searchFiles []*os.File
	for _, f := range logFiles {
		searchFiles = append(searchFiles, f.file)
	}
	var levelFlag int64
	for _, l := range req.Levels {
		levelFlag |= 1 << l
	}
	var patterns []*regexp.Regexp
	for _, p := range req.Patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return err
		}
		patterns = append(patterns, re)
	}
	iter := logIterator{
		begin:     beginTime,
		end:       endTime,
		levelFlag: levelFlag,
		patterns:  patterns,
		pending:   searchFiles,
	}
	defer iter.close()

	for {
		var messages []*pb.LogMessage
		var drained bool
		for i := 0; i < 1024; i++ {
			item, err := iter.next(ctx)
			if err == io.EOF {
				drained = true
				break
			}
			if err != nil {
				return err
			}
			messages = append(messages, item)
		}
		res := &pb.SearchLogResponse{
			Messages: messages,
		}
		if err := stream.Send(res); err != nil {
			return err
		}
		if drained {
			break
		}
	}
	return nil
}

// ServerInfo implements the DiagnosticsServer interface.
func (d *DiagnosticsServer) ServerInfo(ctx context.Context, req *pb.ServerInfoRequest) (*pb.ServerInfoResponse, error) {
	var items []*pb.ServerInfoItem
	switch req.Tp {
	case pb.ServerInfoType_LoadInfo:
		items = getLoadInfo()
	case pb.ServerInfoType_HardwareInfo:
		items = getHardwareInfo()
	case pb.ServerInfoType_SystemInfo:
		items = getSystemInfo()
	case pb.ServerInfoType_All:
		items = append(items, getLoadInfo()...)
		items = append(items, getHardwareInfo()...)
		items = append(items, getSystemInfo()...)
	}

	sort.Slice(items, func(i, j int) bool {
		lhs, rhs := items[i], items[j]
		if lhs.Tp != rhs.Tp {
			return lhs.Tp < rhs.Tp
		}
		return lhs.Name < rhs.Name
	})
	return &pb.ServerInfoResponse{Items: items}, nil
}
