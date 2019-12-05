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

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
)

type DiagnoseServer struct {
}

// SearchLog implements the DiagnosticsServer interface.
func (d *DiagnoseServer) SearchLog(context.Context, *pb.SearchLogRequest) (*pb.SearchLogResponse, error) {
	panic("unimplemented")
}

// ServerInfo implements the DiagnosticsServer interface.
func (d *DiagnoseServer) ServerInfo(ctx context.Context, req *pb.ServerInfoRequest) (*pb.ServerInfoResponse, error) {
	var items []*pb.ServerInfoItem
	var err error
	switch req.Tp {
	case pb.ServerInfoType_LoadInfo:
		items, err = getLoadInfo()
	}
	if err != nil {
		return nil, err
	}
	return &pb.ServerInfoResponse{Items: items}, nil
}
