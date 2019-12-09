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
func (d *DiagnosticsServer) SearchLog(_ context.Context, req *pb.SearchLogRequest) (*pb.SearchLogResponse, error) {
	beginTime := req.StartTime
	endTime := req.EndTime
	if endTime == 0 {
		endTime = math.MaxInt64
	}

	logFiles, err := resolveFiles(d.logFile, beginTime, endTime)
	if err != nil {
		return nil, err
	}

	// Sort log files by start time
	var searchFiles []*os.File
	for _, f := range logFiles {
		searchFiles = append(searchFiles, f.file)
	}
	iter := logIterator{
		begin:   req.GetStartTime(),
		end:     req.GetEndTime(),
		level:   req.GetLevel(),
		pattern: req.GetPattern(),
		pending: searchFiles,
	}
	defer iter.close()

	limit := req.GetLimit()
	if limit <= 0 {
		limit = 64 * 1025
	}

	var messages []*pb.LogMessage
	for i := int64(0); i < limit; i++ {
		item, err := iter.next()
		if err != nil && err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		messages = append(messages, item)
	}
	return &pb.SearchLogResponse{Messages: messages}, nil
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
