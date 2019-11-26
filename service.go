package sysutil

import (
	"context"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
)

type DiagnoseServer struct {
	pb.DiagnosticsServer
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
