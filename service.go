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

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
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
func (d *DiagnosticsServer) SearchLog(req *pb.SearchLogRequest, stream pb.Diagnostics_SearchLogServer) error {
	beginTime := req.StartTime
	endTime := req.EndTime
	if endTime == 0 {
		endTime = math.MaxInt64
	}

	logFiles, err := resolveFiles(d.logFile, beginTime, endTime)
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
	iter := logIterator{
		begin:     req.StartTime,
		end:       req.EndTime,
		levelFlag: levelFlag,
		pattern:   req.Pattern,
		pending:   searchFiles,
	}
	defer iter.close()

	for {
		var messages []*pb.LogMessage
		var drained bool
		for i := 0; i < 1024; i++ {
			item, err := iter.next()
			if err != nil && err == io.EOF {
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
	var err error
	switch req.Tp {
	case pb.ServerInfoType_LoadInfo:
		items, err = getLoadInfo()
	case pb.ServerInfoType_HardwareInfo:
		items, err = getHardwareInfo()
	case pb.ServerInfoType_SystemInfo:
		items, err = getSystemInfo()
	}
	if err != nil {
		return nil, err
	}
	return &pb.ServerInfoResponse{Items: items}, nil
}
