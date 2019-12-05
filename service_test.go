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
	"log"
	"net"
	"testing"
	"time"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"google.golang.org/grpc"
)

func TestRPCServerLoadInfo(t *testing.T) {
	address := "127.0.0.1:10080"
	setUpService(address)

	// Set up a connection to the server.
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := pb.NewDiagnosticsClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := c.ServerInfo(ctx, &pb.ServerInfoRequest{Tp: pb.ServerInfoType_LoadInfo})
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || len(r.Items) == 0 {
		t.Fatal()
	}
	return
}

func setUpService(addr string) {
	go func() {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		s := grpc.NewServer()
		pb.RegisterDiagnosticsServer(s, &DiagnoseServer{})
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
}
