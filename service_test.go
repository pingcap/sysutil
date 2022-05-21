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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sysutil_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"testing"
	"time"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/pingcap/sysutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type serviceSuite struct {
	server  *grpc.Server
	address string
}

func createServiceSuite(t *testing.T) (*serviceSuite, func()) {
	s := new(serviceSuite)

	server := grpc.NewServer()
	pb.RegisterDiagnosticsServer(server, &sysutil.DiagnosticsServer{})

	// Find a available port
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "cannot find available port")

	s.server = server
	s.address = fmt.Sprintf(":%d", listener.Addr().(*net.TCPAddr).Port)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	return s, func() {
		s.server.Stop()
	}
}

func TestRPCServerInfo(t *testing.T) {
	s, clean := createServiceSuite(t)
	defer clean()

	// Set up a connection to the server.
	conn, err := grpc.Dial(s.address, grpc.WithInsecure())
	require.NoError(t, err)

	defer func() {
		require.NoError(t, conn.Close())
	}()
	client := pb.NewDiagnosticsClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test for load info.
	r, err := client.ServerInfo(ctx, &pb.ServerInfoRequest{Tp: pb.ServerInfoType_LoadInfo})
	require.NoError(t, err)
	require.NotNil(t, r)
	require.NotZero(t, len(r.Items))

	// Test for hardware info.
	r, err = client.ServerInfo(ctx, &pb.ServerInfoRequest{Tp: pb.ServerInfoType_HardwareInfo})
	require.NoError(t, err)
	require.NotNil(t, r)
	require.NotZero(t, len(r.Items))

	// Test for system info.
	r, err = client.ServerInfo(ctx, &pb.ServerInfoRequest{Tp: pb.ServerInfoType_SystemInfo})
	require.NoError(t, err)
	require.NotNil(t, r)
	require.NotZero(t, len(r.Items))
}
